package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/01moynul/taptosell-golang/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gosimple/slug"
)

// --- Product Creation ---

// VariantInput defines the structure for a single product variant submitted via the React form.
type VariantInput struct {
	SKU   string  `json:"sku"`
	Price float64 `json:"price" binding:"required,gt=0"`
	Stock int     `json:"stock" binding:"gte=0"`
	// Options represents the combination (e.g., [{"name": "Size", "value": "Small"}])
	Options []models.ProductVariantOption `json:"options" binding:"required,min=1"`
	// CommissionRate is for per-variant overrides
	CommissionRate *float64 `json:"commissionRate,omitempty" binding:"omitempty,gte=0"`
}

// SimpleProductInput defines the fields for a simple (non-variable) product.
type SimpleProductInput struct {
	SKU   string  `json:"sku"`
	Price float64 `json:"price" binding:"required,gt=0"`
	Stock int     `json:"stock" binding:"gte=0"`
	// NEW: Added CommissionRate (nullable)
	CommissionRate *float64 `json:"commissionRate,omitempty" binding:"omitempty,gte=0"`
}

// PackageDimensionsInput matches the nested object in the React form
type PackageDimensionsInput struct {
	Length float64 `json:"length" binding:"gte=0"`
	Width  float64 `json:"width" binding:"gte=0"`
	Height float64 `json:"height" binding:"gte=0"`
}

// CreateProductInput is the main struct for the POST /v1/products endpoint.
type CreateProductInput struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description" binding:"required"`
	Status      string `json:"status" binding:"required,oneof=draft private_inventory pending"`

	// --- Relational Data ---
	BrandName string `json:"brandName"`
	BrandID   *int64 `json:"brandId"`

	// --- Relational Data ---
	CategoryIDs []int64 `json:"categoryIds" binding:"required,min=1"`

	// --- Simple vs. Variable Logic ---
	IsVariable bool `json:"isVariable"`

	// Only one of these will be populated based on 'isVariable'
	SimpleProduct *SimpleProductInput `json:"simpleProduct,omitempty"`
	Variants      []VariantInput      `json:"variants,omitempty"`

	// --- NEW SHIPPING FIELDS ---
	Weight            *float64                `json:"weight" binding:"omitempty,gt=0"`
	PackageDimensions *PackageDimensionsInput `json:"packageDimensions,omitempty"`

	// NEW: Added CommissionRate for variable products (applies to all variants for now)
	CommissionRate *float64 `json:"commissionRate,omitempty" binding:"omitempty,gte=0"`
}

