package routes

import (
	"net/http"

	"github.com/01moynul/taptosell-golang/internal/handlers"
	"github.com/01moynul/taptosell-golang/internal/middleware"
	"github.com/gin-gonic/gin"
)

// --- Secure CORS Middleware ---
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func SetupRouter(h *handlers.Handlers) *gin.Engine {
	router := gin.Default()

	// --- APPLY THE CORS GUARD ---
	router.Use(CORSMiddleware())

	// 1. SERVE UPLOADS STATICALLY
	router.Static("/uploads", "./uploads")

	v1 := router.Group("/v1")
	{
		// --- Ping Route (Public) ---
		v1.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "pong!"})
		})

		// --- Auth Routes (Public) ---
		v1.POST("/register/dropshipper", h.RegisterDropshipper)
		v1.POST("/register/supplier", h.RegisterSupplier)
		v1.POST("/login", h.Login)
		v1.POST("/auth/verify-email", h.VerifyEmail)
		v1.POST("/auth/resend-code", h.ResendVerificationEmail)

		// --- Public Product Data ---
		v1.GET("/products/search", h.SearchProducts)
		v1.GET("/categories", h.GetAllCategories) // Public Read
		v1.GET("/brands", h.GetAllBrands)         // Public Read
		v1.GET("/subscriptions/plans", h.GetSubscriptionPlans)

		// --- Protected Routes (Login Required) ---
		auth := v1.Group("/")
		auth.Use(middleware.AuthMiddleware(h.DB))
		{
			auth.POST("/upload", h.UploadFile)
			auth.GET("/profile/me", func(c *gin.Context) {
				userID, _ := c.Get("userID")
				c.JSON(http.StatusOK, gin.H{"message": "This is a protected route", "yourUserID": userID})
			})

			// AI Chat
			auth.POST("/ai/chat", h.ChatAI)

			// Notifications
			auth.GET("/notifications", h.GetMyNotifications)
			auth.PATCH("/notifications/:id/read", h.MarkNotificationAsRead)

			// Supplier
			auth.POST("/supplier/documents", h.UploadSupplierDocuments)
			auth.POST("/products", h.CreateProduct)
			auth.GET("/products/supplier/me", h.GetMyProducts)
			auth.GET("/products/:id", h.GetProduct)
			auth.PUT("/products/:id", h.UpdateProduct)
			auth.DELETE("/products/:id", h.DeleteProduct)

			// Supplier Wallet
			auth.GET("/supplier/wallet", h.GetSupplierWallet)
			auth.POST("/supplier/wallet/request-withdrawal", h.RequestWithdrawal)
			auth.POST("/products/:id/request-price-change", h.RequestPriceChange)

			// [NEW] Supplier Order Fulfillment
			// This route allows suppliers to fulfill orders containing their items
			auth.PATCH("/supplier/orders/:id/ship", h.UpdateOrderTracking)

			// Supplier Inventory
			supplierInventory := auth.Group("/supplier/inventory")
			{
				supplierInventory.POST("", h.CreateInventoryItem)
				supplierInventory.GET("", h.GetMyInventoryItems)
				supplierInventory.PUT("/:id", h.UpdateInventoryItem)
				supplierInventory.DELETE("/:id", h.DeleteInventoryItem)
				supplierInventory.POST("/:id/promote", h.PromoteInventoryItem)
				supplierInventory.POST("/categories", h.CreateInventoryCategory)
				supplierInventory.GET("/categories", h.GetMyInventoryCategories)
				supplierInventory.POST("/brands", h.CreateInventoryBrand)
				supplierInventory.GET("/brands", h.GetMyInventoryBrands)
			}
			auth.GET("/supplier/dashboard-stats", h.GetSupplierStats)
			auth.GET("/supplier/orders", h.GetSupplierSales)
			auth.GET("/supplier/orders/:id", h.GetSupplierOrderDetails)
		}

		// --- Manager-Only Routes ---
		manager := v1.Group("/manager")
		manager.Use(middleware.AuthMiddleware(h.DB))
		manager.Use(middleware.ManagerMiddleware(h.DB))
		{
			// Dashboard Stats
			manager.GET("/dashboard-stats", h.GetManagerStats)

			// Global Taxonomy Management (Moved here for security)
			manager.POST("/categories", h.CreateCategory)
			manager.DELETE("/categories/:id", h.DeleteCategory) // NEW
			manager.POST("/brands", h.CreateBrand)
			manager.DELETE("/brands/:id", h.DeleteBrand) // NEW

			// Approvals
			manager.GET("/products/pending", h.GetPendingProducts)
			manager.PATCH("/products/:id/approve", h.ApproveProduct)
			manager.PATCH("/products/:id/reject", h.RejectProduct)

			manager.GET("/withdrawal-requests", h.GetWithdrawalRequests)
			manager.PATCH("/withdrawal-requests/:id", h.ProcessWithdrawalRequest)

			manager.GET("/price-requests", h.GetPriceAppeals)
			manager.PATCH("/price-requests/:id", h.ProcessPriceAppeal)

			// Users & Settings
			manager.GET("/settings", h.GetSettings)
			manager.PATCH("/settings", h.UpdateSettings)
			manager.GET("/users", h.GetUsers)
			manager.PATCH("/users/:id/penalty", h.UpdateUserPenalty)
			manager.POST("/users/:id/subscription", h.AssignSubscription)
		}

		// --- Super Admin ---
		admin := v1.Group("/admin")
		admin.Use(middleware.AuthMiddleware(h.DB))
		admin.Use(middleware.SuperAdminMiddleware(h.DB))
		{
			admin.POST("/create-manager", h.CreateManager)
		}

		// --- Dropshipper ---
		dropshipper := v1.Group("/dropshipper")
		dropshipper.Use(middleware.AuthMiddleware(h.DB))
		dropshipper.Use(middleware.DropshipperMiddleware(h.DB))
		{
			dropshipper.GET("/cart", h.GetCart)
			dropshipper.POST("/cart/items", h.AddToCart)
			dropshipper.PUT("/cart/items/:product_id", h.UpdateCartItem)
			dropshipper.DELETE("/cart/items/:product_id", h.DeleteCartItem)
			dropshipper.GET("/wallet", h.GetMyWallet)
			dropshipper.POST("/wallet/topup", h.ManualTopUp)
			dropshipper.POST("/checkout", h.Checkout)
			dropshipper.GET("/orders", h.GetMyOrders)
			dropshipper.GET("/orders/:id", h.GetOrderDetails)
			dropshipper.GET("/dashboard-stats", h.GetDropshipperStats)
			dropshipper.POST("/orders/:id/pay", h.PayOrder)
			// ✅ ADD THIS LINE:
			dropshipper.POST("/orders/:id/complete", h.CompleteOrder)
		}
	}

	return router
}
