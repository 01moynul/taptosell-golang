package models

import "time"

// Plan defines the model for the 'plans' table
type Plan struct {
	ID                int64     `json:"id" db:"id"`
	Name              string    `json:"name" db:"name"`
	Description       string    `json:"description" db:"description"`
	Price             float64   `json:"price" db:"price"`
	DurationDays      int       `json:"durationDays" db:"duration_days"`
	AiCreditsIncluded float64   `json:"aiCreditsIncluded" db:"ai_credits_included"`
	IsPublic          bool      `json:"isPublic" db:"is_public"`
	CreatedAt         time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time `json:"updatedAt" db:"updated_at"`
}
