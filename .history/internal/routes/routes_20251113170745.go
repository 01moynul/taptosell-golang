package routes

import (
	"net/http"

	"github.com/01moynul/taptosell-golang/internal/handlers" // Import our new handlers
	// <-- ADD THIS IMPORT
	"github.com/gin-gonic/gin"
)

// SetupRouter creates the Gin router and defines all our API routes.
// It takes our 'Handlers' struct as a dependency.
func SetupRouter(h *handlers.Handlers) *gin.Engine {
	// 1. Create a new Gin router
	router := gin.Default()

	// 2. Define our API route groups
	v1 := router.Group("/v1")
	{
		// --- Ping Route ---
		// We'll move the 'ping' logic here for better organization
		v1.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message": "pong! The TapToSell v2 API is running.",
			})
		})

		// --- User Routes ---
		// This is our new registration route.
		// It maps the HTTP 'POST' method on '/v1/register/dropshipper'
		// to our new 'h.RegisterDropshipper' handler.
		v1.POST("/register/dropshipper", h.RegisterDropshipper)
		v1.POST("/register/supplier", h.RegisterSupplier)
		v1.POST("/login", h.Login)

		// We will add /v1/register/supplier, /v1/login, etc. here later.
	}

	// 3. Return the fully configured router
	return router
}
