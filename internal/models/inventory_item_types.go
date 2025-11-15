package models

import (
	"database/sql"
	"time"
)

// InventoryItem is the model for the 'inventory_items' table
type InventoryItem struct {
	ID                int64          `json:"id" db:"id"`
	UserID            int64          `json:"userId" db:"user_id"`
	Name              string         `json:"name" db:"name"`
	Description       sql.NullString `json:"description,omitempty" db:"description"`
	SKU               sql.NullString `json:"sku,omitempty" db:"sku"`
	Price             float64        `json:"price" db:"price"`
	Stock             int            `json:"stock" db:"stock"`
	PromotedProductID sql.NullInt64  `json:"promotedProductId,omitempty" db:"promoted_product_id"`
	CreatedAt         time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time      `json:"updatedAt" db:"updated_at"`
}
