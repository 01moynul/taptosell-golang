package models

import "time"

// InventoryCategory defines the struct for the 'inventory_categories' table
type InventoryCategory struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"userId" db:"user_id"`
	Name      string    `json:"name" db:"name"`
	Slug      string    `json:"slug" db:"slug"`
	ParentID  *int64    `json:"parentId,omitempty" db:"parent_id"` // Use pointer for NULL
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}
