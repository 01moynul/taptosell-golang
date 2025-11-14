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
type AddToCartInput struct {
	ProductID int64 `json:"productId" binding:"required"`
	Quantity  int   `json:"quantity" binding:"required,gt=0"` // Must add at least 1
}

// AddToCart is the handler for POST /v1/dropshipper/cart/items
// It adds a new item or updates the quantity of an existing item.
func (h *Handlers) AddToCart(c *gin.Context) {
	// 1. --- Get Dropshipper ID ---
	userID_raw, _ := c.Get("userID")
	dropshipperID := userID_raw.(int64)

	// 2. --- Bind & Validate JSON ---
	var input AddToCartInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Begin Transaction ---
	// All cart operations must be atomic.
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback() // Safety net

	// 4. --- Get or Create Cart ---
	cartID, err := h.getOrCreateCartID(tx, dropshipperID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get or create cart"})
		return
	}

	// 5. --- Check if item is already in cart ---
	var existingQuantity int
	checkQuery := "SELECT quantity FROM cart_items WHERE cart_id = ? AND product_id = ?"
	err = tx.QueryRow(checkQuery, cartID, input.ProductID).Scan(&existingQuantity)

	now := time.Now()
	if err == sql.ErrNoRows {
		// Case 1: New Item. Insert it.
		// First, check if the product is 'published' and has stock
		var stock int
		err := tx.QueryRow("SELECT stock FROM products WHERE id = ? AND status = 'published'", input.ProductID).Scan(&stock)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or is not published"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check product stock"})
			return
		}
		if stock < input.Quantity {
			c.JSON(http.StatusConflict, gin.H{"error": "Not enough stock available"})
			return
		}

		// Insert new cart item
		insertQuery := `
			INSERT INTO cart_items
			(cart_id, product_id, quantity, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)`
		_, err = tx.Exec(insertQuery, cartID, input.ProductID, input.Quantity, now, now)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add item to cart"})
			return
		}
	} else if err == nil {
		// Case 2: Existing Item. Update its quantity.
		newQuantity := existingQuantity + input.Quantity

		// Check stock for the *new* total quantity
		var stock int
		err := tx.QueryRow("SELECT stock FROM products WHERE id = ? AND status = 'published'", input.ProductID).Scan(&stock)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check product stock"})
			return
		}
		if stock < newQuantity {
			c.JSON(http.StatusConflict, gin.H{"error": "Not enough stock to add more items"})
			return
		}

		// Update existing cart item
		updateQuery := `
			UPDATE cart_items
			SET quantity = ?, updated_at = ?
			WHERE cart_id = ? AND product_id = ?`
		_, err = tx.Exec(updateQuery, newQuantity, now, cartID, input.ProductID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update item in cart"})
			return
		}
	} else {
		// Case 3: A real database error occurred
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check cart items"})
		return
	}

	// 6. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// 7. --- Send Success Response ---
	// We'll return the full cart in a moment via GetCart
	c.JSON(http.StatusCreated, gin.H{
		"message": "Item added to cart",
	})
}

// CartItemResponse is a helper struct for the GetCart handler
type CartItemResponse struct {
	ProductID int64   `json:"productId"`
	Name      string  `json:"name"`
	SKU       string  `json:"sku"`
	Price     float64 `json:"price"` // This is the 'TapToSell' price
	Quantity  int     `json:"quantity"`
	LineTotal float64 `json:"lineTotal"`
	Stock     int     `json:"stock"`
	// We'd add a product image URL here in a real app
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
	// This query JOINS cart_items with products to get all details.
	query := `
		SELECT
			ci.product_id, p.name, p.sku, p.price, ci.quantity, p.stock
		FROM cart_items ci
		JOIN products p ON ci.product_id = p.id
		WHERE ci.cart_id = ? AND p.status = 'published'
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
		if err := rows.Scan(
			&item.ProductID,
			&item.Name,
			&productSKU,
			&item.Price,
			&item.Quantity,
			&item.Stock,
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
