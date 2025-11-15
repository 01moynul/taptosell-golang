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
// --- Manager: Product Approval Handlers ---
//

// GetPendingProducts is the handler for GET /v1/manager/products/pending
// It retrieves all products with the status "pending".
func (h *Handlers) GetPendingProducts(c *gin.Context) {
	// 1. --- Build Query ---
	query := `
		SELECT 
			id, supplier_id, sku, name, description, price, stock, is_variable, status, created_at, updated_at,
			weight, pkg_length, pkg_width, pkg_height
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
			&product.Weight,
			&product.PkgLength,
			&product.PkgWidth,
			&product.PkgHeight,
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

	// 1. --- Begin Transaction ---
	// We must use a transaction because we are performing multiple
	// database operations (Update product, get supplier, add notification)
	// that must all succeed or fail together.
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback() // Safety net

	// 2. --- Get Product Info (and lock the row) ---
	// We get the supplier_id and name *before* updating
	// We use 'FOR UPDATE' to lock this product row for the transaction.
	var supplierID int64
	var productName string
	err = tx.QueryRow("SELECT supplier_id, name FROM products WHERE id = ? AND status = 'pending' FOR UPDATE", productIDStr).Scan(&supplierID, &productName)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or was not pending approval"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get product details"})
		return
	}

	// 3. --- Update Product Status ---
	query := `
		UPDATE products
		SET status = ?, updated_at = ?
		WHERE id = ? AND status = ?`

	args := []interface{}{"published", time.Now(), productIDStr, "pending"}

	_, err = tx.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve product"})
		return
	}

	// 4. --- Add Notification (NEW STEP) ---
	message := fmt.Sprintf("Your product \"%s\" has been approved and is now published.", productName)
	link := fmt.Sprintf("/supplier/products/%s", productIDStr) // A future frontend link

	if err := h.AddNotification(tx, supplierID, message, link); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send notification"})
		return
	}

	// 5. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

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

	// 2. --- Begin Transaction ---
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// 3. --- Get Product Info (and lock the row) ---
	var supplierID int64
	var productName string
	err = tx.QueryRow("SELECT supplier_id, name FROM products WHERE id = ? AND status = 'pending' FOR UPDATE", productIDStr).Scan(&supplierID, &productName)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or was not pending approval"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get product details"})
		return
	}

	// 4. --- Update Database ---
	// TODO: Save the input.Reason to a 'rejection_reason' column.
	query := `
		UPDATE products
		SET status = ?, updated_at = ?
		WHERE id = ? AND status = ?`

	args := []interface{}{"rejected", time.Now(), productIDStr, "pending"}

	_, err = tx.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject product"})
		return
	}

	// 5. --- Add Notification (NEW STEP) ---
	message := fmt.Sprintf("Your product \"%s\" was rejected. Reason: %s", productName, input.Reason)
	link := fmt.Sprintf("/supplier/products/%s", productIDStr) // A future frontend link

	if err := h.AddNotification(tx, supplierID, message, link); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send notification"})
		return
	}

	// 6. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Product rejected successfully",
	})
}
