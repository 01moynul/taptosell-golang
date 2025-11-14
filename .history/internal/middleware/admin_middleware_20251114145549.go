package middleware

import (
	"database/sql"
	"net/http"

	"github.com/01moynul/taptosell-golang/internal/handlers"
	"github.com/gin-gonic/gin"
)

//
// --- Role-Based Middleware ---
//
// These middleware functions are designed to be USED *AFTER*
// the main AuthMiddleware(). They read the 'userID' from the context,
// query the DB for that user's role, and then enforce role permissions.
//

// queryUserRole is a helper to get the user's role from the DB.
func queryUserRole(db *sql.DB, userID int64) (string, error) {
	var role string
	query := "SELECT role FROM users WHERE id = ?"
	err := db.QueryRow(query, userID).Scan(&role)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", &gin.Error{
				Err:  err,
				Type: gin.ErrorTypePublic,
				Meta: gin.H{"error": "User not found"},
			}
		}
		return "", &gin.Error{
			Err:  err,
			Type: gin.ErrorTypePrivate,
			Meta: gin.H{"error": "Database error checking role"},
		}
	}
	return role, nil
}

// ManagerMiddleware wraps the Handlers to get DB access.
// It returns a middleware that checks for 'manager' or 'administrator' roles.
func (h *handlers.Handlers) ManagerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Get userID from AuthMiddleware
		userID_raw, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in context (AuthMiddleware must run first)"})
			c.Abort()
			return
		}
		userID := userID_raw.(int64)

		// 2. Query DB for user's role
		role, err := queryUserRole(h.DB, userID)
		if err != nil {
			gErr := err.(*gin.Error)
			c.JSON(http.StatusInternalServerError, gErr.Meta)
			c.Abort()
			return
		}

		// 3. Check permission
		if role != "manager" && role != "administrator" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: Manager or Admin role required"})
			c.Abort()
			return
		}

		// 4. Success! Add role to context and proceed.
		c.Set("userRole", role)
		c.Next()
	}
}

// SuperAdminMiddleware wraps the Handlers to get DB access.
// It returns a middleware that checks for 'administrator' role ONLY.
func (h *handlers.Handlers) SuperAdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Get userID from AuthMiddleware
		userID_raw, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in context (AuthMiddleware must run first)"})
			c.Abort()
			return
		}
		userID := userID_raw.(int64)

		// 2. Query DB for user's role
		role, err := queryUserRole(h.DB, userID)
		if err != nil {
			gErr := err.(*gin.Error)
			c.JSON(http.StatusInternalServerError, gErr.Meta)
			c.Abort()
			return
		}

		// 3. Check permission
		if role != "administrator" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: Super Admin role required"})
			c.Abort()
			return
		}

		// 4. Success! Add role to context and proceed.
		c.Set("userRole", role)
		c.Next()
	}
}
