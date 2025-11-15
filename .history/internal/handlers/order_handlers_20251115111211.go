package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

//
// --- Order Handlers (Dropshipper-Only) ---
//

// CartItemData is a helper struct for fetching cart items during checkout
// NO CHANGE: Internal struct names are fine.
type CartItemData struct {
	ProductID int64
	Quantity  int
	Price     float64 // The *current* price from the products table
	Stock     int
}

// Checkout is the handler for POST /v1/dropshipper/checkout
func (h *Handlers) Checkout(c *gin.Context) {
	// 1. --- Get Dropshipper ID ---
	userID_raw, _ := c.Get("userID")
	dropshipperID := userID_raw.(int64)

	// 2. --- Begin Transaction ---
	tx, err := h.DB.BeginTx(c, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback() // Safety net

	// 3. --- Get User's Cart & Items ---
	var cartID int64
	err = tx.QueryRow("SELECT id FROM carts WHERE user_id = ?", dropshipperID).Scan(&cartID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Your cart is empty"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find cart"})
		return
	}

	// Get all items in the cart AND *lock* the product rows for this transaction
	// UPDATED: Query selects p.price_to_tts and p.stock_quantity
	query := `
		SELECT ci.product_id, ci.quantity, p.price_to_tts, p.stock_quantity
		FROM cart_items ci
		JOIN products p ON ci.product_id = p.id
		WHERE ci.cart_id = ? AND p.status = 'published'
		FOR UPDATE
	` // 'FOR UPDATE' locks the selected 'products' rows

	rows, err := tx.Query(query, cartID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get cart items"})
		return
	}
	defer rows.Close()

	var cartItems []CartItemData
	var totalOrderCost float64 = 0.0

	for rows.Next() {
		var item CartItemData
		// UPDATED: Scan order is correct, scans new cols into struct fields
		if err := rows.Scan(&item.ProductID, &item.Quantity, &item.Price, &item.Stock); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan cart item"})
			return
		}

		// 4. --- Check Stock & Calculate Total ---
		// NO CHANGE: Logic is correct, 'item.Stock' now holds 'stock_quantity'
		if item.Stock < item.Quantity {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Not enough stock for product ID %d", item.ProductID)})
			return
		}
		totalOrderCost += item.Price * float64(item.Quantity)
		cartItems = append(cartItems, item)
	}

	if len(cartItems) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Your cart contains no published products"})
		return
	}

	// 5. --- Check Wallet Balance ---
	// NO CHANGE: This helper function is separate
	var balance sql.NullFloat64
	err = tx.QueryRow("SELECT SUM(amount) FROM wallet_transactions WHERE user_id = ?", dropshipperID).Scan(&balance)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get wallet balance"})
		return
	}
	walletBalance := balance.Float64 // Will be 0.0 if NULL

	// 6. --- Create Order & Process Payment ---
	now := time.Now()
	var orderStatus string

	if walletBalance < totalOrderCost {
		orderStatus = "on-hold"
	} else {
		orderStatus = "processing"
	}

	// Insert the main order record
	orderQuery := `
		INSERT INTO orders (user_id, status, total, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`
	result, err := tx.Exec(orderQuery, dropshipperID, orderStatus, totalOrderCost, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create order"})
		return
	}
	orderID, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get new order ID"})
		return
	}

	// 7. --- Create Order Items & Update Wallet/Stock ---
	itemQuery := `
		INSERT INTO order_items (order_id, product_id, quantity, unit_price, created_at)
		VALUES (?, ?, ?, ?, ?)`

	// UPDATED: Stock deduction query uses stock_quantity
	stockQuery := "UPDATE products SET stock_quantity = stock_quantity - ? WHERE id = ?"

	for _, item := range cartItems {
		// a. Snapshot the item into order_items
		// NO CHANGE: item.Price holds the correct 'price_to_tts'
		_, err := tx.Exec(itemQuery, orderID, item.ProductID, item.Quantity, item.Price, now)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save order item"})
			return
		}

		// b. If the order is paid, deduct stock
		if orderStatus == "processing" {
			// NO CHANGE: Logic is correct
			_, err := tx.Exec(stockQuery, item.Quantity, item.ProductID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to deduct stock"})
				return
			}
		}
	}

	// c. If the order is paid, deduct from wallet
	if orderStatus == "processing" {
		// NO CHANGE: This helper function is separate
		err = h.AddWalletTransaction(tx, dropshipperID, "order", -totalOrderCost, fmt.Sprintf("Payment for Order ID %d", orderID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to deduct from wallet"})
			return
		}
	}

	// 8. --- Clear the Cart ---
	_, err = tx.Exec("DELETE FROM cart_items WHERE cart_id = ?", cartID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear cart"})
		return
	}

	// 9. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit final transaction"})
		return
	}

	// 10. --- Send Success Response ---
	c.JSON(http.StatusCreated, gin.H{
		"message":   fmt.Sprintf("Order created successfully with status: %s", orderStatus),
		"orderId":   orderID,
		"status":    orderStatus,
		"totalPaid": totalOrderCost, // This is the total cost to the dropshipper
	})
}
