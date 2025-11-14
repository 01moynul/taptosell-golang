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

		// --- Category Routes ---
		// TODO: Add manager protection to POST
		v1.POST("/categories", h.CreateCategory)
		v1.GET("/categories", h.GetAllCategories)

		// --- Brand Routes ---
		// TODO: Add manager protection to POST
		v1.POST("/brands", h.CreateBrand)
		v1.GET("/brands", h.GetAllBrands)

		// --- Protected Routes (Login Required) ---
		// We create a new group called 'auth'.
		// .Use(middleware.AuthMiddleware()) applies our "security guard"
		// to EVERY route defined inside this group.
		auth := v1.Group("/")
		auth.Use(middleware.AuthMiddleware()) // <--- APPLY THE "GUARD"
		{
			// Add a new test route: GET /v1/profile/me
			auth.GET("/profile/me", func(c *gin.Context) {
				// We can now get the 'userID' that the middleware set.
				userID, _ := c.Get("userID")

				c.JSON(http.StatusOK, gin.H{
					"message":    "This is a protected route",
					"yourUserID": userID,
				})
			})

			// --- NEW FILE UPLOAD ROUTE ---
			auth.POST("/supplier/documents", h.UploadSupplierDocuments)
		}
	}

	return router
}
