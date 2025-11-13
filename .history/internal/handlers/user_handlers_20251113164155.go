package handlers

import (
	"database/sql" // For our DB connection
	"net/http"     // For HTTP status codes
	"time"         // For time.Now()

	"github.com/01moynul/taptosell-golang/internal/models" // Our User models
	"github.com/gin-gonic/gin"                             // The Gin framework
)

// Handlers struct holds our database connection pool
type Handlers struct {
	DB *sql.DB
}

// --- User Registration ---

// RegisterUserInput defines the expected JSON data for registration.
// The 'binding' tags are used by Gin for automatic validation.
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
	// c.ShouldBindJSON checks the JSON and runs our validation rules.
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Create User Model ---
	// Build our database model from the validated input.
	user := &models.User{
		Role:        "dropshipper",
		Status:      "pending", // New users always start as pending
		Email:       input.Email,
		FullName:    input.FullName,
		PhoneNumber: input.PhoneNumber,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Version:     1,
	}

	// 4. --- Hash the Password ---
	// Use the secure 'Set' method from our user_types.go
	var password models.Password
	if err := password.Set(input.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}
	user.PasswordHash = password.Hash // Store the hash

	// 5. --- Save to Database ---
	// This is the SQL logic we just added.
	query := `
		INSERT INTO users
		(role, status, email, password_hash, full_name, phone_number, created_at, updated_at, version)
		VALUES
		(?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// Prepare arguments in the correct order
	args := []interface{}{
		user.Role,
		user.Status,
		user.Email,
		user.PasswordHash,
		user.FullName,
		user.PhoneNumber,
		user.CreatedAt,
		user.UpdatedAt,
		user.Version,
	}

	// 'h.DB.Exec' executes the query securely.
	result, err := h.DB.Exec(query, args...)
	if err != nil {
		// We will improve this error check later.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
		return
	}

	// Get the new ID that MySQL created.
	id, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get new user ID"})
		return
	}
	user.ID = id // Set the real ID on our user object

	// 6. --- Send Success Response ---
	// Send back the new user object (which now has a real ID).
	c.JSON(http.StatusCreated, gin.H{
		"message": "Dropshipper registered successfully, pending approval.",
		"user":    user,
	})
}

// RegisterSupplier is the handler for the supplier registration endpoint.
func (h *Handlers) RegisterSupplier(c *gin.Context) {
	// 1. --- Define Input ---
	// We can reuse the same input struct, as the fields are identical.
	var input RegisterUserInput

	// 2. --- Bind & Validate JSON ---
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Create User Model ---
	// This is the main difference:
	user := &models.User{
		Role:        "supplier", // Set the role to 'supplier'
		Status:      "pending",  // Suppliers also start as 'pending' [cite: 8, 66]
		Email:       input.Email,
		FullName:    input.FullName,
		PhoneNumber: input.PhoneNumber,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Version:     1,
	}

	// 4. --- Hash the Password ---
	var password models.Password
	if err := password.Set(input.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}
	user.PasswordHash = password.Hash

	// 5. --- Save to Database ---
	// The query is identical.
	query := `
		INSERT INTO users
		(role, status, email, password_hash, full_name, phone_number, created_at, updated_at, version)
		VALUES
		(?, ?, ?, ?, ?, ?, ?, ?, ?)`

	args := []interface{}{
		user.Role,
		user.Status,
		user.Email,
		user.PasswordHash,
		user.FullName,
		user.PhoneNumber,
		user.CreatedAt,
		user.UpdatedAt,
		user.Version,
	}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get new user ID"})
		return
	}
	user.ID = id

	// 6. --- Send Success Response ---
	c.JSON(http.StatusCreated, gin.H{
		"message": "Supplier registered successfully, pending approval.",
		"user":    user,
	})
}

// --- User Login ---

// LoginInput defines the JSON data expected for a login.
type LoginInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password"binding:"required"`
}

// Login is the handler for the /v1/login endpoint.
func (h *Handlers) Login(c *gin.Context) {
	// 1. --- Define Input ---
	var input LoginInput

	// 2. --- Bind & Validate JSON ---
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Find User By Email (Placeholder) ---
	// 4. --- Check Password (Placeholder) ---
	// 5. --- Generate JWT (Placeholder) ---

	// 6. --- Send Success Response (Placeholder) ---
	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful (placeholder)",
		"email":   input.Email,
	})
}
