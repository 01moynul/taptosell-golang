package models

import "time"

// Category defines the struct for the 'categories' table
type Category struct {
	ID        int64     `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Slug      string    `json:"slug" db:"slug"`
	ParentID  *int64    `json:"parentId,omitempty" db:"parent_id"` // Use pointer for NULL
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}
