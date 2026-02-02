package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

//
// --- Cart Handlers (Dropshipper-Only) ---
//

// getOrCreateCartID finds a user's active cart or creates one.
// This is a helper function to be used within a transaction.
func (h *Handlers) getOrCreateCartID(tx *sql.Tx, userID int64) (int64, error) {
	var cartID int64

	// 1. Try to find an existing cart
	query := "SELECT id FROM carts WHERE user_id = ?"
	err := tx.QueryRow(query, userID).Scan(&cartID)

	if err == nil {
		return cartID, nil // Found it
	}

	// 2. If no cart exists (sql.ErrNoRows), create one
	if err == sql.ErrNoRows {
		now := time.Now()
		insertQuery := "INSERT INTO carts (user_id, created_at, updated_at) VALUES (?, ?, ?)"
		result, err := tx.Exec(insertQuery, userID, now, now)
		if err != nil {
			return 0, err // Failed to create
		}
		newCartID, err := result.LastInsertId()
		if err != nil {
			return 0, err
		}
		return newCartID, nil
	}

	// 3. A real database error occurred
	return 0, err
}

// AddToCartInput defines the JSON for adding an item to the cart.
// AddToCartInput defines the JSON for adding an item to the cart.
// FIX: Updated tags to match the snake_case sent by cartService.ts
type AddToCartInput struct {
	ProductID int64  `json:"product_id" binding:"required"`
	VariantID *int64 `json:"variant_id"` // [NEW] Optional variant selection
	Quantity  int    `json:"quantity" binding:"required,gt=0"`
}

// [FIXED] AddToCart: Handles both Simple and Variable Products
func (h *Handlers) AddToCart(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	dropshipperID := userID_raw.(int64)

	var input AddToCartInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input: " + err.Error()})
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction failed"})
		return
	}
	defer tx.Rollback()

	cartID, err := h.getOrCreateCartID(tx, dropshipperID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cart initialization failed"})
		return
	}

	// [FIX] Phase 8.4: Determine price and stock based on Variant vs Base Product
	var stock int
	var price float64

	// If VariantID is provided and > 0, check the VARIANT table
	if input.VariantID != nil && *input.VariantID > 0 {
		err = tx.QueryRow(`
			SELECT stock_quantity, price_to_tts 
			FROM product_variants 
			WHERE id = ? AND product_id = ?`,
			*input.VariantID, input.ProductID).Scan(&stock, &price)

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Selected variant not found"})
			return
		}
	} else {
		// Otherwise, check the BASE PRODUCT table
		err = tx.QueryRow(`
			SELECT stock_quantity, price_to_tts 
			FROM products 
			WHERE id = ? AND status = 'active'`,
			input.ProductID).Scan(&stock, &price)

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or inactive"})
			return
		}
	}

	if stock < input.Quantity {
		c.JSON(http.StatusConflict, gin.H{"error": "Insufficient stock"})
		return
	}

	// [FIX] Manual Check-and-Update to avoid SQL "Unique NULL" headaches
	var existingQty int
	var checkQuery string
	var checkArgs []interface{}

	if input.VariantID != nil && *input.VariantID > 0 {
		checkQuery = "SELECT quantity FROM cart_items WHERE cart_id = ? AND product_id = ? AND variant_id = ?"
		checkArgs = []interface{}{cartID, input.ProductID, *input.VariantID}
	} else {
		checkQuery = "SELECT quantity FROM cart_items WHERE cart_id = ? AND product_id = ? AND variant_id IS NULL"
		checkArgs = []interface{}{cartID, input.ProductID}
	}

	err = tx.QueryRow(checkQuery, checkArgs...).Scan(&existingQty)

	if err == nil {
		// Item exists -> Update Quantity
		updateQuery := "UPDATE cart_items SET quantity = quantity + ?, updated_at = NOW() WHERE cart_id = ? AND product_id = ?"
		updateArgs := []interface{}{input.Quantity, cartID, input.ProductID}

		if input.VariantID != nil && *input.VariantID > 0 {
			updateQuery += " AND variant_id = ?"
			updateArgs = append(updateArgs, *input.VariantID)
		} else {
			updateQuery += " AND variant_id IS NULL"
		}

		_, err = tx.Exec(updateQuery, updateArgs...)
	} else {
		// Item does not exist -> Insert New
		_, err = tx.Exec(`
			INSERT INTO cart_items (cart_id, product_id, variant_id, quantity, updated_at)
			VALUES (?, ?, ?, ?, NOW())`,
			cartID, input.ProductID, input.VariantID, input.Quantity)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update cart items"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Commit failed"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Item added to cart"})
}

// CartItemResponse is a helper struct for the GetCart handler
// NO CHANGE: The JSON response struct remains the same for the frontend.
type CartItemResponse struct {
	ProductID int64   `json:"productId"`
	Name      string  `json:"name"`
	SKU       string  `json:"sku"`
	Price     float64 `json:"price"` // This is the 'TapToSell' price
	Quantity  int     `json:"quantity"`
	LineTotal float64 `json:"lineTotal"`
	Stock     int     `json:"stock"`
}

