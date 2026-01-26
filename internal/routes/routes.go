package routes

import (
	"net/http"

	"github.com/01moynul/taptosell-golang/internal/handlers"
	"github.com/01moynul/taptosell-golang/internal/middleware"
	"github.com/gin-gonic/gin"
)

// --- NEW: Secure CORS Middleware ---
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

	// 1. SERVE UPLOADS STATICALLY (Accessible via http://localhost:8080/uploads/filename.jpg)
	// This line allows the browser to actually load the images you save.
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

		// --- NEW VERIFICATION ROUTES ---
		v1.POST("/auth/verify-email", h.VerifyEmail)
		v1.POST("/auth/resend-code", h.ResendVerificationEmail)

		// --- Public Product Routes ---
		v1.GET("/products/search", h.SearchProducts)

		// --- Category Routes ---
		v1.POST("/categories", h.CreateCategory)
		v1.GET("/categories", h.GetAllCategories)

		// --- Brand Routes ---
		v1.POST("/brands", h.CreateBrand)
		v1.GET("/brands", h.GetAllBrands)

		// --- Public Subscription Routes ---
		v1.GET("/subscriptions/plans", h.GetSubscriptionPlans)

		// --- Protected Routes (Login Required) ---
		auth := v1.Group("/")
		auth.Use(middleware.AuthMiddleware(h.DB))
		{
			// 2. FILE UPLOAD ROUTE (Protected)
			// Used by React to upload images/videos before submitting the product form.
			auth.POST("/upload", h.UploadFile)

			// Test Route
			auth.GET("/profile/me", func(c *gin.Context) {
				userID, _ := c.Get("userID")
				c.JSON(http.StatusOK, gin.H{
					"message":    "This is a protected route",
					"yourUserID": userID,
				})
			})

			// --- AI Chat Route ---
			auth.POST("/ai/chat", h.ChatAI)

			// --- Notification Routes ---
			auth.GET("/notifications", h.GetMyNotifications)
			auth.PATCH("/notifications/:id/read", h.MarkNotificationAsRead)

			// --- Supplier Documents ---
			auth.POST("/supplier/documents", h.UploadSupplierDocuments)

			// --- Product Routes ---
			auth.POST("/products", h.CreateProduct)
			auth.GET("/products/supplier/me", h.GetMyProducts)
			auth.GET("/products/:id", h.GetProduct)
			auth.PUT("/products/:id", h.UpdateProduct)
			auth.DELETE("/products/:id", h.DeleteProduct)

			// --- Supplier Wallet ---
			auth.GET("/supplier/wallet", h.GetSupplierWallet)
			auth.POST("/supplier/wallet/request-withdrawal", h.RequestWithdrawal)

			// --- Price Appeal ---
			auth.POST("/products/:id/request-price-change", h.RequestPriceChange)

			// --- Supplier Inventory ---
			supplierInventory := auth.Group("/supplier/inventory")
			{
				// [FIX] Use empty string "" instead of "/" to match the frontend URL exactly
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

			// Supplier Stats
			auth.GET("/supplier/dashboard-stats", h.GetSupplierStats)
		}

		// --- Manager-Only Routes ---
		manager := v1.Group("/manager")
		manager.Use(middleware.AuthMiddleware(h.DB))
		manager.Use(middleware.ManagerMiddleware(h.DB))
		{
			manager.GET("/products/pending", h.GetPendingProducts)
			manager.PATCH("/products/:id/approve", h.ApproveProduct)
			manager.PATCH("/products/:id/reject", h.RejectProduct)

			manager.GET("/withdrawal-requests", h.GetWithdrawalRequests)
			manager.PATCH("/withdrawal-requests/:id", h.ProcessWithdrawalRequest)

			manager.GET("/price-requests", h.GetPriceAppeals)
			manager.PATCH("/price-requests/:id", h.ProcessPriceAppeal)

			manager.GET("/settings", h.GetSettings)
			manager.PATCH("/settings", h.UpdateSettings)

			manager.POST("/users/:id/subscription", h.AssignSubscription)
			manager.GET("/dashboard-stats", h.GetManagerStats)
		}

		// --- Super Admin-Only Routes ---
		admin := v1.Group("/admin")
		admin.Use(middleware.AuthMiddleware(h.DB))
		admin.Use(middleware.SuperAdminMiddleware(h.DB))
		{
			admin.POST("/create-manager", h.CreateManager)
		}

		// --- Dropshipper-Only Routes ---
		dropshipper := v1.Group("/dropshipper")
		dropshipper.Use(middleware.AuthMiddleware(h.DB))
		dropshipper.Use(middleware.DropshipperMiddleware(h.DB))
		{
			dropshipper.GET("/cart", h.GetCart)
			dropshipper.POST("/cart/items", h.AddToCart)
			dropshipper.PUT("/cart/items/:product_id", h.UpdateCartItem)
			dropshipper.DELETE("/cart/items/:product_id", h.DeleteCartItem)

			dropshipper.GET("/wallet", h.GetMyWallet)
			dropshipper.POST("/checkout", h.Checkout)

			dropshipper.GET("/orders", h.GetMyOrders)
			dropshipper.GET("/orders/:id", h.GetOrderDetails)

			dropshipper.GET("/dashboard-stats", h.GetDropshipperStats)
		}
	}

	return router
}
