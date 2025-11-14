package models

import (
	"database/sql"
	"time"
)

// WalletTransaction is the model for the 'wallet_transactions' table
type WalletTransaction struct {
	ID        int64          `json:"id" db:"id"`
	UserID    int64          `json:"userId" db:"user_id"`
	Type      string         `json:"type" db:"type"`     // e.g., deposit, withdrawal, order
	Amount    float64        `json:"amount" db:"amount"` // Can be positive (deposit) or negative (order)
	Details   sql.NullString `json:"details,omitempty" db:"details"`
	CreatedAt time.Time      `json:"createdAt" db:"created_at"`
}
