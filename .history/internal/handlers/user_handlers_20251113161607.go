// RegisterDropshipper is the handler for our new endpoint.
func (h *Handlers) RegisterDropshipper(c *gin.Context) {
	// 1. --- Define Input ---
	var input RegisterUserInput

	// 2. --- Bind & Validate JSON ---
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Create User Model ---
	user := &models.User{
		Role:        "dropshipper",
		Status:      "pending",
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
	// This is the new section.
	// We define our SQL query. We use '?' as placeholders
	// for our data to prevent SQL injection attacks.
	query := `
		INSERT INTO users
		(role, status, email, password_hash, full_name, phone_number, created_at, updated_at, version)
		VALUES
		(?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// We prepare the data to pass to the query, in the same order.
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

	// 'h.DB.Exec' runs the query against the database.
	result, err := h.DB.Exec(query, args...)
	if err != nil {
		// We'll add a better error check here later to
		// catch duplicate emails.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
		return
	}

	// Get the 'id' of the new user that was just created.
	id, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get new user ID"})
		return
	}
	user.ID = id // Set the new ID on our user model.

	// 6. --- Send Success Response ---
	// This response will now contain the *real* ID from the database.
	c.JSON(http.StatusCreated, gin.H{
		"message": "Dropshipper registered successfully, pending approval.",
		"user":    user,
	})
}