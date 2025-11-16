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

	// Variants will be populated if IsVariable is true
	Variants []ProductVariant `json:"variants,omitempty" db:"-"` // NEW
}

// ProductVariantOption is a helper for storing the key/value of a variant option
// (e.g., "Size: Small") as an array of objects for the API, and will be JSON-encoded
// before saving to the DB's `product_variants.options` column.
type ProductVariantOption struct {
	Name  string `json:"name"`  // e.g., "Size"
	Value string `json:"value"` // e.g., "Small"
}

// ProductVariant is the model for the 'product_variants' table.
type ProductVariant struct {
	ID            int64          `json:"id" db:"id"`
	ProductID     int64          `json:"productId" db:"product_id"`
	SKU           sql.NullString `json:"sku,omitempty" db:"sku"`
	PriceToTTS    float64        `json:"price" db:"price_to_tts"`
	StockQuantity int            `json:"stock" db:"stock_quantity"`
	Options       string         `json:"options" db:"options"` // JSON encoded ProductVariantOption[]

	// A variant can override the main product's commission. If null, it uses the product's commission_rate.
	CommissionRate sql.NullFloat64 `json:"commissionRate,omitempty" db:"commission_rate"`

	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}
