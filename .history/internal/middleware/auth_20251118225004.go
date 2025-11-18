package middleware

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/01moynul/taptosell-golang/internal/auth"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware creates a gin.HandlerFunc that acts as our "security guard".
// Updated for Phase 6.8: It now accepts a database connection to check maintenance mode.
func AuthMiddleware(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. --- CHECK MAINTENANCE MODE ---
		// We check the settings table first.
		var maintenanceMode string
		query := "SELECT setting_value FROM settings WHERE setting_key = 'maintenance_mode'"
		err := db.QueryRow(query).Scan(&maintenanceMode)

		// If the setting doesn't exist yet (db migration pending), we treat it as "false".
		if err != nil && err != sql.ErrNoRows {
			// Log error but don't crash, assume safe to proceed
		}

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

		// 4. --- CHECK USER ROLE (If Maintenance Mode is ON) ---
		if maintenanceMode == "true" {
			// We need to fetch the user's role to see if they are an Administrator
			var role string
			roleQuery := "SELECT role FROM users WHERE id = ?"
			err := db.QueryRow(roleQuery, userID).Scan(&role)

			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify user role during maintenance"})
				c.Abort()
				return
			}

			// Only 'administrator' can bypass maintenance mode.
			// Even 'managers' are blocked to ensure data consistency during updates.
			if role != "administrator" {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": "System is currently in Maintenance Mode. Please try again later.",
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