// CreateProduct is the handler for POST /v1/products
func (h *Handlers) CreateProduct(c *gin.Context) {
	// 1. --- Get Supplier ID ---
	userID_raw, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in context"})
		return
	}
	supplierID := userID_raw.(int64)

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
		input.SimpleProduct = nil
	} else {
		if input.SimpleProduct == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Simple product must have simpleProduct data"})
			return
		}
		input.Variants = nil
	}

	// 4. --- Begin Database Transaction ---
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start database transaction"})
		return
	}
	defer tx.Rollback()

	// 5. --- Handle Brand (Get or Create) ---
	brandID, err := h.getOrCreateBrandID(tx, input.BrandID, input.BrandName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 6. --- Create the Core Product ---
	product := &models.Product{
		SupplierID:  supplierID,
		Name:        input.Name,
		Description: input.Description,
		IsVariable:  input.IsVariable,
		Status:      input.Status,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Handle Commission (nullable)
	var commissionRate sql.NullFloat64
	if !input.IsVariable {
		// Simple Product: Use commission from simpleProduct object
		product.PriceToTTS = input.SimpleProduct.Price
		product.StockQuantity = input.SimpleProduct.Stock
		product.SKU = sql.NullString{String: input.SimpleProduct.SKU, Valid: input.SimpleProduct.SKU != ""}
		if input.SimpleProduct.CommissionRate != nil {
			commissionRate = sql.NullFloat64{Float64: *input.SimpleProduct.CommissionRate, Valid: true}
		}
	} else {
		// Variable Product: Use top-level commissionRate
		if input.CommissionRate != nil {
			commissionRate = sql.NullFloat64{Float64: *input.CommissionRate, Valid: true}
		}
		// Price/Stock are 0 on the main product, they live in variants
		product.PriceToTTS = 0
		product.StockQuantity = 0
	}
	product.CommissionRate = commissionRate // Assign to model

	// Handle nullable shipping fields
	var weight sql.NullFloat64
	var pkgLength sql.NullFloat64
	var pkgWidth sql.NullFloat64
	var pkgHeight sql.NullFloat64

	if input.Weight != nil {
		weight = sql.NullFloat64{Float64: *input.Weight, Valid: true}
	}

	if input.PackageDimensions != nil {
		pkgLength = sql.NullFloat64{Float64: input.PackageDimensions.Length, Valid: true}
		pkgWidth = sql.NullFloat64{Float64: input.PackageDimensions.Width, Valid: true}
		pkgHeight = sql.NullFloat64{Float64: input.PackageDimensions.Height, Valid: true}
	}

	// UPDATED: Query uses price_to_tts, stock_quantity, and commission_rate
	productQuery := `
		INSERT INTO products
		(supplier_id, name, description, price_to_tts, stock_quantity, sku, 
		is_variable, status, created_at, updated_at, 
		weight, pkg_length, pkg_width, pkg_height, commission_rate)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := tx.Exec(productQuery,
		product.SupplierID, product.Name, product.Description,
		product.PriceToTTS, product.StockQuantity, product.SKU,
		product.IsVariable, product.Status, product.CreatedAt, product.UpdatedAt,
		weight, pkgLength, pkgWidth, pkgHeight, product.CommissionRate,
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

	// 9. --- Handle Variants ---
	if product.IsVariable {
		variantQuery := `
			INSERT INTO product_variants
			(product_id, sku, price_to_tts, stock_quantity, options, commission_rate, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

		for _, variant := range input.Variants {
			// Marshal the Options struct into a JSON string for the DB column
			optionsJSON, err := json.Marshal(variant.Options)
			if err != nil {
				// We commit the product, but fail on variant creation
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal variant options"})
				return
			}

			// Handle variant-specific CommissionRate (nullable)
			var variantCommissionRate sql.NullFloat64
			if variant.CommissionRate != nil {
				variantCommissionRate = sql.NullFloat64{Float64: *variant.CommissionRate, Valid: true}
			}

			// Handle SKU (nullable)
			sku := sql.NullString{String: variant.SKU, Valid: variant.SKU != ""}

			_, err = tx.Exec(variantQuery,
				productID,
				sku,
				variant.Price,
				variant.Stock,
				string(optionsJSON),
				variantCommissionRate,
				time.Now(),
				time.Now(),
			)
			if err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert product variant"})
				return
			}
		}
	}

	// 10. --- TODO: Notify Managers (Phase 4.4) ---
	// If product.Status == "pending", we would add logic here
	// to send a notification to all users with the 'manager' role.

	// 11. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// 12. --- Send Success Response ---
	c.JSON(http.StatusCreated, gin.H{
		"message":   "Product created successfully",
		"productId": product.ID,
	})
}

// getOrCreateBrandID (No changes needed in this helper)
func (h *Handlers) getOrCreateBrandID(tx *sql.Tx, brandID *int64, brandName string) (int64, error) {
	// Case 1: An existing BrandID was provided.
	if brandID != nil {
		var exists int
		err := tx.QueryRow("SELECT 1 FROM brands WHERE id = ?", *brandID).Scan(&exists)
		if err != nil {
			if err == sql.ErrNoRows {
				return 0, errors.New("invalid brandId: brand does not exist")
			}
			return 0, err
		}
		return *brandID, nil
	}

	// Case 2: A new BrandName was provided.
	if brandName != "" {
		var existingID int64
		slug := slug.Make(brandName)
		err := tx.QueryRow("SELECT id FROM brands WHERE slug = ?", slug).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			return 0, err
		}
		if err == nil {
			return existingID, nil
		}

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
		return newID, nil
	}

	// Case 3: Neither was provided.
	return 0, errors.New("a brandId or a new brandName is required")
}

// --- Product Retrieval ---

