package handlers

import (
	"database/sql"
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
	ProductID int64 `json:"product_id" binding:"required"`
	Quantity  int   `json:"quantity" binding:"required,gt=0"`
}

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

	// Logic check: Ensure the product is 'active' (our new ENUM value)
	var stock int
	err = tx.QueryRow("SELECT stock_quantity FROM products WHERE id = ? AND status = 'active'", input.ProductID).Scan(&stock)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or not active"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if stock < input.Quantity {
		c.JSON(http.StatusConflict, gin.H{"error": "Insufficient stock"})
		return
	}

	// Insert or Update logic (Upsert)
	_, err = tx.Exec(`
		INSERT INTO cart_items (cart_id, product_id, quantity, updated_at)
		VALUES (?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE 
			quantity = quantity + VALUES(quantity),
			updated_at = NOW()`,
		cartID, input.ProductID, input.Quantity)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update cart"})
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
func (h *Handlers) GetCart(c *gin.Context) {
	// 1. --- Get Dropshipper ID ---
	userID_raw, _ := c.Get("userID")
	dropshipperID := userID_raw.(int64)

	// 2. --- Find the Cart ---
	var cartID int64
	err := h.DB.QueryRow("SELECT id FROM carts WHERE user_id = ?", dropshipperID).Scan(&cartID)
	if err != nil {
		if err == sql.ErrNoRows {
			// No cart exists. Return an empty cart response.
			c.JSON(http.StatusOK, gin.H{
				"items":      []CartItemResponse{},
				"subtotal":   0.0,
				"totalItems": 0,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find cart"})
		return
	}

	// 3. --- Query for Cart Items + Product Details ---
	// UPDATED: Query selects p.price_to_tts and p.stock_quantity
	query := `
		SELECT
			ci.product_id, p.name, p.sku, p.price_to_tts, ci.quantity, p.stock_quantity
		FROM cart_items ci
		JOIN products p ON ci.product_id = p.id
		WHERE ci.cart_id = ? AND p.status = 'active'
	`
	rows, err := h.DB.Query(query, cartID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query cart items"})
		return
	}
	defer rows.Close()

	// 4. --- Scan Rows and Calculate Totals ---
	var items []CartItemResponse
	var subtotal float64 = 0.0
	var totalItems int = 0

	var productSKU sql.NullString // Handle NULLable SKU

	for rows.Next() {
		var item CartItemResponse
		// UPDATED: Scan into item.Price and item.Stock
		if err := rows.Scan(
			&item.ProductID,
			&item.Name,
			&productSKU,
			&item.Price, // Scans 'price_to_tts' into 'Price'
			&item.Quantity,
			&item.Stock, // Scans 'stock_quantity' into 'Stock'
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan cart item"})
			return
		}

		item.SKU = productSKU.String // Convert sql.NullString to string
		item.LineTotal = item.Price * float64(item.Quantity)
		subtotal += item.LineTotal
		totalItems += item.Quantity

		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error iterating cart items"})
		return
	}

	// 5. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"items":      items,
		"subtotal":   subtotal,
		"totalItems": totalItems,
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
