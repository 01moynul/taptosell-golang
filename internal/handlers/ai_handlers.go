package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ChatInput defines the structure of the JSON request body.
type ChatInput struct {
	Message string `json:"message" binding:"required"`
}

// ChatAI handles the interaction with the AI Assistant.
func (h *Handlers) ChatAI(c *gin.Context) {
	// 1. Get User Context
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	role, _ := c.Get("userRole")
	userRole := role.(string)

	// 2. Parse Input
	var input ChatInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. Get AI Settings (Model & Price) from DB
	// We fetch them live so the Admin can change them instantly.
	var modelName string
	var pricePer1kStr string

	// Fetch Model
	err := h.DB.QueryRow("SELECT setting_value FROM settings WHERE setting_key = 'ai_model'").Scan(&modelName)
	if err != nil {
		modelName = "gemini-1.5-flash" // Default fallback
	}

	// Fetch Price
	err = h.DB.QueryRow("SELECT setting_value FROM settings WHERE setting_key = 'ai_price_per_1k_tokens'").Scan(&pricePer1kStr)
	if err != nil {
		pricePer1kStr = "0.00" // Default fallback
	}
	pricePer1k, _ := strconv.ParseFloat(pricePer1kStr, 64)

	// 4. Call the AI Service
	aiResponse, tokenCount, err := h.AIService.GenerateResponse(c.Request.Context(), input.Message, userRole, modelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI Service unavailable: " + err.Error()})
		return
	}

	// 5. Calculate Cost
	// Formula: (Tokens Used / 1000) * Price Per 1k
	cost := (float64(tokenCount) / 1000.0) * pricePer1k

	// 6. Transaction: Deduct Credit & Save History
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database transaction failed"})
		return
	}

	// A. Deduct Credits
	_, err = tx.Exec("UPDATE ai_user_credits SET credits_remaining = credits_remaining - ? WHERE user_id = ?", cost, userID)
	if err != nil {
		tx.Rollback()
		// Note: If they run out mid-chat, they go negative. That is acceptable for now.
		fmt.Printf("Failed to deduct credits: %v\n", err)
	}

	// B. Save History
	query := `
		INSERT INTO ai_chat_history (user_id, user_role, user_message, ai_response, tokens_used, cost_incurred)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err = tx.Exec(query, userID, userRole, input.Message, aiResponse, tokenCount, cost)
	if err != nil {
		tx.Rollback()
		fmt.Printf("Failed to save history: %v\n", err)
	} else {
		tx.Commit()
	}

	// 7. Return Response
	c.JSON(http.StatusOK, gin.H{
		"response":      aiResponse,
		"tokens_used":   tokenCount,
		"cost_incurred": fmt.Sprintf("%.4f", cost), // Send back cost so UI can show "You spent RM 0.002"
	})
}