// GetMyProducts is the handler for GET /v1/products/supplier/me
func (h *Handlers) GetMyProducts(c *gin.Context) {
	// 1. --- Get Supplier ID ---
	userID_raw, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in context"})
		return
	}
	supplierID := userID_raw.(int64)

	// 2. --- Build Query with Filtering ---
	statusFilter := c.Query("status")

	// UPDATED: Query uses price_to_tts, stock_quantity, and commission_rate
	query := `
		SELECT 
			id, supplier_id, sku, name, description, price_to_tts, stock_quantity, 
			is_variable, status, created_at, updated_at,
			weight, pkg_length, pkg_width, pkg_height, commission_rate
		FROM products
		WHERE supplier_id = ?`

	args := []interface{}{supplierID}

	if statusFilter != "" {
		query += " AND status = ?"
		args = append(args, statusFilter)
	}

	query += " ORDER BY created_at DESC"

	// 3. --- Execute Query ---
	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	// 4. --- Scan Rows into Slice ---
	var products []*models.Product

	for rows.Next() {
		var product models.Product
		// UPDATED: Scan uses &product.PriceToTTS, &product.StockQuantity, &product.CommissionRate
		if err := rows.Scan(
			&product.ID,
			&product.SupplierID,
			&product.SKU,
			&product.Name,
			&product.Description,
			&product.PriceToTTS,
			&product.StockQuantity,
			&product.IsVariable,
			&product.Status,
			&product.CreatedAt,
			&product.UpdatedAt,
			&product.Weight,
			&product.PkgLength,
			&product.PkgWidth,
			&product.PkgHeight,
			&product.CommissionRate,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan product row"})
			return
		}
		products = append(products, &product)
	}

	if err = rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error iterating product rows"})
		return
	}

	// 5. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"products": products,
	})
}

// --- Product Update ---

// UpdateProductInput uses pointers (*) so we can tell nil vs. zero-value.
type UpdateProductInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status,omitempty" binding:"omitempty,oneof=draft private_inventory pending"`

	// --- Relational Data ---
	BrandName *string `json:"brandName"`
	BrandID   *int64  `json:"brandId"`

	// We expect the *full* new list of category IDs
	CategoryIDs *[]int64 `json:"categoryIds"`

	// --- Simple Product Update ---
	// Note: We re-use SimpleProductInput which now includes CommissionRate
	SimpleProduct *SimpleProductInput `json:"simpleProduct,omitempty"`

	// --- Variable Product Update ---
	// We will handle this later. For now, they can update top-level commission.
	Variants       []VariantInput `json:"variants,omitempty"`
	CommissionRate *float64       `json:"commissionRate,omitempty" binding:"omitempty,gte=0"`

	// --- Shipping Fields ---
	Weight            *float64                `json:"weight" binding:"omitempty,gt=0"`
	PackageDimensions *PackageDimensionsInput `json:"packageDimensions,omitempty"`
}

