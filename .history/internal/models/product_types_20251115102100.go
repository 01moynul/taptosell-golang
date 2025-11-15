package models

import (
	"database/sql"
	"time"
)

// Product is the model for the 'products' table.
type Product struct {
	ID          int64          `json:"id" db:"id"`
	SupplierID  int64          `json:"supplierId" db:"supplier_id"`
	SKU         sql.NullString `json:"sku,omitempty" db:"sku"`
	Name        string         `json:"name" db:"name"`
	Description string         `json:"description" db:"description"`

	// --- RENAMED ---
	PriceToTTS     float64         `json:"price" db:"price_to_tts"` // The "TapToSell" price
	StockQuantity  int             `json:"stock" db:"stock_quantity"`
	CommissionRate sql.NullFloat64 `json:"commissionRate,omitempty" db:"commission_rate"` // NEW
	// ---------------

	IsVariable bool      `json:"isVariable" db:"is_variable"`
	Status     string    `json:"status" db:"status"` // draft, pending, etc.
	CreatedAt  time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt  time.Time `json:"updatedAt" db:"updated_at"`

	// --- NEW SHIPPING FIELDS ---
	Weight    sql.NullFloat64 `json:"weight,omitempty" db:"weight"`
	PkgLength sql.NullFloat64 `json:"pkgLength,omitempty" db:"pkg_length"`
	PkgWidth  sql.NullFloat64 `json:"pkgWidth,omitempty" db:"pkg_width"`
	PkgHeight sql.NullFloat64 `json:"pkgHeight,omitempty" db:"pkg_height"`
	// --- END NEW ---

	// These fields are not in the DB, but will be
	// populated by our handlers using the join tables.
	Categories []Category `json:"categories,omitempty" db:"-"`
	Brands     []Brand    `json:"brands,omitempty" db:"-"`
	// We will add Variants here later
}
