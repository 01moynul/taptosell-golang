package handlers

import (
	"database/sql" // For our DB connection
	"log"
	"net/http" // For HTTP status codes
	"time"     // For time.Now()

	"io"            // For copying file data
	"os"            // For creating files and directories
	"path/filepath" // For creating safe file paths

	"github.com/01moynul/taptosell-golang/internal/auth"
	"github.com/01moynul/taptosell-golang/internal/models" // Our User models
	"github.com/gin-gonic/gin"                             // The Gin framework

	"crypto/rand" // For generating the verification code
	"fmt"         // For the verification code

	"github.com/01moynul/taptosell-golang/internal/email" // Our new email package
)

// --- User Registration ---

// RegisterUserInput defines the expected JSON data for registration.
// The 'binding' tags are used by Gin for automatic validation.
type RegisterUserInput struct {
	// --- Core Fields (Dropshipper & Supplier) ---
	FullName    string `json:"fullName" binding:"required"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	PhoneNumber string `json:"phoneNumber" binding:"required"`

	// --- Supplier-Only Fields ---
	RegistrationKey string `json:"registrationKey"` // Required only for suppliers
	CompanyName     string `json:"companyName"`
	ICNumber        string `json:"icNumber"`
	SSMNumber       string `json:"ssmNumber"`
	AddressLine1    string `json:"addressLine1"`
	AddressLine2    string `json:"addressLine2"`
	City            string `json:"city"`
	State           string `json:"state"`
	Postcode        string `json:"postcode"`
}

// RegisterDropshipper is the handler for our new endpoint.
func (h *Handlers) RegisterDropshipper(c *gin.Context) {
	// 1. --- Define Input ---
	var input RegisterUserInput

	// 2. --- Bind & Validate JSON ---
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Generate Verification Code ---
	code, err := generateVerificationCode()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate verification code"})
		return
	}
	// Code expires in 15 minutes
	expiry := time.Now().Add(15 * time.Minute)

	// 4. --- Create User Model ---
	user := &models.User{
		Role:        "dropshipper",
		Status:      "unverified", // <-- CHANGED: New users are now 'unverified'
		Email:       input.Email,
		FullName:    input.FullName,
		PhoneNumber: input.PhoneNumber,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Version:     1,
		// --- Set verification fields ---
		VerificationCode:   sql.NullString{String: code, Valid: true},
		VerificationExpiry: sql.NullTime{Time: expiry, Valid: true},
	}

	// 5. --- Hash the Password ---
	var password models.Password
	if err := password.Set(input.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}
	user.PasswordHash = password.Hash

	// 6. --- Save to Database ---
	// The query is now much longer to include the new fields
	query := `
		INSERT INTO users
		(role, status, email, password_hash, full_name, phone_number, created_at, updated_at, version, verification_code, verification_expiry)
		VALUES
		(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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
		user.VerificationCode,
		user.VerificationExpiry,
	}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		// We'll add a duplicate email check here later
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get new user ID"})
		return
	}
	user.ID = id

	// 7. --- Send Verification Email ---
	// Use our new (placeholder) email service
	err = email.SendVerificationEmail(user.Email, code)
	if err != nil {
		// If email fails, we should ideally roll back the transaction
		// For now, we'll just log an error but still tell the user.
		log.Printf("ERROR: Failed to send verification email to %s: %v\n", user.Email, err)
	}

	// 8. --- Send Success Response ---
	// The message is different now, guiding the user to the next step.
	c.JSON(http.StatusCreated, gin.H{
		"message": "Registration successful. Please check your email for a verification code.",
		"user":    user,
	})
}

