package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/01moynul/taptosell-golang/internal/models"
	"github.com/gin-gonic/gin"
)

//
// --- Public Subscription Handlers ---
//

// GetSubscriptionPlans is the handler for GET /v1/subscriptions/plans
// It retrieves all plans that are marked as 'is_public'.
func (h *Handlers) GetSubscriptionPlans(c *gin.Context) {
	// 1. --- Query Database ---
	query := `
		SELECT id, name, description, price, duration_days, ai_credits_included
		FROM plans
		WHERE is_public = 1
		ORDER BY price ASC
	`
	rows, err := h.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	// 2. --- Scan Rows ---
	var plans []*models.Plan
	for rows.Next() {
		var plan models.Plan
		var desc sql.NullString // Handle nullable description
		if err := rows.Scan(
			&plan.ID,
			&plan.Name,
			&desc,
			&plan.Price,
			&plan.DurationDays,
			&plan.AiCreditsIncluded,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan plan row"})
			return
		}
		plan.Description = desc.String
		plans = append(plans, &plan)
	}

	if err = rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error iterating rows"})
		return
	}

	// 3. --- Send Response ---
	c.JSON(http.StatusOK, gin.H{
		"plans": plans,
	})
}

//
// --- Manager: Subscription Handlers ---
//

// AssignSubscriptionInput defines the JSON for assigning a plan to a user
type AssignSubscriptionInput struct {
	PlanID int64 `json:"planId" binding:"required"`
}

// AssignSubscription is the handler for POST /v1/manager/users/:id/subscription
// It assigns a subscription plan to a user.
func (h *Handlers) AssignSubscription(c *gin.Context) {
	// 1. --- Get User ID from URL ---
	userIDStr := c.Param("id")

	// 2. --- Bind & Validate JSON ---
	var input AssignSubscriptionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Begin Transaction ---
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// 4. --- Get Plan Details ---
	var plan models.Plan
	err = tx.QueryRow("SELECT duration_days, ai_credits_included FROM plans WHERE id = ?", input.PlanID).Scan(&plan.DurationDays, &plan.AiCreditsIncluded)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Plan not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get plan details"})
		return
	}

	// 5. --- Create or Update User Subscription ---
	// We'll set the expiry date based on the plan's duration
	now := time.Now()
	expiresAt := now.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)

	// Use ON DUPLICATE KEY UPDATE to either create a new subscription
	// or update the existing one for this user.
	subQuery := `
		INSERT INTO user_subscriptions
		(user_id, plan_id, status, expires_at, created_at, updated_at)
		VALUES (?, ?, 'active', ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		plan_id = VALUES(plan_id),
		status = VALUES(status),
		expires_at = VALUES(expires_at),
		updated_at = VALUES(updated_at)
	`
	_, err = tx.Exec(subQuery, userIDStr, input.PlanID, expiresAt, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assign subscription"})
		return
	}

	// 6. --- Add AI Credits ---
	// Use ON DUPLICATE KEY UPDATE to add credits.
	// This will create the user's credit record if it doesn't exist,
	// or add the included credits to their existing balance.
	creditQuery := `
		INSERT INTO ai_user_credits (user_id, credits_remaining, updated_at)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
		credits_remaining = credits_remaining + VALUES(credits_remaining),
		updated_at = VALUES(updated_at)
	`
	_, err = tx.Exec(creditQuery, userIDStr, plan.AiCreditsIncluded, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add AI credits"})
		return
	}

	// 7. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// 8. --- Send Response ---
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Subscription successfully assigned to user %s. Credits added: %.4f", userIDStr, plan.AiCreditsIncluded),
	})
}
