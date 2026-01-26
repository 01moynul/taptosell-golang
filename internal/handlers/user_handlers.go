package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/01moynul/taptosell-golang/internal/auth"
	"github.com/01moynul/taptosell-golang/internal/email"
	"github.com/01moynul/taptosell-golang/internal/models"
	"github.com/gin-gonic/gin"
)

// Helper: Converts string to pointer (empty string -> nil)
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// --- Registration ---

type RegisterUserInput struct {
	FullName    string `json:"fullName" binding:"required"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	PhoneNumber string `json:"phoneNumber" binding:"required"`

	// Supplier Fields
	RegistrationKey string `json:"registrationKey"`
	CompanyName     string `json:"companyName"`
	ICNumber        string `json:"icNumber"`
	SSMNumber       string `json:"ssmNumber"`
	AddressLine1    string `json:"addressLine1"`
	AddressLine2    string `json:"addressLine2"`
	City            string `json:"city"`
	State           string `json:"state"`
	Postcode        string `json:"postcode"`
}

func (h *Handlers) RegisterDropshipper(c *gin.Context) {
	var input RegisterUserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	code, _ := generateVerificationCode()
	expiry := time.Now().Add(15 * time.Minute)

	user := &models.User{
		Role:               "dropshipper",
		Status:             "unverified",
		Email:              input.Email,
		FullName:           input.FullName,
		PhoneNumber:        input.PhoneNumber,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		Version:            1,
		VerificationCode:   &code,   // Pointer
		VerificationExpiry: &expiry, // Pointer
	}

	var password models.Password
	if err := password.Set(input.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}
	user.PasswordHash = password.Hash

	query := `INSERT INTO users (role, status, email, password_hash, full_name, phone_number, created_at, updated_at, version, verification_code, verification_expiry) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := h.DB.Exec(query, user.Role, user.Status, user.Email, user.PasswordHash, user.FullName, user.PhoneNumber, user.CreatedAt, user.UpdatedAt, user.Version, user.VerificationCode, user.VerificationExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
		return
	}

	id, _ := result.LastInsertId()
	user.ID = id
	email.SendVerificationEmail(user.Email, code)

	c.JSON(http.StatusCreated, gin.H{"message": "Registration successful. Please check your email.", "user": user})
}

func (h *Handlers) RegisterSupplier(c *gin.Context) {
	var input RegisterUserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var correctKey string
	err := h.DB.QueryRow("SELECT setting_value FROM settings WHERE setting_key = 'supplier_registration_key'").Scan(&correctKey)
	if err != nil || input.RegistrationKey != correctKey {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid registration key"})
		return
	}

	code, _ := generateVerificationCode()
	expiry := time.Now().Add(15 * time.Minute)

	user := &models.User{
		Role:               "supplier",
		Status:             "unverified",
		Email:              input.Email,
		FullName:           input.FullName,
		PhoneNumber:        input.PhoneNumber,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		Version:            1,
		VerificationCode:   &code,
		VerificationExpiry: &expiry,

		// Use helper for clean pointer assignment
		CompanyName:  strPtr(input.CompanyName),
		ICNumber:     strPtr(input.ICNumber),
		SSMNumber:    strPtr(input.SSMNumber),
		AddressLine1: strPtr(input.AddressLine1),
		AddressLine2: strPtr(input.AddressLine2),
		City:         strPtr(input.City),
		State:        strPtr(input.State),
		Postcode:     strPtr(input.Postcode),
	}

	var password models.Password
	password.Set(input.Password)
	user.PasswordHash = password.Hash

	query := `INSERT INTO users (role, status, email, password_hash, full_name, phone_number, created_at, updated_at, version, verification_code, verification_expiry, company_name, ic_number, ssm_number, address_line1, address_line2, city, state, postcode) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := h.DB.Exec(query, user.Role, user.Status, user.Email, user.PasswordHash, user.FullName, user.PhoneNumber, user.CreatedAt, user.UpdatedAt, user.Version, user.VerificationCode, user.VerificationExpiry, user.CompanyName, user.ICNumber, user.SSMNumber, user.AddressLine1, user.AddressLine2, user.City, user.State, user.Postcode)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register supplier"})
		return
	}

	id, _ := result.LastInsertId()
	user.ID = id
	email.SendVerificationEmail(user.Email, code)

	c.JSON(http.StatusCreated, gin.H{"message": "Supplier registration successful.", "user": user})
}

// --- Login & Verification ---

type LoginInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *Handlers) Login(c *gin.Context) {
	var input LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	err := h.DB.QueryRow("SELECT id, password_hash, role, status FROM users WHERE email = ?", input.Email).Scan(&user.ID, &user.PasswordHash, &user.Role, &user.Status)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if user.Status == "unverified" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Account not verified."})
		return
	}
	if user.Status == "suspended" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Account suspended."})
		return
	}

	var password models.Password
	password.Hash = user.PasswordHash
	match, _ := password.Matches(input.Password)
	if !match {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	token, _ := auth.GenerateToken(user.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Login successful", "token": token, "user": gin.H{"id": user.ID, "role": user.Role}})
}

func generateVerificationCode() (string, error) {
	n := 100000 + (int(rand.Intn(900000)))
	return fmt.Sprintf("%d", n), nil
}

type VerifyEmailInput struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required"`
}

