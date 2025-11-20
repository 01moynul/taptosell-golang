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

// --- Inputs ---

type VariantInput struct {
	SKU            string                        `json:"sku"`
	Price          float64                       `json:"price" binding:"gte=0"`
	Stock          int                           `json:"stock" binding:"gte=0"`
	SRP            float64                       `json:"srp"`
	Options        []models.ProductVariantOption `json:"options" binding:"omitempty,min=1"`
	CommissionRate *float64                      `json:"commissionRate,omitempty" binding:"omitempty,gte=0"`
}

type SimpleProductInput struct {
	SKU            string   `json:"sku"`
	Price          float64  `json:"price" binding:"gte=0"`
	Stock          int      `json:"stock" binding:"gte=0"`
	SRP            float64  `json:"srp"`
	CommissionRate *float64 `json:"commissionRate,omitempty" binding:"omitempty,gte=0"`
}

type PackageDimensionsInput struct {
	Length float64 `json:"length" binding:"gte=0"`
	Width  float64 `json:"width" binding:"gte=0"`
	Height float64 `json:"height" binding:"gte=0"`
}

// CreateProductInput - Updated for Phase 8.2
type CreateProductInput struct {
	Name        string  `json:"name" binding:"required"`
	Description string  `json:"description"`
	Status      string  `json:"status" binding:"required,oneof=draft private_inventory pending"`
	BrandName   string  `json:"brandName"`
	BrandID     *int64  `json:"brandId"`
	CategoryIDs []int64 `json:"category_ids"`
	IsVariable  bool    `json:"isVariable"`

	// --- New Media Fields ---
	Images          []string               `json:"images"` // Array of URLs
	VideoURL        string                 `json:"videoUrl"`
	SizeChart       map[string]interface{} `json:"sizeChart"`       // Flexible object
	VariationImages map[string]string      `json:"variationImages"` // {"Red": "url"}

	SimpleProduct *SimpleProductInput `json:"simpleProduct,omitempty"`
	Variants      []VariantInput      `json:"variants,omitempty"`

	Weight            *float64                `json:"weight" binding:"omitempty,gt=0"`
	PackageDimensions *PackageDimensionsInput `json:"packageDimensions,omitempty"`
	CommissionRate    *float64                `json:"commissionRate,omitempty" binding:"omitempty,gte=0"`
}

