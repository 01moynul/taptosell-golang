package models

import "time"

// AiUserCredit defines the model for the 'ai_user_credits' table
type AiUserCredit struct {
	ID               int64     `json:"id" db:"id"`
	UserID           int64     `json:"userId" db:"user_id"`
	CreditsRemaining float64   `json:"creditsRemaining" db:"credits_remaining"`
	UpdatedAt        time.Time `json:"updatedAt" db:"updated_at"`
}
