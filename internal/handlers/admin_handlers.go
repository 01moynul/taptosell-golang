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
			id, supplier_id, sku, name, description, price_to_tts, stock_quantity, 
			is_variable, status, created_at, updated_at,
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
		// [FIX] We scan directly into the struct.
		// Since models.Product now uses *float64 for Weight/Dimensions,
		// rows.Scan handles NULLs automatically (setting the pointer to nil).
		if err := rows.Scan(
			&product.ID,
			&product.SupplierID,
			&product.SKU,
			&product.Name,
			&product.Description,
			&product.PriceToTTS,
			&product.StockQuantity,
			&product.IsVariable,
			&product.Status,
			&product.CreatedAt,
			&product.UpdatedAt,
			&product.Weight,
			&product.PkgLength,
			&product.PkgWidth,
			&product.PkgHeight,
		); err != nil {
			fmt.Printf("Scan Error: %v\n", err) // Log scan errors to console
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan product row"})
			return
		}
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
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback() // Safety net

	// 2. --- Get Product Info (and lock the row) ---
	var supplierID int64
	var productName string
	// We use 'FOR UPDATE' to lock this product row for the transaction.
	err = tx.QueryRow("SELECT supplier_id, name FROM products WHERE id = ? AND status = 'pending' FOR UPDATE", productIDStr).Scan(&supplierID, &productName)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or was not pending approval"})
			return
		}
		fmt.Printf("ApproveProduct DB Error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get product details"})
		return
	}

	// 3. --- Update Product Status ---
	query := `
		UPDATE products
		SET status = ?, updated_at = ?
		WHERE id = ? AND status = ?`

	_, err = tx.Exec(query, "published", time.Now(), productIDStr, "pending")
	if err != nil {
		fmt.Printf("ApproveProduct Update Error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve product"})
		return
	}

	// 4. --- Add Notification ---
	// If this fails, we LOG it but we don't fail the entire approval (optional safety).
	message := fmt.Sprintf("Your product \"%s\" has been approved and is now published.", productName)
	link := fmt.Sprintf("/supplier/products") // Link to supplier's product list

	// NOTE: Assuming h.AddNotification accepts (tx *sql.Tx, ...).
	// If your AddNotification implementation does NOT accept tx, change 'tx' to 'h.DB' (but careful with locks).
	if err := h.AddNotification(tx, supplierID, message, link); err != nil {
		// Log the error but proceed (don't block approval just because notification failed?)
		// For now, we will block to be safe, but print the error.
		fmt.Printf("ApproveProduct Notification Error: %v\n", err)
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

	// 3. --- Get Product Info ---
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
	query := `
		UPDATE products
		SET status = ?, updated_at = ?
		WHERE id = ? AND status = ?`

	_, err = tx.Exec(query, "rejected", time.Now(), productIDStr, "pending")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject product"})
		return
	}

	// 5. --- Add Notification ---
	message := fmt.Sprintf("Your product \"%s\" was rejected. Reason: %s", productName, input.Reason)
	link := fmt.Sprintf("/supplier/products")

	if err := h.AddNotification(tx, supplierID, message, link); err != nil {
		fmt.Printf("RejectProduct Notification Error: %v\n", err)
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

// ... (GetSettings and UpdateSettings remain unchanged) ...
// You can keep the existing code for Settings below this point.
//
// --- Manager: Settings Handlers ---
//

// Setting is a helper struct for the GetSettings handler
type Setting struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// GetSettings is the handler for GET /v1/manager/settings
func (h *Handlers) GetSettings(c *gin.Context) {
	query := "SELECT setting_key, setting_value, description FROM settings"

	rows, err := h.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	settingsMap := make(map[string]Setting)
	for rows.Next() {
		var s Setting
		var desc sql.NullString
		if err := rows.Scan(&s.Key, &s.Value, &desc); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan setting row"})
			return
		}
		s.Description = desc.String
		settingsMap[s.Key] = s
	}

	c.JSON(http.StatusOK, gin.H{
		"settings": settingsMap,
	})
}

type UpdateSettingsInput struct {
	Settings map[string]string `json:"settings" binding:"required"`
}

// UpdateSettings is the handler for PATCH /v1/manager/settings
func (h *Handlers) UpdateSettings(c *gin.Context) {
	var input UpdateSettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(input.Settings) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No settings provided to update"})
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	query := `
		INSERT INTO settings (setting_key, setting_value)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value)
	`
	stmt, err := tx.Prepare(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare update statement"})
		return
	}
	defer stmt.Close()

	for key, value := range input.Settings {
		if _, err := stmt.Exec(key, value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update setting: %s", key)})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Settings updated successfully",
	})
}
