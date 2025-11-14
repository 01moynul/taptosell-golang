package handlers

// --- Product Creation ---

// VariantInput defines the structure for a single product variant.
type VariantInput struct {
	SKU   string  `json:"sku"`
	Price float64 `json:"price" binding:"required,gt=0"`
	Stock int     `json:"stock" binding:"gte=0"`
	// We will add options like "Size: Small, Color: Red" later
}

// SimpleProductInput defines the fields for a simple (non-variable) product.
type SimpleProductInput struct {
	SKU   string  `json:"sku"`
	Price float64 `json:"price" binding:"required,gt=0"`
	Stock int     `json:"stock" binding:"gte=0"`
}

// CreateProductInput is the main struct for the POST /v1/products endpoint.
// It's designed to handle both simple and variable products.
type CreateProductInput struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description" binding:"required"`
	Status      string `json:"status" binding:"required,oneof=draft private_inventory pending"` // Validate allowed statuses

	// --- Relational Data ---
	// We accept a *new* brand name (string) or an *existing* brand ID (int64)
	BrandName string `json:"brandName"` // For creating a new brand "on-the-fly"
	BrandID   *int64 `json:"brandId"`   // For linking an existing brand

	// We accept an array of *existing* category IDs
	CategoryIDs []int64 `json:"categoryIds" binding:"required,min=1"`

	// --- Simple vs. Variable Logic ---
	IsVariable bool `json:"isVariable"`

	// Only one of these will be populated based on 'isVariable'
	SimpleProduct *SimpleProductInput `json:"simpleProduct,omitempty"`
	Variants      []VariantInput      `json:"variants,omitempty"`
}

// (We will add the CreateProduct handler function here next)
