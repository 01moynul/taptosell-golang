package middleware

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/01moynul/taptosell-golang/internal/auth"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware creates a gin.HandlerFunc that acts as our "security guard".
// UPDATED: It now accepts 'db' to check for Maintenance Mode.
func AuthMiddleware(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. --- CHECK MAINTENANCE MODE ---
		var maintenanceMode string
		// We check the settings table. We ignore errors (defaults to empty string)
		// if the setting hasn't been created yet.
		_ = db.QueryRow("SELECT setting_value FROM settings WHERE setting_key = 'maintenance_mode'").Scan(&maintenanceMode)

		// 2. --- Get Authorization Header ---
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token format (must be Bearer)"})
			c.Abort()
			return
		}
		tokenString := parts[1]

		// 3. --- Validate Token ---
		userID, err := auth.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// 4. --- ENFORCE MAINTENANCE MODE ---
		// If maintenance is ON ("true"), only Administrators can pass.
		if maintenanceMode == "true" {
			var role string
			err := db.QueryRow("SELECT role FROM users WHERE id = ?", userID).Scan(&role)
			if err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Service unavailable (maintenance check failed)"})
				c.Abort()
				return
			}

			if role != "administrator" {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": "⛔ The system is currently in Maintenance Mode. Please try again later.",
				})
				c.Abort()
				return
			}
		}

		// 5. --- Success ---
		c.Set("userID", userID)
		c.Next()
	}
}
