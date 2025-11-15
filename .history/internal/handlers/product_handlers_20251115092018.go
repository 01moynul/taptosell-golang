package handlers

import (
	"database/sql"
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

// PackageDimensionsInput matches the nested object in the React form
type PackageDimensionsInput struct {
	Length float64 `json:"length" binding:"gte=0"`
	Width  float64 `json:"width" binding:"gte=0"`
	Height float64 `json:"height" binding:"gte=0"`
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

	// --- NEW SHIPPING FIELDS ---
	Weight            *float64                `json:"weight" binding:"omitempty,gt=0"`
	PackageDimensions *PackageDimensionsInput `json:"packageDimensions,omitempty"`
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

	productQuery := `
		INSERT INTO products
		(supplier_id, name, description, price, stock, sku, is_variable, status, created_at, updated_at, weight, pkg_length, pkg_width, pkg_height)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := tx.Exec(productQuery,
		product.SupplierID, product.Name, product.Description,
		product.Price, product.Stock, product.SKU,
		product.IsVariable, product.Status, product.CreatedAt, product.UpdatedAt,
		weight, pkgLength, pkgWidth, pkgHeight,
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

// --- Product Retrieval ---

// GetMyProducts is the handler for GET /v1/products/supplier/me
// It retrieves all products belonging to the authenticated supplier.
// It can be filtered by status, e.g., /v1/products/supplier/me?status=pending
func (h *Handlers) GetMyProducts(c *gin.Context) {
	// 1. --- Get Supplier ID ---
	userID_raw, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in context"})
		return
	}
	supplierID := userID_raw.(int64)

	// 2. --- Build Query with Filtering ---
	// Get the optional 'status' query parameter from the URL
	statusFilter := c.Query("status")

	query := `
		SELECT 
			id, supplier_id, sku, name, description, price, stock, is_variable, status, created_at, updated_at,
			weight, pkg_length, pkg_width, pkg_height
		FROM products
		WHERE supplier_id = ?`

	// We use a slice of interfaces{} for our query arguments
	// because the number of arguments can change based on the filter.
	args := []interface{}{supplierID}

	if statusFilter != "" {
		// If the status filter exists, add it to the query
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
		if err := rows.Scan(
			&product.ID,
			&product.SupplierID,
			&product.SKU,
			&product.Name,
			&product.Description,
			&product.Price,
			&product.Stock,
			&product.IsVariable,
			&product.Status,
			&product.CreatedAt,
			&product.UpdatedAt,
			&product.Weight,
			&product.PkgLength,
			&product.PkgWidth,
			&product.PkgHeight,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan product row"})
			return
		}
		// TODO: In a future step, we will also query and attach the
		// categories, brands, and variants for each product here.
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

// UpdateProductInput defines the *optional* fields for updating a product.
// We use pointers (*) so we can tell the difference between a
// field being sent with a "zero" value (e.g., 0, "", false)
// and a field not being sent at all (nil).
type UpdateProductInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status,omitempty" binding:"omitempty,oneof=draft private_inventory pending"`

	// --- Relational Data ---
	BrandName *string `json:"brandName"`
	BrandID   *int64  `json:"brandId"`

	// We expect the *full* new list of category IDs
	CategoryIDs *[]int64 `json:"categoryIds"`

	// --- Simple vs. Variable Logic ---
	// Updating variants is complex, so we'll stub it for now.
	// For a simple product, they can update these:
	SimpleProduct *SimpleProductInput `json:"simpleProduct,omitempty"`
	// For variable products, we will handle variant updates later.
	Variants []VariantInput `json:"variants,omitempty"`

	// --- NEW SHIPPING FIELDS ---
	Weight            *float64                `json:"weight" binding:"omitempty,gt=0"`
	PackageDimensions *PackageDimensionsInput `json:"packageDimensions,omitempty"`
}

// UpdateProduct is the handler for PUT /v1/products/:id
func (h *Handlers) UpdateProduct(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)

	productIDStr := c.Param("id")
	// (We'll skip parsing to int64 for brevity, but in production, you'd validate this)

	// 2. --- Verify Ownership ---
	// This is a critical security check. We must ensure the product
	// exists AND belongs to the supplier trying to edit it.
	var currentProduct models.Product
	err := h.DB.QueryRow("SELECT id FROM products WHERE id = ? AND supplier_id = ?", productIDStr, supplierID).Scan(&currentProduct.ID)
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
	// Updating a product (especially its categories/brand) is a
	// multi-step process that must be atomic.
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback() // Safety net

	// 5. --- Dynamically Build UPDATE Query ---
	// We build the "SET" part of the query and the arguments list
	// based *only* on the fields the user provided (non-nil fields).
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
	if input.SimpleProduct != nil {
		querySet += ", price = ?, stock = ?, sku = ?"
		queryArgs = append(queryArgs, input.SimpleProduct.Price, input.SimpleProduct.Stock, input.SimpleProduct.SKU)
	}
	if input.Weight != nil {
		querySet += ", weight = ?"
		queryArgs = append(queryArgs, *input.Weight)
	}
	if input.PackageDimensions != nil {
		querySet += ", pkg_length = ?, pkg_width = ?, pkg_height = ?"
		queryArgs = append(queryArgs, input.PackageDimensions.Length, input.PackageDimensions.Width, input.PackageDimensions.Height)
	}

	// Add the product ID for the WHERE clause
	queryArgs = append(queryArgs, productIDStr)

	query := fmt.Sprintf("UPDATE products SET %s WHERE id = ?", querySet)

	_, err = tx.Exec(query, queryArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update core product details"})
		return
	}

	// 6. --- Handle Category Update ---
	// If the user sent a new list of categories...
	if input.CategoryIDs != nil {
		// 1. Delete all *existing* category links for this product
		_, err := tx.Exec("DELETE FROM product_categories WHERE product_id = ?", productIDStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear old categories"})
			return
		}

		// 2. Insert the new links
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
	// If the user sent a new brand (either ID or Name)...
	if input.BrandID != nil || (input.BrandName != nil && *input.BrandName != "") {
		brandNameStr := ""
		if input.BrandName != nil {
			brandNameStr = *input.BrandName
		}

		// 1. Get or create the new brand ID
		newBrandID, err := h.getOrCreateBrandID(tx, input.BrandID, brandNameStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 2. Update the 'product_brands' junction table
		_, err = tx.Exec("UPDATE product_brands SET brand_id = ? WHERE product_id = ?", newBrandID, productIDStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update brand link"})
			return
		}
	}

	// 8. --- TODO: Handle Variant Update ---
	// This would be a complex step for a future task.

	// 9. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// 10. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"message": "Product updated successfully",
	})
}

