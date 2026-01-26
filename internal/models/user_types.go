package models

import (
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User Model with Pointers for Nullable Fields
type User struct {
	ID           int64  `json:"id" db:"id"`
	Role         string `json:"role" db:"role"`
	Status       string `json:"status" db:"status"`
	Email        string `json:"email" db:"email"`
	PasswordHash string `json:"-" db:"password_hash"`
	FullName     string `json:"fullName" db:"full_name"`
	PhoneNumber  string `json:"phoneNumber" db:"phone_number"`

	// Phase 8.7: Manager Penalty Field
	PenaltyStrikes int `json:"penaltyStrikes" db:"penalty_strikes"`

	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
	Version   int       `json:"-" db:"version"`

	// --- Profile Fields (Pointers = Clean JSON) ---
	CompanyName      *string `json:"companyName,omitempty" db:"company_name"`
	ICNumber         *string `json:"icNumber,omitempty" db:"ic_number"`
	SSMNumber        *string `json:"ssmNumber,omitempty" db:"ssm_number"`
	AddressLine1     *string `json:"addressLine1,omitempty" db:"address_line1"`
	AddressLine2     *string `json:"addressLine2,omitempty" db:"address_line2"`
	City             *string `json:"city,omitempty" db:"city"`
	State            *string `json:"state,omitempty" db:"state"`
	Postcode         *string `json:"postcode,omitempty" db:"postcode"`
	SSMDocumentURL   *string `json:"ssmDocumentUrl,omitempty" db:"ssm_document_url"`
	BankStatementURL *string `json:"bankStatementUrl,omitempty" db:"bank_statement_url"`

	// Verification
	VerificationCode   *string    `json:"-" db:"verification_code"`
	VerificationExpiry *time.Time `json:"-" db:"verification_expiry"`
}

// Password Helper (Standard)
type Password struct {
	Plaintext *string
	Hash      string
}

func (p *Password) Set(plaintextPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	p.Hash = string(hash)
	p.Plaintext = &plaintextPassword
	return nil
}

func (p *Password) Matches(plaintextPassword string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(p.Hash), []byte(plaintextPassword))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
