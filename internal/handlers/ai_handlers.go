package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ChatInput defines the structure of the JSON request body.
type ChatInput struct {
	Message string `json:"message" binding:"required"`
}

// ChatAI handles the interaction with the AI Assistant.
func (h *Handlers) ChatAI(c *gin.Context) {
	// 1. Get User Context (set by AuthMiddleware)
	// We need to know WHO is asking (ID) and WHAT they are allowed to see (Role).
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userRole, exists := c.Get("userRole")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// 2. Parse Input
	var input ChatInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. Call the AI Service (The "Brain")
	// We pass the user's role so the AI knows if it's talking to a Supplier or Dropshipper.
	responseParam := input.Message
	roleParam := userRole.(string)

	aiResponse, err := h.AIService.GenerateResponse(c.Request.Context(), responseParam, roleParam)
	if err != nil {
		// If the AI fails (e.g., API error), we return a 500 error.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI Service unavailable: " + err.Error()})
		return
	}

	// 4. Save to History (The "Memory")
	// We record the interaction in the database for future reference/billing.
	// Note: For now, we set 'tokens_used' to 0. We can refine this later.
	query := `
		INSERT INTO ai_chat_history (user_id, user_role, user_message, ai_response, tokens_used, cost_incurred)
		VALUES (?, ?, ?, ?, 0, 0.00)
	`
	_, dbErr := h.DB.Exec(query, userID, roleParam, input.Message, aiResponse)
	if dbErr != nil {
		// We log the error but don't fail the request, as the user already got their answer.
		// In a production app, you might use a logger here.
		println("Warning: Failed to save chat history:", dbErr.Error())
	}

	// 5. Return the Answer
	c.JSON(http.StatusOK, gin.H{
		"response": aiResponse,
	})
}