// GetCart is the handler for GET /v1/dropshipper/cart
// It retrieves the full contents of the user's cart.
// [FIXED] GetCart: Joins with Variants AND fetches Options for display
func (h *Handlers) GetCart(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	dropshipperID := userID_raw.(int64)

	var cartID int64
	err := h.DB.QueryRow("SELECT id FROM carts WHERE user_id = ?", dropshipperID).Scan(&cartID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"items": []interface{}{}, "subtotal": 0})
		return
	}

	// [CHANGE 1] Added 'v.options' to the SELECT statement
	query := `
		SELECT
			ci.product_id, 
			p.name, 
			COALESCE(v.sku, p.sku) as display_sku, 
			COALESCE(v.price_to_tts, p.price_to_tts) as unit_price, 
			ci.quantity, 
			COALESCE(v.stock_quantity, p.stock_quantity) as stock,
            v.options  -- <--- WE NEED THIS
		FROM cart_items ci
		JOIN products p ON ci.product_id = p.id
		LEFT JOIN product_variants v ON ci.variant_id = v.id
		WHERE ci.cart_id = ? AND p.status = 'active'
	`
	rows, err := h.DB.Query(query, cartID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch cart"})
		return
	}
	defer rows.Close()

	var items []gin.H
	var subtotal float64

	for rows.Next() {
		var pid int64
		var name, sku string
		var price float64
		var qty, stock int
		var optionsJSON []byte // [CHANGE 2] Buffer to catch the JSON string

		// [CHANGE 3] Scan the optionsJSON
		err := rows.Scan(&pid, &name, &sku, &price, &qty, &stock, &optionsJSON)
		if err != nil {
			continue
		}

		lineTotal := price * float64(qty)
		subtotal += lineTotal

		// [CHANGE 4] Parse the options so React can display "Color: Red"
		var options []map[string]string
		if len(optionsJSON) > 0 {
			_ = json.Unmarshal(optionsJSON, &options)
		}

		items = append(items, gin.H{
			"product_id":   pid,
			"product_name": name, // Ensure frontend uses this key
			"name":         name, // Duplicate for safety if frontend uses 'name'
			"sku":          sku,
			"unit_price":   price,
			"price":        price, // Duplicate for safety
			"quantity":     qty,
			"stock":        stock,
			"lineTotal":    lineTotal,
			"options":      options, // <--- Send the parsed options to Frontend
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items":       items,
		"subtotal":    subtotal,
		"total_items": len(items),
		"grand_total": subtotal,
	})
}

// UpdateCartItemInput defines the JSON for updating an item's quantity.
type UpdateCartItemInput struct {
	Quantity int `json:"quantity" binding:"required,gte=0"` // gte=0 allows setting quantity to 0, which we'll treat as a delete
}

// UpdateCartItem is the handler for PUT /v1/dropshipper/cart/items/:product_id
func (h *Handlers) UpdateCartItem(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	dropshipperID := userID_raw.(int64)
	productIDStr := c.Param("product_id")

	// 2. --- Bind & Validate JSON ---
	var input UpdateCartItemInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Get User's Cart ID ---
	var cartID int64
	err := h.DB.QueryRow("SELECT id FROM carts WHERE user_id = ?", dropshipperID).Scan(&cartID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Cart not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find cart"})
		return
	}

	// --- Handle Quantity ---
	if input.Quantity == 0 {
		// If quantity is 0, this is a "delete" request.
		h.deleteCartItem(c, cartID, productIDStr)
		return
	}

	// 4. --- Check Stock ---
	// UPDATED: Select stock_quantity
	var stock int
	err = h.DB.QueryRow("SELECT stock_quantity FROM products WHERE id = ? AND status = 'active'", productIDStr).Scan(&stock)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check product stock"})
		return
	}
	if stock < input.Quantity {
		c.JSON(http.StatusConflict, gin.H{"error": "Not enough stock available for this quantity"})
		return
	}

	// 5. --- Execute Update ---
	query := `
		UPDATE cart_items
		SET quantity = ?, updated_at = ?
		WHERE cart_id = ? AND product_id = ?`

	result, err := h.DB.Exec(query, input.Quantity, time.Now(), cartID, productIDStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update item"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found in cart"})
		return
	}

	// 6. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{"message": "Cart item quantity updated"})
}

// DeleteCartItem is the handler for DELETE /v1/dropshipper/cart/items/:product_id
func (h *Handlers) DeleteCartItem(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	dropshipperID := userID_raw.(int64)
	productIDStr := c.Param("product_id")

	// 2. --- Get User's Cart ID ---
	var cartID int64
	err := h.DB.QueryRow("SELECT id FROM carts WHERE user_id = ?", dropshipperID).Scan(&cartID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Cart not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find cart"})
		return
	}

	// 3. --- Call delete helper ---
	h.deleteCartItem(c, cartID, productIDStr)
}

// deleteCartItem is a helper to DRY up the delete logic
func (h *Handlers) deleteCartItem(c *gin.Context, cartID int64, productIDStr string) {
	// Execute atomic delete, checking both cart_id and product_id
	query := "DELETE FROM cart_items WHERE cart_id = ? AND product_id = ?"
	result, err := h.DB.Exec(query, cartID, productIDStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete item"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found in cart"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Cart item removed"})
}
