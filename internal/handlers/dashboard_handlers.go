package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

//
// --- Dropshipper Dashboard Stats ---
//

type DropshipperStats struct {
	WalletBalance    float64 `json:"walletBalance"`
	ProcessingOrders int     `json:"processingOrders"`
	ActionRequired   int     `json:"actionRequired"` // Count of 'on-hold' orders
}

// GetDropshipperStats returns KPI data for the dropshipper dashboard
// GET /v1/dropshipper/dashboard-stats
func (h *Handlers) GetDropshipperStats(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)

	stats := DropshipperStats{}

	// 1. Wallet Balance
	balance, err := h.GetWalletBalance(h.DB, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get wallet balance"})
		return
	}
	stats.WalletBalance = balance

	// 2. Processing Orders Count
	err = h.DB.QueryRow("SELECT COUNT(*) FROM orders WHERE user_id = ? AND status = 'processing'", userID).Scan(&stats.ProcessingOrders)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count processing orders"})
		return
	}

	// 3. Action Required (On-Hold) Count
	err = h.DB.QueryRow("SELECT COUNT(*) FROM orders WHERE user_id = ? AND status = 'on-hold'", userID).Scan(&stats.ActionRequired)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count on-hold orders"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

//
// --- Supplier Dashboard Stats ---
//

type SupplierStats struct {
	// Private Inventory KPIs (Tab A)
	TotalValuation float64 `json:"totalValuation"`
	LowStockCount  int     `json:"lowStockCount"`

	// Marketplace KPIs (Tab B)
	AvailableBalance float64 `json:"availableBalance"`
	PendingBalance   float64 `json:"pendingBalance"`
	LiveProducts     int     `json:"liveProducts"`
	UnderReview      int     `json:"underReview"`
}

// GetSupplierStats returns KPI data for the supplier dashboard
// GET /v1/supplier/dashboard-stats
func (h *Handlers) GetSupplierStats(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)

	stats := SupplierStats{}

	// 1. Private Inventory Valuation (Sum of Cost * Stock)
	// We use COALESCE(..., 0) to ensure we return 0 instead of NULL if the table is empty
	queryValuation := `
		SELECT COALESCE(SUM(cost_price * stock_quantity), 0)
		FROM inventory_items
		WHERE user_id = ?
	`
	err := h.DB.QueryRow(queryValuation, supplierID).Scan(&stats.TotalValuation)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to calculate valuation"})
		return
	}

	// 2. Low Stock Count (< 10)
	queryLowStock := `
		SELECT COUNT(*)
		FROM inventory_items
		WHERE user_id = ? AND stock_quantity < 10
	`
	err = h.DB.QueryRow(queryLowStock, supplierID).Scan(&stats.LowStockCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count low stock"})
		return
	}

	// 3. Wallet: Available Balance
	stats.AvailableBalance, err = h.GetWalletBalance(h.DB, supplierID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get wallet balance"})
		return
	}

	// 4. Wallet: Pending Balance
	// Logic matches 'withdrawal_handlers.go': Sum of items in 'shipped' orders
	queryPending := `
		SELECT COALESCE(SUM(oi.unit_price * oi.quantity), 0)
		FROM order_items oi
		JOIN orders o ON oi.order_id = o.id
		JOIN products p ON oi.product_id = p.id
		WHERE p.supplier_id = ? AND o.status = 'shipped'
	`
	err = h.DB.QueryRow(queryPending, supplierID).Scan(&stats.PendingBalance)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get pending balance"})
		return
	}

	// 5. Marketplace Product Counts
	err = h.DB.QueryRow("SELECT COUNT(*) FROM products WHERE supplier_id = ? AND status = 'active'", supplierID).Scan(&stats.LiveProducts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count live products"})
		return
	}

	err = h.DB.QueryRow("SELECT COUNT(*) FROM products WHERE supplier_id = ? AND status = 'pending'", supplierID).Scan(&stats.UnderReview)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count pending products"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// --- Manager Dashboard Stats ---

// Updated to include TotalUsers as per Phase 8.7 plan
type ManagerStats struct {
	PendingProducts    int `json:"pendingProducts"`
	WithdrawalRequests int `json:"withdrawalRequests"`
	PriceAppeals       int `json:"priceAppeals"`
	TotalUsers         int `json:"totalUsers"` // [NEW] Track platform growth
}

// GetManagerStats returns KPI data for the manager dashboard
// GET /v1/manager/dashboard-stats
func (h *Handlers) GetManagerStats(c *gin.Context) {
	stats := ManagerStats{}

	// 1. Pending Products
	err := h.DB.QueryRow("SELECT COUNT(*) FROM products WHERE status = 'pending'").Scan(&stats.PendingProducts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count pending products"})
		return
	}

	// 2. Pending Withdrawal Requests
	err = h.DB.QueryRow("SELECT COUNT(*) FROM withdrawal_requests WHERE status = 'pending'").Scan(&stats.WithdrawalRequests)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count withdrawal requests"})
		return
	}

	// 3. Pending Price Appeals
	err = h.DB.QueryRow("SELECT COUNT(*) FROM price_appeals WHERE status = 'pending'").Scan(&stats.PriceAppeals)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count price appeals"})
		return
	}

	// 4. Total Active Users (Dropshippers + Suppliers)
	// [NEW] We count only active users to give a realistic view of the user base
	err = h.DB.QueryRow("SELECT COUNT(*) FROM users WHERE status = 'active'").Scan(&stats.TotalUsers)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count users"})
		return
	}

	c.JSON(http.StatusOK, stats)
}