func (h *Handlers) VerifyEmail(c *gin.Context) {
	var input VerifyEmailInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	// Scan directly into pointers
	err := h.DB.QueryRow("SELECT id, status, verification_code, verification_expiry FROM users WHERE email = ?", input.Email).Scan(&user.ID, &user.Status, &user.VerificationCode, &user.VerificationExpiry)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if user.Status != "unverified" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Already verified"})
		return
	}

	// Safety check for nil pointers
	if user.VerificationCode == nil || user.VerificationExpiry == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No code found"})
		return
	}
	if *user.VerificationCode != input.Code {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid code"})
		return
	}
	if time.Now().After(*user.VerificationExpiry) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Code expired"})
		return
	}

	h.DB.Exec("UPDATE users SET status = 'pending', verification_code = NULL, verification_expiry = NULL WHERE id = ?", user.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Email verified."})
}

type ResendVerificationEmailInput struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *Handlers) ResendVerificationEmail(c *gin.Context) {
	var input ResendVerificationEmailInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var user models.User
	if err := h.DB.QueryRow("SELECT id, status FROM users WHERE email = ?", input.Email).Scan(&user.ID, &user.Status); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if user.Status != "unverified" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Already verified"})
		return
	}
	code, _ := generateVerificationCode()
	expiry := time.Now().Add(15 * time.Minute)
	h.DB.Exec("UPDATE users SET verification_code = ?, verification_expiry = ? WHERE id = ?", code, expiry, user.ID)
	email.SendVerificationEmail(input.Email, code)
	c.JSON(http.StatusOK, gin.H{"message": "New code sent."})
}

// --- Uploads ---

func (h *Handlers) UploadSupplierDocuments(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)
	uploadDir := "./uploads"
	os.MkdirAll(uploadDir, os.ModePerm)

	saveFile := func(name string) string {
		file, header, err := c.Request.FormFile(name)
		if err != nil {
			return ""
		}
		defer file.Close()
		path := filepath.Join(uploadDir, fmt.Sprintf("%d-%s-%s", userID, name, filepath.Base(header.Filename)))
		dst, _ := os.Create(path)
		defer dst.Close()
		io.Copy(dst, file)
		return path
	}

	ssm := saveFile("ssm_document")
	bank := saveFile("bank_statement")

	if ssm != "" {
		h.DB.Exec("UPDATE users SET ssm_document_url = ? WHERE id = ?", ssm, userID)
	}
	if bank != "" {
		h.DB.Exec("UPDATE users SET bank_statement_url = ? WHERE id = ?", bank, userID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Uploaded"})
}

// --- Manager Functions ---

// GetUsers returns all users
// GET /v1/manager/users
func (h *Handlers) GetUsers(c *gin.Context) {
	query := `SELECT id, role, status, email, full_name, phone_number, penalty_strikes, created_at FROM users ORDER BY id DESC`
	rows, err := h.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DB error"})
		return
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var u models.User
		var penaltyStrikes sql.NullInt64

		// [FIX] Scanning pointers matches the updated User struct
		if err := rows.Scan(&u.ID, &u.Role, &u.Status, &u.Email, &u.FullName, &u.PhoneNumber, &penaltyStrikes, &u.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Scan error"})
			return
		}

		// Handle Nullable Int logic separately (since struct field is int, not *int)
		if penaltyStrikes.Valid {
			u.PenaltyStrikes = int(penaltyStrikes.Int64)
		} else {
			u.PenaltyStrikes = 0
		}

		users = append(users, &u)
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

type UpdateUserPenaltyInput struct {
	Action string `json:"action" binding:"required,oneof=increment decrement reset"`
}

// UpdateUserPenalty
func (h *Handlers) UpdateUserPenalty(c *gin.Context) {
	id := c.Param("id")
	var input UpdateUserPenaltyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var current int
	h.DB.QueryRow("SELECT COALESCE(penalty_strikes, 0) FROM users WHERE id = ?", id).Scan(&current)

	if input.Action == "increment" {
		current++
	}
	if input.Action == "decrement" && current > 0 {
		current--
	}
	if input.Action == "reset" {
		current = 0
	}

	h.DB.Exec("UPDATE users SET penalty_strikes = ? WHERE id = ?", current, id)
	c.JSON(http.StatusOK, gin.H{"message": "Penalty updated"})
}

// --- Admin ---
type CreateManagerInput struct {
	FullName    string `json:"fullName"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	PhoneNumber string `json:"phoneNumber"`
}

func (h *Handlers) CreateManager(c *gin.Context) {
	var input CreateManagerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user := &models.User{
		Role: "manager", Status: "active", Email: input.Email, FullName: input.FullName, PhoneNumber: input.PhoneNumber, CreatedAt: time.Now(), UpdatedAt: time.Now(), Version: 1,
	}
	var password models.Password
	password.Set(input.Password)
	user.PasswordHash = password.Hash

	res, _ := h.DB.Exec("INSERT INTO users (role, status, email, password_hash, full_name, phone_number, created_at, updated_at, version) VALUES (?,?,?,?,?,?,?,?,?)",
		user.Role, user.Status, user.Email, user.PasswordHash, user.FullName, user.PhoneNumber, user.CreatedAt, user.UpdatedAt, user.Version)

	id, _ := res.LastInsertId()
	user.ID = id
	c.JSON(http.StatusCreated, gin.H{"message": "Manager created", "user": user})
}
