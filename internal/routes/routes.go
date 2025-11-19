package routes

import (
	"net/http" // Add this

	"github.com/01moynul/taptosell-golang/internal/handlers"
	"github.com/01moynul/taptosell-golang/internal/middleware" // <-- ADD THIS IMPORT
	"github.com/gin-gonic/gin"
)

func SetupRouter(h *handlers.Handlers) *gin.Engine {
	router := gin.Default()

	v1 := router.Group("/v1")
	{
		// --- Ping Route (Public) ---
		v1.GET("/ping", func(c *gin.Context) {
			// ... (rest of ping code)
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
		// TODO: Add manager protection to POST
		v1.POST("/categories", h.CreateCategory)
		v1.GET("/categories", h.GetAllCategories)

		// --- Brand Routes ---
		// TODO: Add manager protection to POST
		v1.POST("/brands", h.CreateBrand)
		v1.GET("/brands", h.GetAllBrands)

		// --- Public Subscription Routes ---
		v1.GET("/subscriptions/plans", h.GetSubscriptionPlans)

		// --- Protected Routes (Login Required) ---
		// We create a new group called 'auth'.
		// .Use(middleware.AuthMiddleware()) applies our "security guard"
		// to EVERY route defined inside this group.
		auth := v1.Group("/")
		auth.Use(middleware.AuthMiddleware(h.DB)) // <--- APPLY THE "GUARD"
		{
			// Add a new test route: GET /v1/profile/me
			auth.GET("/profile/me", func(c *gin.Context) {
				userID, _ := c.Get("userID")
				c.JSON(http.StatusOK, gin.H{
					"message":    "This is a protected route",
					"yourUserID": userID,
				})
			})

			// --- AI Chat Route (NEW) ---
			// Available to all logged-in users.
			// The handler determines behavior based on the user's role.
			auth.POST("/ai/chat", h.ChatAI)

			// --- Notification Routes ---
			auth.GET("/notifications", h.GetMyNotifications)
			auth.PATCH("/notifications/:id/read", h.MarkNotificationAsRead)

			// --- NEW FILE UPLOAD ROUTE ---
			auth.POST("/supplier/documents", h.UploadSupplierDocuments)

			// --- NEW PRODUCT ROUTE ---
			auth.POST("/products", h.CreateProduct)
			auth.GET("/products/supplier/me", h.GetMyProducts)
			auth.PUT("/products/:id", h.UpdateProduct)
			auth.DELETE("/products/:id", h.DeleteProduct)

			// --- Supplier Wallet Routes ---
			auth.GET("/supplier/wallet", h.GetSupplierWallet)
			auth.POST("/supplier/wallet/request-withdrawal", h.RequestWithdrawal)

			// --- Price Appeal Route ---
			auth.POST("/products/:id/request-price-change", h.RequestPriceChange)

			// --- NEW: Supplier-Only Inventory Routes ---
			// We can re-use the AuthMiddleware but will add a Supplier-specific check later
			supplierInventory := auth.Group("/supplier/inventory")
			{
				// Item CRUD
				supplierInventory.POST("/", h.CreateInventoryItem)
				supplierInventory.GET("/", h.GetMyInventoryItems)
				supplierInventory.PUT("/:id", h.UpdateInventoryItem)
				supplierInventory.DELETE("/:id", h.DeleteInventoryItem)

				// --- Promote Route ---
				supplierInventory.POST("/:id/promote", h.PromoteInventoryItem)

				// Category CRUD (Simplified)
				supplierInventory.POST("/categories", h.CreateInventoryCategory)
				supplierInventory.GET("/categories", h.GetMyInventoryCategories)
				// We can add PUT/DELETE for categories later if needed

				// Brand CRUD (Simplified)
				supplierInventory.POST("/brands", h.CreateInventoryBrand)
				supplierInventory.GET("/brands", h.GetMyInventoryBrands)
				// We can add PUT/DELETE for brands later if needed
			}

		}

		// --- Manager-Only Routes (Login + Role Required) ---
		// This group is for 'manager' and 'administrator' roles
		manager := v1.Group("/manager")
		manager.Use(middleware.AuthMiddleware(h.DB))    // 1. Must be logged in
		manager.Use(middleware.ManagerMiddleware(h.DB)) // 2. Must be a Manager
		{
			// Product Approval Routes
			manager.GET("/products/pending", h.GetPendingProducts)
			manager.PATCH("/products/:id/approve", h.ApproveProduct)
			manager.PATCH("/products/:id/reject", h.RejectProduct)

			// Withdrawal Approval Routes
			manager.GET("/withdrawal-requests", h.GetWithdrawalRequests)
			manager.PATCH("/withdrawal-requests/:id", h.ProcessWithdrawalRequest)

			// Price Appeal Routes
			manager.GET("/price-requests", h.GetPriceAppeals)
			manager.PATCH("/price-requests/:id", h.ProcessPriceAppeal)

			// Settings Routes
			manager.GET("/settings", h.GetSettings)
			manager.PATCH("/settings", h.UpdateSettings)

			// User Subscription Management Routes
			manager.POST("/users/:id/subscription", h.AssignSubscription)
		}

		// --- Super Admin-Only Routes (Login + 'administrator' Role Required) ---
		admin := v1.Group("/admin")
		admin.Use(middleware.AuthMiddleware(h.DB))       // 1. Must be logged in
		admin.Use(middleware.SuperAdminMiddleware(h.DB)) // 2. Must be an Administrator
		{
			admin.POST("/create-manager", h.CreateManager)
		}

		// --- Dropshipper-Only Routes (Login + Role Required) ---
		// This group is for the 'dropshipper' role
		dropshipper := v1.Group("/dropshipper")
		dropshipper.Use(middleware.AuthMiddleware(h.DB))        // 1. Must be logged in
		dropshipper.Use(middleware.DropshipperMiddleware(h.DB)) // 2. Must be a Dropshipper
		{
			// Cart Routes
			dropshipper.GET("/cart", h.GetCart)
			dropshipper.POST("/cart/items", h.AddToCart)
			dropshipper.PUT("/cart/items/:product_id", h.UpdateCartItem)
			dropshipper.DELETE("/cart/items/:product_id", h.DeleteCartItem)

			// Wallet Route
			dropshipper.GET("/wallet", h.GetMyWallet)

			// Checkout Route
			dropshipper.POST("/checkout", h.Checkout)
		}
	}

	return router
}
