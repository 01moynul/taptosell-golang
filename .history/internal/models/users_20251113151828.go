package models

import (
	"errors" // For handling errors
	"time"   // For 'created_at' and 'updated_at'

	"golang.org/x/crypto/bcrypt" // Import the package we just installed
)

// User is the model for the 'users' table in our database.
// The 'json:"..."' tags define how data is named when sent/received as JSON.
// The 'db:"..."' tags will be used later for database operations.
type User struct {
	ID           int64     `json:"id" db:"id"`
	Role         string    `json:"role" db:"role"`
	Status       string    `json:"status" db:"status"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"` // '-' means never include this in JSON
	FullName     string    `json:"fullName" db:"full_name"`
	PhoneNumber  string    `json:"phoneNumber" db:"phone_number"`
	CreatedAt    time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time `json:"updatedAt" db:"updated_at"`
	Version      int       `json:"-" db:"version"` // Also hide version from JSON
}

// Password is a helper struct for handling password operations.
type Password struct {
	Plaintext *string // A pointer to a string for the plaintext password
	Hash      string  // The resulting hash
}

// Set takes a plaintext password, hashes it using bcrypt, and stores the hash.
func (p *Password) Set(plaintextPassword string) error {
	// Use bcrypt.GenerateFromPassword to create the hash
	// bcrypt.DefaultCost is a good, secure default setting.
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Store the hash and the original plaintext
	p.Hash = string(hash)
	p.Plaintext = &plaintextPassword

	return nil
}

// Matches checks if a given plaintext password matches the stored hash.
func (p *Password) Matches(plaintextPassword string) (bool, error) {
	// Compare the stored hash with the new plaintext password.
	err := bcrypt.CompareHashAndPassword([]byte(p.Hash), []byte(plaintextPassword))
	if err != nil {
		// If they don't match, bcrypt returns a specific error.
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil // Passwords do not match
		}
		// Some other error occurred
		return false, err
	}

	return true, nil // Passwords match
}