// UpdateProduct is the handler for PUT /v1/products/:id
func (h *Handlers) UpdateProduct(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)

	productIDStr := c.Param("id")

	// 2. --- Verify Ownership ---
	// UPDATED: Query uses price_to_tts
	var currentProduct models.Product
	err := h.DB.QueryRow("SELECT id, status, price_to_tts, is_variable FROM products WHERE id = ? AND supplier_id = ?", productIDStr, supplierID).Scan(
		&currentProduct.ID,
		&currentProduct.Status,
		&currentProduct.PriceToTTS, // UPDATED
		&currentProduct.IsVariable,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or you do not have permission to edit it"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error checking ownership"})
		return
	}

	// 3. --- Bind & Validate JSON ---
	var input UpdateProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 4. --- Begin Database Transaction ---
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// 4.5 --- Price Change Validation (Roadmap 5.3) ---
	if currentProduct.Status == "published" && !currentProduct.IsVariable && input.SimpleProduct != nil {
		// UPDATED: Check against PriceToTTS
		if input.SimpleProduct.Price != currentProduct.PriceToTTS {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "You cannot change the price of a 'published' product. Please use the 'Request Price Change' feature.",
			})
			return
		}
	}
	// TODO: Add logic for variable product price change blocking later

	// 5. --- Dynamically Build UPDATE Query ---
	querySet := "updated_at = ?"
	queryArgs := []interface{}{time.Now()}

	if input.Name != nil {
		querySet += ", name = ?"
		queryArgs = append(queryArgs, *input.Name)
	}
	if input.Description != nil {
		querySet += ", description = ?"
		queryArgs = append(queryArgs, *input.Description)
	}
	if input.Status != nil {
		querySet += ", status = ?"
		queryArgs = append(queryArgs, *input.Status)
	}
	if input.Weight != nil {
		querySet += ", weight = ?"
		queryArgs = append(queryArgs, *input.Weight)
	}
	if input.PackageDimensions != nil {
		querySet += ", pkg_length = ?, pkg_width = ?, pkg_height = ?"
		queryArgs = append(queryArgs, input.PackageDimensions.Length, input.PackageDimensions.Width, input.PackageDimensions.Height)
	}

	// Handle Simple vs Variable updates
	if !currentProduct.IsVariable && input.SimpleProduct != nil {
		// Simple Product Update
		// UPDATED: Uses price_to_tts and stock_quantity
		querySet += ", price_to_tts = ?, stock_quantity = ?, sku = ?"
		queryArgs = append(queryArgs, input.SimpleProduct.Price, input.SimpleProduct.Stock, input.SimpleProduct.SKU)

		// NEW: Update commission rate if provided
		if input.SimpleProduct.CommissionRate != nil {
			querySet += ", commission_rate = ?"
			queryArgs = append(queryArgs, *input.SimpleProduct.CommissionRate)
		}
	} else if currentProduct.IsVariable && input.CommissionRate != nil {
		// Variable Product Update
		// NEW: Update top-level commission rate if provided
		querySet += ", commission_rate = ?"
		queryArgs = append(queryArgs, *input.CommissionRate)
	}
	// TODO: Handle input.Variants update here later

	// Add the product ID for the WHERE clause
	queryArgs = append(queryArgs, productIDStr)

	query := fmt.Sprintf("UPDATE products SET %s WHERE id = ?", querySet)

	_, err = tx.Exec(query, queryArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update core product details"})
		return
	}

	// 6. --- Handle Category Update ---
	if input.CategoryIDs != nil {
		_, err := tx.Exec("DELETE FROM product_categories WHERE product_id = ?", productIDStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear old categories"})
			return
		}

		categoryQuery := `INSERT INTO product_categories (product_id, category_id) VALUES (?, ?)`
		for _, catID := range *input.CategoryIDs {
			_, err := tx.Exec(categoryQuery, productIDStr, catID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link new category"})
				return
			}
		}
	}

	// 7. --- Handle Brand Update ---
	if input.BrandID != nil || (input.BrandName != nil && *input.BrandName != "") {
		brandNameStr := ""
		if input.BrandName != nil {
			brandNameStr = *input.BrandName
		}

		newBrandID, err := h.getOrCreateBrandID(tx, input.BrandID, brandNameStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		_, err = tx.Exec("UPDATE product_brands SET brand_id = ? WHERE product_id = ?", newBrandID, productIDStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update brand link"})
			return
		}
	}

	// 8. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// 9. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"message": "Product updated successfully",
	})
}

// --- Product Deletion ---

// DeleteProduct (No changes needed in this handler)
func (h *Handlers) DeleteProduct(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)

	productIDStr := c.Param("id")

	// 2. --- Execute Deletion with Ownership Check ---
	query := "DELETE FROM products WHERE id = ? AND supplier_id = ?"

	result, err := h.DB.Exec(query, productIDStr, supplierID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete product"})
		return
	}

	// 3. --- Check Rows Affected ---
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check affected rows"})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or you do not have permission to delete it"})
		return
	}

	// 4. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"message": "Product deleted successfully",
	})
}

// --- Public Product Search ---

