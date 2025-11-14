package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/01moynul/taptosell-golang/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gosimple/slug"
)

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
// CreateProduct is the handler for POST /v1/products
// It is protected by the AuthMiddleware and requires a supplier role.
func (h *Handlers) CreateProduct(c *gin.Context) {
	// 1. --- Get Supplier ID ---
	// We get the userID from the AuthMiddleware context
	userID_raw, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in context"})
		return
	}
	supplierID := userID_raw.(int64) // Cast the userID to int64

	// (Future Step: We can add a check here to ensure the user's role is 'supplier')

	// 2. --- Bind & Validate JSON ---
	var input CreateProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Validate Logic (Simple vs Variable) ---
	if input.IsVariable {
		if len(input.Variants) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Variable product must have at least one variant"})
			return
		}
		// Clear simple product data just in case
		input.SimpleProduct = nil
	} else {
		if input.SimpleProduct == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Simple product must have simpleProduct data"})
			return
		}
		// Clear variant data just in case
		input.Variants = nil
	}

	// 4. --- Begin Database Transaction ---
	// Product creation is complex (multiple tables). A transaction ensures
	// that if one part fails (e.g., linking a category), the entire
	// operation is rolled back, preventing orphaned data.
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start database transaction"})
		return
	}
	// 'defer tx.Rollback()' is a safety net. If we return anywhere
	// *without* tx.Commit(), this will automatically cancel the transaction.
	defer tx.Rollback()

	// 5. --- Handle Brand (Get or Create) ---
	brandID, err := h.getOrCreateBrandID(tx, input.BrandID, input.BrandName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 6. --- Create the Core Product ---
	// For now, we are saving the *main* product.
	// We will handle variants in a future step.
	product := &models.Product{
		SupplierID:  supplierID,
		Name:        input.Name,
		Description: input.Description,
		IsVariable:  input.IsVariable,
		Status:      input.Status,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// If it's a simple product, save its price/sku/stock on the main record
	if !input.IsVariable {
		product.Price = input.SimpleProduct.Price
		product.Stock = input.SimpleProduct.Stock
		product.SKU = sql.NullString{String: input.SimpleProduct.SKU, Valid: input.SimpleProduct.SKU != ""}
	}

	productQuery := `
		INSERT INTO products
		(supplier_id, name, description, price, stock, sku, is_variable, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := tx.Exec(productQuery,
		product.SupplierID, product.Name, product.Description,
		product.Price, product.Stock, product.SKU,
		product.IsVariable, product.Status, product.CreatedAt, product.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert product"})
		return
	}
	productID, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get new product ID"})
		return
	}
	product.ID = productID

	// 7. --- Link Categories ---
	// We loop through the array of category IDs and insert each link
	// into the 'product_categories' junction table.
	categoryQuery := `INSERT INTO product_categories (product_id, category_id) VALUES (?, ?)`
	for _, catID := range input.CategoryIDs {
		_, err := tx.Exec(categoryQuery, productID, catID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link category"})
			return
		}
	}

	// 8. --- Link Brand ---
	brandQuery := `INSERT INTO product_brands (product_id, brand_id) VALUES (?, ?)`
	_, err = tx.Exec(brandQuery, productID, brandID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link brand"})
		return
	}

	// 9. --- TODO: Handle Variants (Phase 4.3 extension) ---
	// If input.IsVariable is true, we would loop through input.Variants
	// and insert them into a new 'product_variants' table,
	// linking them to this 'productID'. We will add this later.

	// 10. --- TODO: Notify Managers (Phase 4.4) ---
	// If product.Status == "pending", we would add logic here
	// to send a notification to all users with the 'manager' role.

	// 11. --- Commit Transaction ---
	// If we get this far, all database queries were successful.
	// We commit the transaction, making all changes permanent.
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// 12. --- Send Success Response ---
	// We'll fetch the full product details later. For now, send back the new ID.
	c.JSON(http.StatusCreated, gin.H{
		"message":   "Product created successfully",
		"productId": product.ID,
	})
}

// getOrCreateBrandID is a helper function to find an existing brand
// or create a new one "on-the-fly" within the transaction.
func (h *Handlers) getOrCreateBrandID(tx *sql.Tx, brandID *int64, brandName string) (int64, error) {
	// Case 1: An existing BrandID was provided.
	if brandID != nil {
		// We still do a quick check to make sure it exists
		var exists int
		err := tx.QueryRow("SELECT 1 FROM brands WHERE id = ?", *brandID).Scan(&exists)
		if err != nil {
			if err == sql.ErrNoRows {
				return 0, errors.New("invalid brandId: brand does not exist")
			}
			return 0, err
		}
		return *brandID, nil // Return the existing ID
	}

	// Case 2: A new BrandName was provided.
	if brandName != "" {
		// Check if this brand name *already* exists
		var existingID int64
		slug := slug.Make(brandName)
		err := tx.QueryRow("SELECT id FROM brands WHERE slug = ?", slug).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			return 0, err // A real database error occurred
		}
		if err == nil {
			return existingID, nil // Brand already exists, return its ID
		}

		// Brand does not exist, so we create it
		now := time.Now()
		query := `INSERT INTO brands (name, slug, created_at, updated_at) VALUES (?, ?, ?, ?)`
		result, err := tx.Exec(query, brandName, slug, now, now)
		if err != nil {
			return 0, err
		}
		newID, err := result.LastInsertId()
		if err != nil {
			return 0, err
		}
		return newID, nil // Return the newly created ID
	}

	// Case 3: Neither was provided.
	return 0, errors.New("a brandId or a new brandName is required")
}
