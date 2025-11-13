package handlers

import (
	"database/sql"
	"net/http"
	"time" // We'll need this for 'created_at'

	"github.com/01moynul/taptosell-golang/internal/models" // Import our models
	"github.com/gin-gonic/gin"
)

// Handlers struct (as before)
type Handlers struct {
	DB *sql.DB
}

// --- User Registration ---

// We define a struct to hold the *input* from the user.
// This is separate from our main 'models.User' struct because
// we don't want to accept an 'id' or 'status' from the user.
type RegisterUserInput struct {
	FullName    string `json:"fullName" binding:"required"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	PhoneNumber string `json:"phoneNumber" binding:"required"`
}

// RegisterDropshipper is the handler for our new endpoint.
func (h *Handlers) RegisterDropshipper(c *gin.Context) {
	// 1. --- Define Input ---
	var input RegisterUserInput

	// 2. --- Bind & Validate JSON ---
	// 'c.ShouldBindJSON' tries to map the incoming JSON
	// to our 'input' struct. It also validates the 'binding' tags.
	if err := c.ShouldBindJSON(&input); err != nil {
		// If validation fails (e.g., password < 8 chars),
		// send a 400 Bad Request error.
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Create User Model ---
	// We are now safe to build our 'models.User'
	user := &models.User{
		Role:        "dropshipper", // Set the role
		Status:      "pending",     // All new users are 'pending'
		Email:       input.Email,
		FullName:    input.FullName,
		PhoneNumber: input.PhoneNumber,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Version:     1,
	}

	// 4. --- Hash the Password ---
	// Use the 'Set' method from our 'models/user_types.go'
	var password models.Password
	if err := password.Set(input.Password); err != nil {
		// If hashing fails (rare), send a server error.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}
	user.PasswordHash = password.Hash // Set the hash on our user model

	// 5. --- Save to Database (Placeholder) ---
	// This is our next step. For now, we'll just check
	// that everything else worked.

	// 6. --- Send Success Response ---
	// We send a 201 Created status and the new user object (as JSON).
	// Gin automatically respects the 'json:"-"' tags on the sensitive fields!
	c.JSON(http.StatusCreated, gin.H{
		"message": "Dropshipper registered successfully, pending approval.",
		"user":    user,
	})
}
