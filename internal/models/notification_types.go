package models

import (
	"database/sql"
	"time"
)

// Notification is the model for the 'notifications' table
type Notification struct {
	ID        int64          `json:"id" db:"id"`
	UserID    int64          `json:"userId" db:"user_id"`
	Message   string         `json:"message" db:"message"`
	Link      sql.NullString `json:"link,omitempty" db:"link"`
	IsRead    bool           `json:"isRead" db:"is_read"`
	CreatedAt time.Time      `json:"createdAt" db:"created_at"`
}
