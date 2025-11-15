package models

import (
	"database/sql"
	"time"
)

// WithdrawalRequest is the model for the 'withdrawal_requests' table
type WithdrawalRequest struct {
	ID              int64          `json:"id" db:"id"`
	UserID          int64          `json:"userId" db:"user_id"`
	Amount          float64        `json:"amount" db:"amount"`
	Status          string         `json:"status" db:"status"`
	BankDetails     string         `json:"bankDetails" db:"bank_details"`
	RejectionReason sql.NullString `json:"rejectionReason,omitempty" db:"rejection_reason"`
	CreatedAt       time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time      `json:"updatedAt" db:"updated_at"`

	// These fields are not in the DB, but will be
	// populated by our handlers (e.g., in manager view).
	SupplierName  string `json:"supplierName,omitempty" db:"-"`
	SupplierEmail string `json:"supplierEmail,omitempty" db:"-"`
}