// --- Product Deletion ---

// DeleteProduct is the handler for DELETE /v1/products/:id
func (h *Handlers) DeleteProduct(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)

	productIDStr := c.Param("id")

	// 2. --- Execute Deletion with Ownership Check ---
	// We combine the delete operation and the ownership check into
	// a single, atomic query.
	// This query will only delete the product IF the id matches
	// AND the supplier_id matches.
	// We assume ON DELETE CASCADE is set for product_categories,
	// product_brands, and cart_items to clean up linked data.

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

	// If 0 rows were affected, it means no product was found
	// with that ID *and* belonging to that supplier.
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
// This is a public endpoint for browsing the main catalog.
func (h *Handlers) SearchProducts(c *gin.Context) {
	// 1. --- Define Query Parameters ---
	// We get all optional query params from the URL
	// e.g., /v1/products/search?q=shirt&category=5&brand=2&min_price=10&max_price=50
	q := c.Query("q")
	categoryID := c.Query("category")
	brandID := c.Query("brand")
	minPrice := c.Query("min_price")
	maxPrice := c.Query("max_price")

	// 2. --- Dynamically Build Query ---
	// We use strings.Builder for efficient query string construction
	var queryBuilder strings.Builder
	var args []interface{}

	// We select 'p' (products) fields.
	// We must use 'DISTINCT p.id' to avoid duplicate products
	// if a product is in multiple categories that are part of a search.
	queryBuilder.WriteString(`
		SELECT DISTINCT
			p.id, p.supplier_id, p.sku, p.name, p.description,
			p.price, p.stock, p.is_variable, p.status,
			p.created_at, p.updated_at,
			p.weight, p.pkg_length, p.pkg_width, p.pkg_height
		FROM products p
	`)

	// --- Dynamic JOINs ---
	// We only JOIN tables if we need to filter by them.
	if categoryID != "" {
		queryBuilder.WriteString(" JOIN product_categories pc ON p.id = pc.product_id")
	}
	if brandID != "" {
		queryBuilder.WriteString(" JOIN product_brands pb ON p.id = pb.product_id")
	}

	// --- Dynamic WHERE clauses ---
	// We start the WHERE clause, defaulting to only "published" products.
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
	if minPrice != "" {
		queryBuilder.WriteString(" AND p.price >= ?")
		args = append(args, minPrice)
	}
	if maxPrice != "" {
		queryBuilder.WriteString(" AND p.price <= ?")
		args = append(args, maxPrice)
	}
	if q != "" {
		// For text search, we check name AND description
		// We add '%' wildcards for a partial match
		queryBuilder.WriteString(" AND (p.name LIKE ? OR p.description LIKE ?)")
		searchTerm := "%" + q + "%"
		args = append(args, searchTerm, searchTerm)
	}

	// --- Add Ordering ---
	queryBuilder.WriteString(" ORDER BY p.created_at DESC")
	// TODO: Add pagination (LIMIT, OFFSET) in a future step

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
		if err := rows.Scan(
			&product.ID,
			&product.SupplierID,
			&product.SKU,
			&product.Name,
			&product.Description,
			&product.Price,
			&product.Stock,
			&product.IsVariable,
			&product.Status,
			&product.CreatedAt,
			&product.UpdatedAt,
			&product.Weight,
			&product.PkgLength,
			&product.PkgWidth,
			&product.PkgHeight,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan product row"})
			return
		}
		// TODO: Attach Categories, Brands, and Variants for each product
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

// RequestPriceChangeInput defines the JSON for a price appeal
type RequestPriceChangeInput struct {
	NewPrice float64 `json:"newPrice" binding:"required,gt=0"`
	Reason   string  `json:"reason,omitempty"`
}

// RequestPriceChange is the handler for POST /v1/products/:id/request-price-change
// It allows a supplier to request a price change for their *published* product.
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
	// We must lock the product row and check its status and ownership
	var currentProduct models.Product
	query := `
		SELECT id, supplier_id, price, status 
		FROM products 
		WHERE id = ? FOR UPDATE
	`
	err = tx.QueryRow(query, productIDStr).Scan(
		&currentProduct.ID,
		&currentProduct.SupplierID,
		&currentProduct.Price,
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

	// Security Check: Must be the owner
	if currentProduct.SupplierID != supplierID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You do not have permission to modify this product"})
		return
	}

	// Logic Check: Must be a 'published' product
	if currentProduct.Status != "published" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Price appeals can only be made for 'published' products. Please edit your 'draft' product directly."})
		return
	}

	// Logic Check: New price must be different
	if currentProduct.Price == input.NewPrice {
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

	appealQuery := `
		INSERT INTO price_appeals
		(product_id, supplier_id, old_price, new_price, reason, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)`

	now := time.Now()
	_, err = tx.Exec(appealQuery,
		currentProduct.ID,
		supplierID,
		currentProduct.Price,
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