// SearchProducts is the handler for GET /v1/products/search
func (h *Handlers) SearchProducts(c *gin.Context) {
	// 1. --- Define Query Parameters ---
	q := c.Query("q")
	categoryID := c.Query("category")
	brandID := c.Query("brand")
	minPrice := c.Query("min_price")
	maxPrice := c.Query("max_price")

	// 2. --- Dynamically Build Query ---
	var queryBuilder strings.Builder
	var args []interface{}

	// UPDATED: Query uses price_to_tts, stock_quantity, and commission_rate
	queryBuilder.WriteString(`
		SELECT DISTINCT
			p.id, p.supplier_id, p.sku, p.name, p.description,
			p.price_to_tts, p.stock_quantity, p.is_variable, p.status,
			p.created_at, p.updated_at,
			p.weight, p.pkg_length, p.pkg_width, p.pkg_height, p.commission_rate
		FROM products p
	`)

	if categoryID != "" {
		queryBuilder.WriteString(" JOIN product_categories pc ON p.id = pc.product_id")
	}
	if brandID != "" {
		queryBuilder.WriteString(" JOIN product_brands pb ON p.id = pb.product_id")
	}

	queryBuilder.WriteString(" WHERE p.status = ?")
	args = append(args, "published")

	if categoryID != "" {
		queryBuilder.WriteString(" AND pc.category_id = ?")
		args = append(args, categoryID)
	}
	if brandID != "" {
		queryBuilder.WriteString(" AND pb.brand_id = ?")
		args = append(args, brandID)
	}
	// UPDATED: Filter uses p.price_to_tts
	if minPrice != "" {
		queryBuilder.WriteString(" AND p.price_to_tts >= ?")
		args = append(args, minPrice)
	}
	if maxPrice != "" {
		queryBuilder.WriteString(" AND p.price_to_tts <= ?")
		args = append(args, maxPrice)
	}
	if q != "" {
		queryBuilder.WriteString(" AND (p.name LIKE ? OR p.description LIKE ?)")
		searchTerm := "%" + q + "%"
		args = append(args, searchTerm, searchTerm)
	}

	queryBuilder.WriteString(" ORDER BY p.created_at DESC")

	// 3. --- Execute Query ---
	query := queryBuilder.String()
	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed", "details": err.Error()})
		return
	}
	defer rows.Close()

	// 4. --- Scan Rows into Slice ---
	var products []*models.Product
	for rows.Next() {
		var product models.Product
		// UPDATED: Scan uses &product.PriceToTTS, &product.StockQuantity, &product.CommissionRate
		if err := rows.Scan(
			&product.ID,
			&product.SupplierID,
			&product.SKU,
			&product.Name,
			&product.Description,
			&product.PriceToTTS,
			&product.StockQuantity,
			&product.IsVariable,
			&product.Status,
			&product.CreatedAt,
			&product.UpdatedAt,
			&product.Weight,
			&product.PkgLength,
			&product.PkgWidth,
			&product.PkgHeight,
			&product.CommissionRate,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan product row"})
			return
		}
		products = append(products, &product)
	}
	if err = rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error iterating product rows"})
		return
	}

	// 5. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"products": products,
	})
}

//
// --- Price Appeal Handlers ---
//

// RequestPriceChangeInput (No changes needed)
type RequestPriceChangeInput struct {
	NewPrice float64 `json:"newPrice" binding:"required,gt=0"`
	Reason   string  `json:"reason,omitempty"`
}

// RequestPriceChange is the handler for POST /v1/products/:id/request-price-change
func (h *Handlers) RequestPriceChange(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)
	productIDStr := c.Param("id")

	// 2. --- Bind & Validate JSON ---
	var input RequestPriceChangeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Begin Transaction ---
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// 4. --- Get Product & Verify Ownership/Status ---
	var currentProduct models.Product
	// UPDATED: Query uses price_to_tts
	query := `
		SELECT id, supplier_id, price_to_tts, status 
		FROM products 
		WHERE id = ? FOR UPDATE
	`
	// UPDATED: Scan uses &product.PriceToTTS
	err = tx.QueryRow(query, productIDStr).Scan(
		&currentProduct.ID,
		&currentProduct.SupplierID,
		&currentProduct.PriceToTTS,
		&currentProduct.Status,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error checking product"})
		return
	}

	if currentProduct.SupplierID != supplierID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You do not have permission to modify this product"})
		return
	}
	if currentProduct.Status != "published" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Price appeals can only be made for 'published' products. Please edit your 'draft' product directly."})
		return
	}

	// UPDATED: Check against PriceToTTS
	if currentProduct.PriceToTTS == input.NewPrice {
		c.JSON(http.StatusBadRequest, gin.H{"error": "The new price must be different from the current price"})
		return
	}

	// 5. --- Check for Existing Pending Appeal ---
	var pendingCount int
	checkQuery := "SELECT COUNT(*) FROM price_appeals WHERE product_id = ? AND status = 'pending'"
	err = tx.QueryRow(checkQuery, productIDStr).Scan(&pendingCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check for pending appeals"})
		return
	}
	if pendingCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "An appeal for this product is already pending review."})
		return
	}

	// 6. --- Create the Price Appeal ---
	var nullReason sql.NullString
	if input.Reason != "" {
		nullReason = sql.NullString{String: input.Reason, Valid: true}
	}

	// UPDATED: Uses currentProduct.PriceToTTS as old_price
	appealQuery := `
		INSERT INTO price_appeals
		(product_id, supplier_id, old_price, new_price, reason, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)`

	now := time.Now()
	_, err = tx.Exec(appealQuery,
		currentProduct.ID,
		supplierID,
		currentProduct.PriceToTTS, // UPDATED
		input.NewPrice,
		nullReason,
		now,
		now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create price appeal"})
		return
	}

	// 7. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// 8. --- Send Success Response ---
	c.JSON(http.StatusCreated, gin.H{
		"message": "Price appeal submitted successfully and is pending review.",
	})
}
