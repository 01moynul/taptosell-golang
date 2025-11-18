package middleware

import (
	"net/http"
	"strings"

	"github.com/01moynul/taptosell-golang/internal/auth" // Import our auth package
	"github.com/gin-gonic/gin"
)

// AuthMiddleware creates a gin.HandlerFunc that acts as our "security guard".
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Get the "Authorization" header from the request.
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort() // Stop the request
			return
		}

		// 2. The header should be in the format "Bearer [token]".
		// We split the string to get just the token part.
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token format (must be Bearer)"})
			c.Abort()
			return
		}
		tokenString := parts[1]

		// 3. Validate the token using our new function.
		userID, err := auth.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// 4. Success! The passport is valid.
		// We add the 'userID' to the request "context" so our
		// next handler can know *who* is making the request.
		c.Set("userID", userID)

		// 5. Let the request proceed to the final handler.
		c.Next()
	}
}
