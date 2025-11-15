package models

import "time"

// UserSubscription defines the model for the 'user_subscriptions' table
type UserSubscription struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"userId" db:"user_id"`
	PlanID    int64     `json:"planId" db:"plan_id"`
	Status    string    `json:"status" db:"status"`
	ExpiresAt time.Time `json:"expiresAt" db:"expires_at"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`

	// These fields are not in the DB, but will be
	// populated by our handlers for the manager view.
	PlanName string `json:"planName,omitempty" db:"-"`
	UserName string `json:"userName,omitempty" db:"-"`
}
