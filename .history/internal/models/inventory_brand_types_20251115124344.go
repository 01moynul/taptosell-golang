package models

import "time"

// InventoryBrand defines the struct for the 'inventory_brands' table
type InventoryBrand struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"userId" db:"user_id"`
	Name      string    `json:"name" db:"name"`
	Slug      string    `json:"slug" db:"slug"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}
