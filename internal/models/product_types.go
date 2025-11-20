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

	// --- Pricing & Stock ---
	PriceToTTS    float64         `json:"price" db:"price_to_tts"`
	StockQuantity int             `json:"stock" db:"stock_quantity"`
	SRP           sql.NullFloat64 `json:"srp,omitempty" db:"srp"` // Legacy Support

	// --- Configuration ---
	IsVariable     bool            `json:"isVariable" db:"is_variable"`
	Status         string          `json:"status" db:"status"`
	CommissionRate sql.NullFloat64 `json:"commissionRate,omitempty" db:"commission_rate"`

	// --- Media & Content (NEW PHASE 8.2) ---
	// We use sql.NullString because these are stored as JSON strings in the DB
	Images          sql.NullString `json:"images,omitempty" db:"images"` // ["url1", "url2"]
	VideoURL        sql.NullString `json:"videoUrl,omitempty" db:"video_url"`
	VideoStatus     string         `json:"videoStatus" db:"video_status"`                   // 'processing', 'ready'
	SizeChart       sql.NullString `json:"sizeChart,omitempty" db:"size_chart"`             // {"type":"image", "url":"..."}
	VariationImages sql.NullString `json:"variationImages,omitempty" db:"variation_images"` // {"Red":"url1", "Blue":"url2"}

	// --- Shipping ---
	Weight      sql.NullFloat64 `json:"weight,omitempty" db:"weight"`
	WeightGrams int             `json:"weightGrams" db:"weight_grams"` // Legacy Support
	PkgLength   sql.NullFloat64 `json:"pkgLength,omitempty" db:"pkg_length"`
	PkgWidth    sql.NullFloat64 `json:"pkgWidth,omitempty" db:"pkg_width"`
	PkgHeight   sql.NullFloat64 `json:"pkgHeight,omitempty" db:"pkg_height"`

	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`

	// Joins
	Categories []Category       `json:"categories,omitempty" db:"-"`
	Brands     []Brand          `json:"brands,omitempty" db:"-"`
	Variants   []ProductVariant `json:"variants,omitempty" db:"-"`
}

// ProductVariant and ProductVariantOption remain unchanged...
type ProductVariantOption struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ProductVariant struct {
	ID             int64           `json:"id" db:"id"`
	ProductID      int64           `json:"productId" db:"product_id"`
	SKU            sql.NullString  `json:"sku,omitempty" db:"sku"`
	PriceToTTS     float64         `json:"price" db:"price_to_tts"`
	StockQuantity  int             `json:"stock" db:"stock_quantity"`
	Options        string          `json:"options" db:"options"`
	CommissionRate sql.NullFloat64 `json:"commissionRate,omitempty" db:"commission_rate"`
	CreatedAt      time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time       `json:"updatedAt" db:"updated_at"`
}