// CreateProduct Handler
func (h *Handlers) CreateProduct(c *gin.Context) {
	userID_raw, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found"})
		return
	}
	supplierID := userID_raw.(int64)

	var input CreateProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// --- 1. Validation Logic ---
	isDraft := input.Status == "draft" || input.Status == "private_inventory"
	if !isDraft {
		// STRICT VALIDATION for Pending/Active
		if input.Description == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Description is required for submission."})
			return
		}
		if len(input.CategoryIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Category is required."})
			return
		}
		if input.BrandID == nil && input.BrandName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Brand is required."})
			return
		}
		// NEW: Image Validation
		if len(input.Images) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "At least 1 product image is required."})
			return
		}

		if input.IsVariable {
			if len(input.Variants) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Variants are required."})
				return
			}
		} else {
			if input.SimpleProduct == nil || input.SimpleProduct.Price <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Price is required."})
				return
			}
		}
	}

	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DB Transaction failed"})
		return
	}
	defer tx.Rollback()

	// --- 2. Handle Brand ---
	var brandID int64
	var brandNameLegacy string = "Generic"
	if input.BrandID != nil || input.BrandName != "" {
		brandID, err = h.getOrCreateBrandID(tx, input.BrandID, input.BrandName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if input.BrandName != "" {
			brandNameLegacy = input.BrandName
		}
	}

	// --- 3. Prepare JSON Data ---
	// We marshal the arrays/maps to strings for the DB
	imagesJSON, _ := json.Marshal(input.Images)
	sizeChartJSON, _ := json.Marshal(input.SizeChart)
	variationImagesJSON, _ := json.Marshal(input.VariationImages)

	var videoURL sql.NullString
	if input.VideoURL != "" {
		videoURL = sql.NullString{String: input.VideoURL, Valid: true}
	}

	// --- 4. Prepare Product Data ---
	product := &models.Product{
		SupplierID:  supplierID,
		Name:        input.Name,
		Description: input.Description,
		IsVariable:  input.IsVariable,
		Status:      input.Status,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Logic for Price, Stock, Legacy Columns
	var srp float64 = 0
	var weightGrams int = 0
	var categoryLegacy string = "Uncategorized"

	var commissionRate sql.NullFloat64
	product.PriceToTTS = 0
	product.StockQuantity = 0

	if !input.IsVariable && input.SimpleProduct != nil {
		product.PriceToTTS = input.SimpleProduct.Price
		product.StockQuantity = input.SimpleProduct.Stock
		product.SKU = sql.NullString{String: input.SimpleProduct.SKU, Valid: input.SimpleProduct.SKU != ""}
		srp = input.SimpleProduct.SRP
		if input.SimpleProduct.CommissionRate != nil {
			commissionRate = sql.NullFloat64{Float64: *input.SimpleProduct.CommissionRate, Valid: true}
		}
	} else {
		if input.CommissionRate != nil {
			commissionRate = sql.NullFloat64{Float64: *input.CommissionRate, Valid: true}
		}
	}
	product.CommissionRate = commissionRate

	var weight, pkgLength, pkgWidth, pkgHeight sql.NullFloat64
	if input.Weight != nil {
		weight = sql.NullFloat64{Float64: *input.Weight, Valid: true}
		weightGrams = int(*input.Weight * 1000)
	}
	if input.PackageDimensions != nil {
		pkgLength = sql.NullFloat64{Float64: input.PackageDimensions.Length, Valid: true}
		pkgWidth = sql.NullFloat64{Float64: input.PackageDimensions.Width, Valid: true}
		pkgHeight = sql.NullFloat64{Float64: input.PackageDimensions.Height, Valid: true}
	}

	// --- 5. INSERT QUERY (Updated) ---
	productQuery := `
		INSERT INTO products
		(supplier_id, name, description, price_to_tts, stock_quantity, sku, 
		is_variable, status, created_at, updated_at, 
		weight, pkg_length, pkg_width, pkg_height, commission_rate,
		category, brand, srp, weight_grams,
		images, video_url, size_chart, variation_images) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := tx.Exec(productQuery,
		product.SupplierID, product.Name, product.Description,
		product.PriceToTTS, product.StockQuantity, product.SKU,
		product.IsVariable, product.Status, product.CreatedAt, product.UpdatedAt,
		weight, pkgLength, pkgWidth, pkgHeight, product.CommissionRate,
		categoryLegacy, brandNameLegacy, srp, weightGrams,
		string(imagesJSON), videoURL, string(sizeChartJSON), string(variationImagesJSON), // NEW
	)
	if err != nil {
		fmt.Printf("DB Error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert product"})
		return
	}
	productID, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get ID"})
		return
	}

	// --- 6. Link Relations ---
	if len(input.CategoryIDs) > 0 {
		catQ := `INSERT INTO product_categories (product_id, category_id) VALUES (?, ?)`
		for _, cid := range input.CategoryIDs {
			tx.Exec(catQ, productID, cid)
		}
	}
	if brandID != 0 {
		tx.Exec(`INSERT INTO product_brands (product_id, brand_id) VALUES (?, ?)`, productID, brandID)
	}

	// --- 7. Handle Variants ---
	if product.IsVariable && len(input.Variants) > 0 {
		varQ := `INSERT INTO product_variants (product_id, sku, price_to_tts, stock_quantity, options, commission_rate, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
		for _, v := range input.Variants {
			optJSON, _ := json.Marshal(v.Options)
			var vComm sql.NullFloat64
			if v.CommissionRate != nil {
				vComm = sql.NullFloat64{Float64: *v.CommissionRate, Valid: true}
			}
			sku := sql.NullString{String: v.SKU, Valid: v.SKU != ""}
			tx.Exec(varQ, productID, sku, v.Price, v.Stock, string(optJSON), vComm, time.Now(), time.Now())
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Commit failed"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Product saved", "productId": productID})
}

// getOrCreateBrandID (Helper)
func (h *Handlers) getOrCreateBrandID(tx *sql.Tx, brandID *int64, brandName string) (int64, error) {
	if brandID != nil {
		var exists int
		err := tx.QueryRow("SELECT 1 FROM brands WHERE id = ?", *brandID).Scan(&exists)
		if err != nil {
			// FIXED: lowercase "invalid"
			return 0, errors.New("invalid brandId")
		}
		return *brandID, nil
	}
	if brandName != "" {
		var existingID int64
		slug := slug.Make(brandName)
		err := tx.QueryRow("SELECT id FROM brands WHERE slug = ?", slug).Scan(&existingID)
		if err == nil {
			return existingID, nil
		}

		res, err := tx.Exec(`INSERT INTO brands (name, slug) VALUES (?, ?)`, brandName, slug)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	// FIXED: Lowercase "brand"
	return 0, errors.New("brand required")
}

// --- Product Retrieval (Same as before) ---
func (h *Handlers) GetMyProducts(c *gin.Context) {
	userID_raw, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in context"})
		return
	}
	supplierID := userID_raw.(int64)

	statusFilter := c.Query("status")

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

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	var products []*models.Product

	for rows.Next() {
		var product models.Product
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

	c.JSON(http.StatusOK, gin.H{
		"products": products,
	})
}

// --- Product Update ---

type UpdateProductInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status,omitempty" binding:"omitempty,oneof=draft private_inventory pending"`

	BrandName *string `json:"brandName"`
	BrandID   *int64  `json:"brandId"`

	CategoryIDs *[]int64 `json:"category_ids"`

	SimpleProduct  *SimpleProductInput `json:"simpleProduct,omitempty"`
	Variants       []VariantInput      `json:"variants,omitempty"`
	CommissionRate *float64            `json:"commissionRate,omitempty" binding:"omitempty,gte=0"`

	Weight            *float64                `json:"weight" binding:"omitempty,gt=0"`
	PackageDimensions *PackageDimensionsInput `json:"packageDimensions,omitempty"`
}

// UpdateProduct is the handler for PUT /v1/products/:id
func (h *Handlers) UpdateProduct(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)
	productIDStr := c.Param("id")

	var currentProduct models.Product
	err := h.DB.QueryRow("SELECT id, status, price_to_tts, is_variable FROM products WHERE id = ? AND supplier_id = ?", productIDStr, supplierID).Scan(
		&currentProduct.ID,
		&currentProduct.Status,
		&currentProduct.PriceToTTS,
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

	var input UpdateProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// 4.5 --- Price Change Validation ---
	if currentProduct.Status == "published" && !currentProduct.IsVariable && input.SimpleProduct != nil {
		if input.SimpleProduct.Price != currentProduct.PriceToTTS {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "You cannot change the price of a 'published' product. Please use the 'Request Price Change' feature.",
			})
			return
		}
	}

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

	if !currentProduct.IsVariable && input.SimpleProduct != nil {
		querySet += ", price_to_tts = ?, stock_quantity = ?, sku = ?"
		queryArgs = append(queryArgs, input.SimpleProduct.Price, input.SimpleProduct.Stock, input.SimpleProduct.SKU)
		if input.SimpleProduct.CommissionRate != nil {
			querySet += ", commission_rate = ?"
			queryArgs = append(queryArgs, *input.SimpleProduct.CommissionRate)
		}
	} else if currentProduct.IsVariable && input.CommissionRate != nil {
		querySet += ", commission_rate = ?"
		queryArgs = append(queryArgs, *input.CommissionRate)
	}

	queryArgs = append(queryArgs, productIDStr)
	query := fmt.Sprintf("UPDATE products SET %s WHERE id = ?", querySet)

	_, err = tx.Exec(query, queryArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update core product details"})
		return
	}

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

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Product updated successfully",
	})
}