// RegisterSupplier is the handler for the supplier registration endpoint.
func (h *Handlers) RegisterSupplier(c *gin.Context) {
	// 1. --- Define Input ---
	var input RegisterUserInput

	// 2. --- Bind & Validate JSON ---
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- VALIDATE REGISTRATION KEY ---
	// From our blueprint, supplier registration is protected.
	if input.RegistrationKey != supplierRegistrationKey {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid registration key"})
		return
	}

	// 4. --- Generate Verification Code ---
	code, err := generateVerificationCode()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate verification code"})
		return
	}
	expiry := time.Now().Add(15 * time.Minute) // Code expires in 15 minutes

	// 5. --- Create User Model ---
	user := &models.User{
		Role:        "supplier",
		Status:      "unverified", // <-- CHANGED: New users are 'unverified'
		Email:       input.Email,
		FullName:    input.FullName,
		PhoneNumber: input.PhoneNumber,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Version:     1,
		// --- Set verification fields ---
		VerificationCode:   sql.NullString{String: code, Valid: true},
		VerificationExpiry: sql.NullTime{Time: expiry, Valid: true},
		// --- Set new supplier-specific fields ---
		// We must convert our simple strings into 'sql.NullString'
		CompanyName:  sql.NullString{String: input.CompanyName, Valid: input.CompanyName != ""},
		ICNumber:     sql.NullString{String: input.ICNumber, Valid: input.ICNumber != ""},
		SSMNumber:    sql.NullString{String: input.SSMNumber, Valid: input.SSMNumber != ""},
		AddressLine1: sql.NullString{String: input.AddressLine1, Valid: input.AddressLine1 != ""},
		AddressLine2: sql.NullString{String: input.AddressLine2, Valid: input.AddressLine2 != ""},
		City:         sql.NullString{String: input.City, Valid: input.City != ""},
		State:        sql.NullString{String: input.State, Valid: input.State != ""},
		Postcode:     sql.NullString{String: input.Postcode, Valid: input.Postcode != ""},
	}
	// Note: ssm_document_url and bank_statement_url will be set in Phase 3.6

	// 6. --- Hash the Password ---
	var password models.Password
	if err := password.Set(input.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}
	user.PasswordHash = password.Hash

	// 7. --- Save to Database ---
	// This query is now massive and includes all new fields.
	query := `
		INSERT INTO users
		(role, status, email, password_hash, full_name, phone_number, created_at, updated_at, version,
		verification_code, verification_expiry,
		company_name, ic_number, ssm_number, address_line1, address_line2, city, state, postcode)
		VALUES
		(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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
		user.VerificationCode,
		user.VerificationExpiry,
		user.CompanyName,
		user.ICNumber,
		user.SSMNumber,
		user.AddressLine1,
		user.AddressLine2,
		user.City,
		user.State,
		user.Postcode,
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

	// 8. --- Send Verification Email ---
	err = email.SendVerificationEmail(user.Email, code)
	if err != nil {
		log.Printf("ERROR: Failed to send verification email to %s: %v\n", user.Email, err)
	}

	// 9. --- Send Success Response ---
	c.JSON(http.StatusCreated, gin.H{
		"message": "Supplier registration successful. Please check your email for a verification code.",
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

	// 3. --- Find User By Email ---
	var user models.User
	query := "SELECT id, password_hash, role, status FROM users WHERE email = ?"

	err := h.DB.QueryRow(query, input.Email).Scan(
		&user.ID,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// 4. --- CHECK USER STATUS (NEW) ---
	// We now block logins based on the new status ENUM
	switch user.Status {
	case "unverified":
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Account not verified. Please check your email for a verification code."})
		return
	case "pending":
		c.JSON(http.StatusForbidden, gin.H{"error": "Your account is pending approval by an administrator."})
		return
	case "suspended":
		c.JSON(http.StatusForbidden, gin.H{"error": "Your account has been suspended. Please contact support."})
		return
	case "active":
		// User is active, continue to password check
		break
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unknown user status"})
		return
	}

	// 5. --- Check Password ---
	// This code now only runs if the user is 'active'
	var password models.Password
	password.Hash = user.PasswordHash

	match, err := password.Matches(input.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check password"})
		return
	}
	if !match {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// 6. --- Generate JWT (The "Passport") ---
	token, err := auth.GenerateToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// 7. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"token":   token,
		"user": gin.H{
			"id":   user.ID,
			"role": user.Role,
		},
	})
}

// generateVerificationCode creates a simple 6-digit numeric code.
func generateVerificationCode() (string, error) {
	// Create a 6-digit code (100000 - 999999)
	n := 100000 + (int(rand.Intn(900000)))
	return fmt.Sprintf("%d", n), nil
}

// --- User Verification ---

// VerifyEmailInput defines the expected JSON for email verification.
type VerifyEmailInput struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required"`
}

// VerifyEmail is the handler for the /v1/auth/verify-email endpoint.
func (h *Handlers) VerifyEmail(c *gin.Context) {
	// 1. --- Define Input ---
	var input VerifyEmailInput

	// 2. --- Bind & Validate JSON ---
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Find User By Email ---
	var user models.User
	query := "SELECT id, status, verification_code, verification_expiry FROM users WHERE email = ?"

	// Get the user's current data
	err := h.DB.QueryRow(query, input.Email).Scan(
		&user.ID,
		&user.Status,
		&user.VerificationCode,
		&user.VerificationExpiry,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// 4. --- Check Status ---
	if user.Status != "unverified" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Account is already verified or in an invalid state."})
		return
	}

	// 5. --- Check Code & Expiry ---
	// Check if code or expiry are NULL (invalid state)
	if !user.VerificationCode.Valid || !user.VerificationExpiry.Valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No verification code found for this user."})
		return
	}
	// Check if code matches
	if user.VerificationCode.String != input.Code {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid verification code."})
		return
	}
	// Check if expired
	if time.Now().After(user.VerificationExpiry.Time) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Verification code has expired."})
		return
	}

	// 6. --- SUCCESS: Update User ---
	// Set status to 'pending' (ready for admin approval)
	// Set verification fields to NULL to clean up.
	updateQuery := `
		UPDATE users
		SET status = 'pending', verification_code = NULL, verification_expiry = NULL, updated_at = ?
		WHERE id = ?`

	_, err = h.DB.Exec(updateQuery, time.Now(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user status"})
		return
	}

	// 7. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"message": "Email verified successfully. Your account is now pending administrator approval.",
	})
}

