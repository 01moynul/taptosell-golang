package middleware

import (
	"database/sql"
	"net/http"

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
			// Use a generic error to avoid exposing user existence
			return "", &gin.Error{
				Err:  err,
				Type: gin.ErrorTypePublic,
				Meta: gin.H{"error": "Invalid user"},
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

// ManagerMiddleware now takes the DB connection as an argument
// and *returns* the gin.HandlerFunc.
func ManagerMiddleware(db *sql.DB) gin.HandlerFunc {
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
		role, err := queryUserRole(db, userID)
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

// SuperAdminMiddleware also takes the DB connection as an argument.
func SuperAdminMiddleware(db *sql.DB) gin.HandlerFunc {
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
		role, err := queryUserRole(db, userID)
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

// DropshipperMiddleware takes the DB connection as an argument.
// It returns a middleware that checks for 'dropshipper' role ONLY.
func DropshipperMiddleware(db *sql.DB) gin.HandlerFunc {
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
		role, err := queryUserRole(db, userID)
		if err != nil {
			gErr := err.(*gin.Error)
			c.JSON(http.StatusInternalServerError, gErr.Meta)
			c.Abort()
			return
		}

		// 3. Check permission
		if role != "dropshipper" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: Dropshipper role required"})
			c.Abort()
			return
		}

		// 4. Success! Add role to context and proceed.
		c.Set("userRole", role)
		c.Next()
	}
}
