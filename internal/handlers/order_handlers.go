package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/01moynul/taptosell-golang/internal/models" // <-- Added this import
	"github.com/gin-gonic/gin"
)

//
// --- Order Handlers (Dropshipper-Only) ---
//

// CartItemData is a helper struct for fetching cart items during checkout
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
	query := `
		SELECT ci.product_id, ci.quantity, p.price_to_tts, p.stock_quantity
		FROM cart_items ci
		JOIN products p ON ci.product_id = p.id
		WHERE ci.cart_id = ? AND p.status = 'published'
		FOR UPDATE
	`

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
		if err := rows.Scan(&item.ProductID, &item.Quantity, &item.Price, &item.Stock); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan cart item"})
			return
		}

		// 4. --- Check Stock & Calculate Total ---
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

	stockQuery := "UPDATE products SET stock_quantity = stock_quantity - ? WHERE id = ?"

	for _, item := range cartItems {
		// a. Snapshot the item into order_items
		_, err := tx.Exec(itemQuery, orderID, item.ProductID, item.Quantity, item.Price, now)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save order item"})
			return
		}

		// b. If the order is paid, deduct stock
		if orderStatus == "processing" {
			_, err := tx.Exec(stockQuery, item.Quantity, item.ProductID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to deduct stock"})
				return
			}
		}
	}

	// c. If the order is paid, deduct from wallet
	if orderStatus == "processing" {
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
		"totalPaid": totalOrderCost,
	})
}

//
// --- NEW: Order Retrieval Handlers ---
//

// OrderItemDetail extends the base OrderItem to include Product info
type OrderItemDetail struct {
	models.OrderItem
	ProductName string `json:"productName"`
	ProductSKU  string `json:"productSku"`
}

// GetMyOrders is the handler for GET /v1/dropshipper/orders
func (h *Handlers) GetMyOrders(c *gin.Context) {
	// 1. --- Get Dropshipper ID ---
	userID_raw, _ := c.Get("userID")
	dropshipperID := userID_raw.(int64)

	// 2. --- Query Orders ---
	query := `
		SELECT id, user_id, status, total, created_at, updated_at, tracking 
		FROM orders 
		WHERE user_id = ? 
		ORDER BY created_at DESC
	`

	rows, err := h.DB.Query(query, dropshipperID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch orders"})
		return
	}
	defer rows.Close()

	// 3. --- Scan Rows ---
	var orders []models.Order
	for rows.Next() {
		var o models.Order
		var tracking sql.NullString

		if err := rows.Scan(&o.ID, &o.UserID, &o.Status, &o.Total, &o.CreatedAt, &o.UpdatedAt, &tracking); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan order data"})
			return
		}
		o.Tracking = tracking
		orders = append(orders, o)
	}

	// 4. --- Return Response ---
	if orders == nil {
		orders = []models.Order{}
	}

	c.JSON(http.StatusOK, gin.H{
		"orders": orders,
	})
}

// GetOrderDetails is the handler for GET /v1/dropshipper/orders/:id
func (h *Handlers) GetOrderDetails(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	dropshipperID := userID_raw.(int64)
	orderID := c.Param("id")

	// 2. --- Fetch Order & Verify Ownership ---
	var o models.Order
	var tracking sql.NullString

	queryOrder := `
		SELECT id, user_id, status, total, created_at, updated_at, tracking 
		FROM orders 
		WHERE id = ? AND user_id = ?
	`
	err := h.DB.QueryRow(queryOrder, orderID, dropshipperID).Scan(
		&o.ID, &o.UserID, &o.Status, &o.Total, &o.CreatedAt, &o.UpdatedAt, &tracking,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch order"})
		return
	}
	o.Tracking = tracking

	// 3. --- Fetch Order Items with Product Details ---
	queryItems := `
		SELECT 
			oi.id, oi.order_id, oi.product_id, oi.quantity, oi.unit_price, oi.created_at,
			p.name, p.sku
		FROM order_items oi
		JOIN products p ON oi.product_id = p.id
		WHERE oi.order_id = ?
	`

	rows, err := h.DB.Query(queryItems, o.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch order items"})
		return
	}
	defer rows.Close()

	var items []OrderItemDetail
	for rows.Next() {
		var item OrderItemDetail
		if err := rows.Scan(
			&item.ID, &item.OrderID, &item.ProductID, &item.Quantity, &item.UnitPrice, &item.CreatedAt,
			&item.ProductName, &item.ProductSKU,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan order item"})
			return
		}
		items = append(items, item)
	}

	// 4. --- Return Combined Response ---
	if items == nil {
		items = []OrderItemDetail{}
	}

	c.JSON(http.StatusOK, gin.H{
		"order": o,
		"items": items,
	})
}

// internal/handlers/order_handlers.go

// PayOrder handles the payment for an existing "on-hold" order.
// Route: POST /v1/dropshipper/orders/:id/pay
func (h *Handlers) PayOrder(c *gin.Context) {
	// 1. Get IDs
	userID_raw, _ := c.Get("userID")
	dropshipperID := userID_raw.(int64)
	orderID := c.Param("id")

	// 2. Begin Transaction
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// 3. Fetch Order Details (Locking the row to prevent double payment)
	var totalAmount float64
	var status string
	queryOrder := "SELECT total, status FROM orders WHERE id = ? AND user_id = ? FOR UPDATE"
	err = tx.QueryRow(queryOrder, orderID, dropshipperID).Scan(&totalAmount, &status)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch order"})
		return
	}

	if status != "on-hold" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Order is not in 'on-hold' status"})
		return
	}

	// 4. Check Wallet Balance
	balance, err := h.GetWalletBalance(tx, dropshipperID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check wallet balance"})
		return
	}

	if balance < totalAmount {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "Insufficient wallet balance"})
		return
	}

	// 5. Fetch Order Items to Check & Deduct Stock
	// (Since stock wasn't deducted when the order was placed as 'on-hold', we must do it now)
	type ItemStockCheck struct {
		ProductID int64
		Quantity  int
		Stock     int
	}
	queryItems := `
		SELECT oi.product_id, oi.quantity, p.stock_quantity
		FROM order_items oi
		JOIN products p ON oi.product_id = p.id
		WHERE oi.order_id = ?
	`
	rows, err := tx.Query(queryItems, orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch order items"})
		return
	}
	defer rows.Close()

	var itemsToCheck []ItemStockCheck
	for rows.Next() {
		var item ItemStockCheck
		if err := rows.Scan(&item.ProductID, &item.Quantity, &item.Stock); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan item"})
			return
		}
		itemsToCheck = append(itemsToCheck, item)
	}

	// Check if items are still in stock
	for _, item := range itemsToCheck {
		if item.Stock < item.Quantity {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Product ID %d is now out of stock", item.ProductID)})
			return
		}
	}

	// 6. Execute Updates

	// A. Deduct Stock
	updateStock := "UPDATE products SET stock_quantity = stock_quantity - ? WHERE id = ?"
	for _, item := range itemsToCheck {
		if _, err := tx.Exec(updateStock, item.Quantity, item.ProductID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update stock"})
			return
		}
	}

	// B. Deduct Wallet
	err = h.AddWalletTransaction(tx, dropshipperID, "order_payment", -totalAmount, fmt.Sprintf("Payment for Order #%s", orderID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process wallet deduction"})
		return
	}

	// C. Update Order Status
	_, err = tx.Exec("UPDATE orders SET status = 'processing', updated_at = ? WHERE id = ?", time.Now(), orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update order status"})
		return
	}

	// 7. Commit
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Payment successful",
		"new_status": "processing",
	})
}