// ResendVerificationEmailInput defines the JSON for resending a code.
type ResendVerificationEmailInput struct {
	Email string `json:"email" binding:"required,email"`
}

// ResendVerificationEmail is the handler for /v1/auth/resend-code
func (h *Handlers) ResendVerificationEmail(c *gin.Context) {
	// 1. --- Define Input ---
	var input ResendVerificationEmailInput

	// 2. --- Bind & Validate JSON ---
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Find User ---
	var user models.User
	query := "SELECT id, status FROM users WHERE email = ?"
	err := h.DB.QueryRow(query, input.Email).Scan(&user.ID, &user.Status)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// 4. --- Check Status ---
	if user.Status != "unverified" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This account is already verified."})
		return
	}

	// 5. --- Generate New Code & Expiry ---
	code, err := generateVerificationCode()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate code"})
		return
	}
	expiry := time.Now().Add(15 * time.Minute)

	// 6. --- Update User in DB ---
	updateQuery := `
		UPDATE users
		SET verification_code = ?, verification_expiry = ?, updated_at = ?
		WHERE id = ?`

	_, err = h.DB.Exec(updateQuery, code, expiry, time.Now(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update verification code"})
		return
	}

	// 7. --- Send New Email ---
	err = email.SendVerificationEmail(input.Email, code)
	if err != nil {
		log.Printf("ERROR: Failed to resend verification email to %s: %v\n", input.Email, err)
	}

	// 8. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"message": "A new verification code has been sent to your email.",
	})
}

// --- Supplier Document Upload ---

// UploadSupplierDocuments handles the multipart/form-data upload for SSM and Bank Statements.
// This is a protected route, so it will have access to the "userID" from the auth middleware.
func (h *Handlers) UploadSupplierDocuments(c *gin.Context) {
	// 1. --- Get UserID from Middleware ---
	// We know this user is authenticated because this route will be protected.
	userID_raw, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in context"})
		return
	}
	userID := userID_raw.(int64) // Cast the userID to int64

	// 2. --- Verify User is a Supplier ---
	// (We'll add this check for extra security, though a 'pending' user can't log in yet)
	// For now, we'll assume the user is the correct one.
	// In a future step, we'd query the DB to check role:
	// var userRole string
	// h.DB.QueryRow("SELECT role FROM users WHERE id = ?", userID).Scan(&userRole)
	// if userRole != "supplier" { ... }

	// 3. --- Ensure Uploads Directory Exists ---
	// We'll save files to a folder named 'uploads' in our project root.
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}

	// 4. --- Process Files ---
	// We will store the paths to update the DB
	var ssmDocPath string
	var bankStmtPath string

	// Create a map to update the DB
	dbUpdateMap := make(map[string]string)

	// Helper function to save a file
	saveFile := func(formFileName string, dbColumnName string) error {
		file, header, err := c.Request.FormFile(formFileName)
		if err != nil {
			// http.ErrMissingFile means the user just didn't upload this one, which is fine.
			if err == http.ErrMissingFile {
				return nil
			}
			return err
		}
		defer file.Close()

		// Create a safe, unique filename
		// e.g., "uploads/123-ssm_document-original_filename.pdf"
		safeFilename := fmt.Sprintf("%d-%s-%s", userID, formFileName, filepath.Base(header.Filename))
		targetPath := filepath.Join(uploadDir, safeFilename)

		// Create the destination file
		dst, err := os.Create(targetPath)
		if err != nil {
			return err
		}
		defer dst.Close()

		// Copy the uploaded file data to the destination file
		if _, err := io.Copy(dst, file); err != nil {
			return err
		}

		// Store the path for our DB update
		dbUpdateMap[dbColumnName] = targetPath
		return nil
	}

	// 5. --- Save the files from the form ---
	if err := saveFile("ssm_document", "ssm_document_url"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save ssm_document: %v", err)})
		return
	}
	if err := saveFile("bank_statement", "bank_statement_url"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save bank_statement: %v", err)})
		return
	}

	if len(dbUpdateMap) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No files were uploaded"})
		return
	}

	// 6. --- Update User in Database ---
	// This query is a bit dynamic.
	// We build the "SET" part of the query based on which files were uploaded.
	queryArgs := []interface{}{time.Now()}
	querySet := "updated_at = ?"

	if path, ok := dbUpdateMap["ssm_document_url"]; ok {
		querySet += ", ssm_document_url = ?"
		queryArgs = append(queryArgs, path)
		ssmDocPath = path
	}
	if path, ok := dbUpdateMap["bank_statement_url"]; ok {
		querySet += ", bank_statement_url = ?"
		queryArgs = append(queryArgs, path)
		bankStmtPath = path
	}

	// Add the userID as the final argument for the WHERE clause
	queryArgs = append(queryArgs, userID)

	query := fmt.Sprintf("UPDATE users SET %s WHERE id = ?", querySet)

	_, err := h.DB.Exec(query, queryArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user document paths"})
		return
	}

	// 7. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"message":            "Documents uploaded successfully.",
		"ssm_document_url":   ssmDocPath,
		"bank_statement_url": bankStmtPath,
	})
}
