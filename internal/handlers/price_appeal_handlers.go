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
// --- Manager: Price Appeal Handlers ---
//

// GetPriceAppeals is the handler for GET /v1/manager/price-requests
// It retrieves all 'pending' price appeals for managers to review.
func (h *Handlers) GetPriceAppeals(c *gin.Context) {
	// 1. --- Query Database ---
	// We JOIN with 'products' and 'users' to get all context for the manager
	query := `
		SELECT 
			pa.id, pa.product_id, pa.supplier_id, pa.old_price, pa.new_price,
			pa.reason, pa.status, pa.created_at,
			p.name AS product_name,
			u.full_name AS supplier_name,
			u.email AS supplier_email
		FROM price_appeals pa
		JOIN products p ON pa.product_id = p.id
		JOIN users u ON pa.supplier_id = u.id
		WHERE pa.status = 'pending'
		ORDER BY pa.created_at ASC
	`
	rows, err := h.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	// 2. --- Scan Rows ---
	var appeals []*models.PriceAppeal
	for rows.Next() {
		var appeal models.PriceAppeal
		if err := rows.Scan(
			&appeal.ID,
			&appeal.ProductID,
			&appeal.SupplierID,
			&appeal.OldPrice,
			&appeal.NewPrice,
			&appeal.Reason,
			&appeal.Status,
			&appeal.CreatedAt,
			&appeal.ProductName,
			&appeal.SupplierName,
			&appeal.SupplierEmail,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan price appeal"})
			return
		}
		appeals = append(appeals, &appeal)
	}

	if err = rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error iterating rows"})
		return
	}

	// 3. --- Send Response ---
	c.JSON(http.StatusOK, gin.H{
		"appeals": appeals,
	})
}

// ProcessPriceAppealInput defines the JSON for approving/rejecting a request
type ProcessPriceAppealInput struct {
	Action          string `json:"action" binding:"required,oneof=approve reject"`
	RejectionReason string `json:"rejectionReason,omitempty"`
}

// ProcessPriceAppeal is the handler for PATCH /v1/manager/price-requests/:id
func (h *Handlers) ProcessPriceAppeal(c *gin.Context) {
	// 1. --- Get IDs & Bind Input ---
	appealID := c.Param("id")

	var input ProcessPriceAppealInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Action == "reject" && input.RejectionReason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A rejectionReason is required when rejecting an appeal"})
		return
	}

	// 2. --- Begin Transaction ---
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// 3. --- Get Appeal Details ---
	// Lock the row and check its status
	var appeal models.PriceAppeal
	query := "SELECT id, product_id, supplier_id, new_price, status FROM price_appeals WHERE id = ? FOR UPDATE"
	err = tx.QueryRow(query, appealID).Scan(
		&appeal.ID,
		&appeal.ProductID,
		&appeal.SupplierID,
		&appeal.NewPrice,
		&appeal.Status,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Price appeal not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get appeal details"})
		return
	}

	if appeal.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"error": "This appeal has already been processed"})
		return
	}

	// 4. --- Process Action ---
	if input.Action == "approve" {
		// Action: Approve
		// 1. Update the appeal status
		appealQuery := "UPDATE price_appeals SET status = 'approved' WHERE id = ?"
		if _, err := tx.Exec(appealQuery, appealID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve appeal"})
			return
		}

		// 2. Update the actual price in the 'products' table
		productQuery := "UPDATE products SET price = ?, updated_at = ? WHERE id = ?"
		if _, err := tx.Exec(productQuery, appeal.NewPrice, time.Now(), appeal.ProductID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update product price"})
			return
		}

		// 3. Add notification to supplier
		message := fmt.Sprintf("Your price change request for product ID %d to RM %.2f has been approved.", appeal.ProductID, appeal.NewPrice)
		if err := h.AddNotification(tx, appeal.SupplierID, message, ""); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send notification"})
			return
		}

	} else {
		// Action: Reject
		// 1. Update the appeal status and reason
		appealQuery := "UPDATE price_appeals SET status = 'rejected', rejection_reason = ? WHERE id = ?"
		if _, err := tx.Exec(appealQuery, input.RejectionReason, appealID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject appeal"})
			return
		}

		// 2. Add notification to supplier
		message := fmt.Sprintf("Your price change request for product ID %d was rejected. Reason: %s", appeal.ProductID, input.RejectionReason)
		if err := h.AddNotification(tx, appeal.SupplierID, message, ""); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send notification"})
			return
		}
	}

	// 5. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// 6. --- Send Response ---
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Price appeal successfully %sed", input.Action),
	})
}