// DeleteProduct and SearchProducts (Include SearchProducts and RequestPriceChange logic from previous file)
func (h *Handlers) DeleteProduct(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)

	productIDStr := c.Param("id")

	query := "DELETE FROM products WHERE id = ? AND supplier_id = ?"

	result, err := h.DB.Exec(query, productIDStr, supplierID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete product"})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check affected rows"})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or you do not have permission to delete it"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Product deleted successfully",
	})
}

func (h *Handlers) SearchProducts(c *gin.Context) {
	q := c.Query("q")
	categoryID := c.Query("category")
	brandID := c.Query("brand")
	minPrice := c.Query("min_price")
	maxPrice := c.Query("max_price")

	var queryBuilder strings.Builder
	var args []interface{}

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

	query := queryBuilder.String()
	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed", "details": err.Error()})
		return
	}
	defer rows.Close()

	var products []*models.Product
	for rows.Next() {
		var product models.Product
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

	c.JSON(http.StatusOK, gin.H{
		"products": products,
	})
}

type RequestPriceChangeInput struct {
	NewPrice float64 `json:"newPrice" binding:"required,gt=0"`
	Reason   string  `json:"reason,omitempty"`
}

func (h *Handlers) RequestPriceChange(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)
	productIDStr := c.Param("id")

	var input RequestPriceChangeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	var currentProduct models.Product
	query := `
		SELECT id, supplier_id, price_to_tts, status 
		FROM products 
		WHERE id = ? FOR UPDATE
	`
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

	if currentProduct.PriceToTTS == input.NewPrice {
		c.JSON(http.StatusBadRequest, gin.H{"error": "The new price must be different from the current price"})
		return
	}

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
		currentProduct.PriceToTTS,
		input.NewPrice,
		nullReason,
		now,
		now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create price appeal"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Price appeal submitted successfully and is pending review.",
	})
}
