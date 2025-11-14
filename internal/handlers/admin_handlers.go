package handlers

import (
	"net/http"
	"time"

	"github.com/01moynul/taptosell-golang/internal/models"
	"github.com/gin-gonic/gin"
)

//
// --- Manager: Product Approval Handlers ---
//

// GetPendingProducts is the handler for GET /v1/manager/products/pending
// It retrieves all products with the status "pending".
func (h *Handlers) GetPendingProducts(c *gin.Context) {
	// 1. --- Build Query ---
	query := `
		SELECT id, supplier_id, sku, name, description, price, stock, is_variable, status, created_at, updated_at
		FROM products
		WHERE status = ?
		ORDER BY created_at ASC`

	args := []interface{}{"pending"}

	// 2. --- Execute Query ---
	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	// 3. --- Scan Rows into Slice ---
	var products []*models.Product
	for rows.Next() {
		var product models.Product
		if err := rows.Scan(
			&product.ID,
			&product.SupplierID,
			&product.SKU,
			&product.Name,
			&product.Description,
			&product.Price,
			&product.Stock,
			&product.IsVariable,
			&product.Status,
			&product.CreatedAt,
			&product.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan product row"})
			return
		}
		// TODO: Attach supplier info, categories, brands, etc.
		products = append(products, &product)
	}
	if err = rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error iterating product rows"})
		return
	}

	// 4. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"products": products,
	})
}

// ApproveProduct is the handler for PATCH /v1/manager/products/:id/approve
// It changes a product's status from "pending" to "published".
func (h *Handlers) ApproveProduct(c *gin.Context) {
	productIDStr := c.Param("id")

	query := `
		UPDATE products
		SET status = ?, updated_at = ?
		WHERE id = ? AND status = ?`

	args := []interface{}{"published", time.Now(), productIDStr, "pending"}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve product"})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check affected rows"})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or was not pending approval"})
		return
	}

	// TODO: Send a notification to the supplier (Phase 4.4 extension)
	// We would query for the product's supplier_id and use a
	// (future) notifications.Send() function.

	c.JSON(http.StatusOK, gin.H{
		"message": "Product approved and published successfully",
	})
}

// RejectProductInput defines the JSON input for rejecting a product.
type RejectProductInput struct {
	Reason string `json:"reason" binding:"required"`
}

// RejectProduct is the handler for PATCH /v1/manager/products/:id/reject
// It changes a product's status from "pending" to "rejected".
func (h *Handlers) RejectProduct(c *gin.Context) {
	productIDStr := c.Param("id")

	// 1. --- Bind & Validate JSON ---
	var input RejectProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 2. --- Update Database ---
	// We'll store the rejection reason in a future 'rejection_reason'
	// column. For now, we just update the status.
	query := `
		UPDATE products
		SET status = ?, updated_at = ?
		WHERE id = ? AND status = ?`

	args := []interface{}{"rejected", time.Now(), productIDStr, "pending"}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject product"})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check affected rows"})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or was not pending approval"})
		return
	}

	// TODO: Save the input.Reason to a 'rejection_reason' column.
	// TODO: Send a notification to the supplier with the input.Reason.

	c.JSON(http.StatusOK, gin.H{
		"message": "Product rejected successfully",
	})
}
