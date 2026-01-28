package models

import (
	"time"
)

// Product is the model for the 'products' table.
// [FIX]: Switched sql.Null* types to Pointers (*string, *float64) for clean JSON serialization.
type Product struct {
	ID          int64   `json:"id" db:"id"`
	SupplierID  int64   `json:"supplierId" db:"supplier_id"`
	SKU         *string `json:"sku,omitempty" db:"sku"` // Changed from sql.NullString
	Name        string  `json:"name" db:"name"`
	Description string  `json:"description" db:"description"`

	// --- Pricing & Stock ---
	PriceToTTS    float64 `json:"price" db:"price_to_tts"`
	StockQuantity int     `json:"stock" db:"stock_quantity"`
	SRP           float64 `json:"srp" db:"srp"`

	// --- Configuration ---
	IsVariable     bool     `json:"isVariable" db:"is_variable"`
	Status         string   `json:"status" db:"status"`
	CommissionRate *float64 `json:"commissionRate,omitempty" db:"commission_rate"` // Changed from sql.NullFloat64

	// --- Media & Content ---
	Images          []string               `json:"images"`
	VideoURL        string                 `json:"videoUrl"`
	VideoStatus     string                 `json:"videoStatus" db:"video_status"`
	SizeChart       map[string]interface{} `json:"sizeChart"`
	VariationImages map[string]string      `json:"variationImages"`

	// --- Shipping ---
	Weight      *float64 `json:"weight,omitempty" db:"weight"` // Changed from sql.NullFloat64
	WeightGrams int      `json:"weightGrams" db:"weight_grams"`
	PkgLength   *float64 `json:"pkgLength,omitempty" db:"pkg_length"` // Changed from sql.NullFloat64
	PkgWidth    *float64 `json:"pkgWidth,omitempty" db:"pkg_width"`   // Changed from sql.NullFloat64
	PkgHeight   *float64 `json:"pkgHeight,omitempty" db:"pkg_height"` // Changed from sql.NullFloat64

	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`

	// Joins (Not in DB table, populated manually)
	Categories []Category       `json:"categories,omitempty" db:"-"`
	Brands     []Brand          `json:"brands,omitempty" db:"-"`
	Variants   []ProductVariant `json:"variants,omitempty" db:"-"`

	// Flattened fields for UI convenience (populated manually if needed)
	SupplierName string `json:"supplierName,omitempty" db:"-"`
}

// ProductVariantOption defines the structure for variant options JSON
type ProductVariantOption struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ProductVariant is the model for the 'product_variants' table
type ProductVariant struct {
	ID             int64     `json:"id" db:"id"`
	ProductID      int64     `json:"productId" db:"product_id"`
	SKU            *string   `json:"sku,omitempty" db:"sku"` // Changed from sql.NullString
	PriceToTTS     float64   `json:"price" db:"price_to_tts"`
	StockQuantity  int       `json:"stock" db:"stock_quantity"`
	Options        string    `json:"options" db:"options"`                          // Stored as JSON string in DB
	CommissionRate *float64  `json:"commissionRate,omitempty" db:"commission_rate"` // Changed from sql.NullFloat64
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time `json:"updatedAt" db:"updated_at"`
}
