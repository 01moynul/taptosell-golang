package models

import (
	"database/sql"
	"time"
)

// Product is the model for the 'products' table.
// We are creating this file for the first time.
type Product struct {
	ID          int64          `json:"id" db:"id"`
	SupplierID  int64          `json:"supplierId" db:"supplier_id"`
	SKU         sql.NullString `json:"sku,omitempty" db:"sku"`
	Name        string         `json:"name" db:"name"`
	Description string         `json:"description" db:"description"`
	Price       float64        `json:"price" db:"price"` // The "TapToSell" price
	Stock       int            `json:"stock" db:"stock"`
	IsVariable  bool           `json:"isVariable" db:"is_variable"`
	Status      string         `json:"status" db:"status"` // draft, pending, etc.
	CreatedAt   time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time      `json:"updatedAt" db:"updated_at"`

	// These fields are not in the DB, but will be
	// populated by our handlers using the join tables.
	Categories []Category `json:"categories,omitempty" db:"-"`
	Brands     []Brand    `json:"brands,omitempty" db:"-"`
	// We will add Variants here later
}
