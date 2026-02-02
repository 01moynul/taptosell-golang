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
	imagesJSON, _ := json.Marshal(input.Images)
	sizeChartJSON, _ := json.Marshal(input.SizeChart)
	variationImagesJSON, _ := json.Marshal(input.VariationImages)

	// --- 4. Prepare Product Data ---
	product := &models.Product{
		SupplierID:  supplierID,
		Name:        input.Name,
		Description: input.Description,
		IsVariable:  input.IsVariable,
		Status:      input.Status,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Images:      input.Images,
	}

	var srp float64 = 0
	var weightGrams int = 0
	var categoryLegacy string = "Uncategorized"

	// [FIX]: Pointers for new Model
	var commissionRate *float64
	var sku *string

	if !input.IsVariable && input.SimpleProduct != nil {
		// SIMPLE PRODUCT
		product.PriceToTTS = input.SimpleProduct.Price
		product.StockQuantity = input.SimpleProduct.Stock

		// Assign pointers directly (No sql.NullString needed!)
		if input.SimpleProduct.SKU != "" {
			val := input.SimpleProduct.SKU
			sku = &val
		}
		srp = input.SimpleProduct.SRP
		commissionRate = input.SimpleProduct.CommissionRate

	} else if input.IsVariable && len(input.Variants) > 0 {
		// VARIABLE PRODUCT: Roll-up logic
		var totalStock int
		var minPrice float64 = input.Variants[0].Price

		for _, v := range input.Variants {
			totalStock += v.Stock
			if v.Price < minPrice {
				minPrice = v.Price
			}
		}
		product.PriceToTTS = minPrice
		product.StockQuantity = totalStock
		commissionRate = input.CommissionRate
	}

	// Assign Pointers directly
	product.CommissionRate = commissionRate
	product.SKU = sku

	// Dimensions (Direct Pointers)
	var pkgLength, pkgWidth, pkgHeight *float64
	if input.Weight != nil {
		product.Weight = input.Weight
		weightGrams = int(*input.Weight * 1000)
	}
	if input.PackageDimensions != nil {
		l := input.PackageDimensions.Length
		w := input.PackageDimensions.Width
		h := input.PackageDimensions.Height
		pkgLength = &l
		pkgWidth = &w
		pkgHeight = &h
	}

	// --- 5. INSERT QUERY ---
	productQuery := `
		INSERT INTO products
		(supplier_id, name, description, price_to_tts, stock_quantity, sku, 
		is_variable, status, created_at, updated_at, 
		weight, pkg_length, pkg_width, pkg_height, commission_rate,
		category, brand, srp, weight_grams,
		images, video_url, size_chart, variation_images) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// [FIX]: Passing pointers directly. SQL driver handles nil automatically.
	result, err := tx.Exec(productQuery,
		product.SupplierID, product.Name, product.Description,
		product.PriceToTTS, product.StockQuantity, product.SKU,
		product.IsVariable, product.Status, product.CreatedAt, product.UpdatedAt,
		product.Weight, pkgLength, pkgWidth, pkgHeight, product.CommissionRate,
		categoryLegacy, brandNameLegacy, srp, weightGrams,
		string(imagesJSON), input.VideoURL, string(sizeChartJSON), string(variationImagesJSON),
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
			var vSku *string
			if v.SKU != "" {
				s := v.SKU
				vSku = &s
			}
			// Pass pointers directly
			tx.Exec(varQ, productID, vSku, v.Price, v.Stock, string(optJSON), v.CommissionRate, time.Now(), time.Now())
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

// GetMyProducts (Updated to fetch Images)
func (h *Handlers) GetMyProducts(c *gin.Context) {
	userID_raw, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in context"})
		return
	}
	supplierID := userID_raw.(int64)

	statusFilter := c.Query("status")

	// [FIX] Added 'images' to the SELECT query
	query := `
		SELECT 
			id, supplier_id, sku, name, description, price_to_tts, stock_quantity, 
			is_variable, status, created_at, updated_at,
			weight, pkg_length, pkg_width, pkg_height, commission_rate,
			images
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
		var dbImages []byte // [FIX] Buffer for the JSON string

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
			&dbImages, // [FIX] Scan images
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan product row"})
			return
		}

		// [FIX] Parse the JSON images
		if len(dbImages) > 0 {
			json.Unmarshal(dbImages, &product.Images)
		} else {
			product.Images = []string{}
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
	if currentProduct.Status == "active" && !currentProduct.IsVariable && input.SimpleProduct != nil {
		if input.SimpleProduct.Price != currentProduct.PriceToTTS {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "You cannot change the price of a 'active' product. Please use the 'Request Price Change' feature.",
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

// [FIXED] SearchProducts with Images and Variants
func (h *Handlers) SearchProducts(c *gin.Context) {
	q := c.Query("q")
	categoryID := c.Query("category")
	brandID := c.Query("brand")
	minPrice := c.Query("min_price")
	maxPrice := c.Query("max_price")

	var queryBuilder strings.Builder
	var args []interface{}

	// 1. SELECT - Added p.images and p.variation_images
	queryBuilder.WriteString(`
        SELECT DISTINCT
            p.id, p.supplier_id, p.sku, p.name, p.description,
            p.price_to_tts, p.stock_quantity, p.srp, p.is_variable, p.status,
            p.created_at, p.updated_at,
            p.weight, p.pkg_length, p.pkg_width, p.pkg_height, p.commission_rate,
            p.images, p.variation_images
        FROM products p
    `)

	if categoryID != "" {
		queryBuilder.WriteString(" JOIN product_categories pc ON p.id = pc.product_id")
	}
	if brandID != "" {
		queryBuilder.WriteString(" JOIN product_brands pb ON p.id = pb.product_id")
	}

	// 2. Filter by 'active'
	queryBuilder.WriteString(" WHERE p.status = ?")
	args = append(args, "active")

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

	// 3. Scan Rows
	for rows.Next() {
		var product models.Product
		var dbImages, dbVariationImages []byte // Buffers for JSON columns

		if err := rows.Scan(
			&product.ID,
			&product.SupplierID,
			&product.SKU,
			&product.Name,
			&product.Description,
			&product.PriceToTTS,
			&product.StockQuantity,
			&product.SRP,
			&product.IsVariable,
			&product.Status,
			&product.CreatedAt,
			&product.UpdatedAt,
			&product.Weight,
			&product.PkgLength,
			&product.PkgWidth,
			&product.PkgHeight,
			&product.CommissionRate,
			&dbImages,          // Scan Images
			&dbVariationImages, // Scan Variation Images
		); err != nil {
			fmt.Printf("Scan Error: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan product row"})
			return
		}

		// 4. Parse JSON Columns (Ensure they aren't nil/empty)
		if len(dbImages) > 0 {
			_ = json.Unmarshal(dbImages, &product.Images)
		} else {
			product.Images = []string{}
		}

		// 5. Fetch Variants if Variable
		if product.IsVariable {
			vRows, err := h.DB.Query(`
				SELECT id, sku, price_to_tts, stock_quantity, options 
				FROM product_variants 
				WHERE product_id = ?`, product.ID)

			if err == nil {
				var variants []models.ProductVariant
				for vRows.Next() {
					var v models.ProductVariant
					var optsJSON []byte

					// Scan database columns
					err := vRows.Scan(&v.ID, &v.SKU, &v.PriceToTTS, &v.StockQuantity, &optsJSON)
					if err != nil {
						fmt.Printf("Variant Scan Error: %v\n", err)
						continue
					}

					// [FIX] Phase 8.4: Handle JSON delivery to string field
					// Convert DB bytes to string. If empty/null, provide valid JSON array string "[]"
					if len(optsJSON) > 0 && string(optsJSON) != "null" && string(optsJSON) != `""` {
						v.Options = string(optsJSON)
					} else {
						v.Options = "[]"
					}

					variants = append(variants, v)
				}
				vRows.Close()
				product.Variants = variants
			}
		}

		products = append(products, &product)
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
	if currentProduct.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Price appeals can only be made for 'active' products. Please edit your 'draft' product directly."})
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

// ProductDetailResponse matches the structure needed by the Frontend "Edit" Form
type ProductDetailResponse struct {
	ID          int64   `json:"id"`
	SupplierID  int64   `json:"supplierId"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
	IsVariable  bool    `json:"isVariable"`
	SKU         *string `json:"sku"` // For Simple Products

	// Prices & Stock
	PriceToTTS     float64  `json:"priceToTTS"`
	SRP            float64  `json:"srp"`
	StockQuantity  int      `json:"stockQuantity"`
	CommissionRate *float64 `json:"commissionRate"`

	// Dimensions
	Weight            *float64                `json:"weight"`
	PackageDimensions *PackageDimensionsInput `json:"packageDimensions"`

	// Media (Parsed from JSON)
	Images          []string               `json:"images"`
	VideoURL        string                 `json:"videoUrl"`
	SizeChart       map[string]interface{} `json:"sizeChart"`
	VariationImages map[string]string      `json:"variationImages"`

	// Relations
	BrandID     int64   `json:"brandId"`
	BrandName   string  `json:"brandName"`
	CategoryIDs []int64 `json:"category_ids"`

	// Variants
	Variants []VariantInput `json:"variants"`
}

// GetProduct (Updated for Edit Page Reliability)
func (h *Handlers) GetProduct(c *gin.Context) {
	userID_raw, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found"})
		return
	}
	userID := userID_raw.(int64)
	userRole := c.GetString("userRole")
	productID := c.Param("id")

	// 1. Fetch Core Product Data
	query := `
		SELECT 
			id, supplier_id, name, description, status, is_variable, 
			sku, price_to_tts, srp, stock_quantity, commission_rate,
			weight, pkg_length, pkg_width, pkg_height,
			images, video_url, size_chart, variation_images,
			brand
		FROM products 
		WHERE id = ?`

	var p ProductDetailResponse
	var dbImages, dbSizeChart, dbVariationImages []byte
	var dbVideoURL, dbSKU, dbBrandName sql.NullString
	var dbWeight, dbLen, dbWid, dbHgt, dbComm sql.NullFloat64

	err := h.DB.QueryRow(query, productID).Scan(
		&p.ID, &p.SupplierID, &p.Name, &p.Description, &p.Status, &p.IsVariable,
		&dbSKU, &p.PriceToTTS, &p.SRP, &p.StockQuantity, &dbComm,
		&dbWeight, &dbLen, &dbWid, &dbHgt,
		&dbImages, &dbVideoURL, &dbSizeChart, &dbVariationImages,
		&dbBrandName,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		return
	}

	// 2. Security Check
	isManager := (userRole == "manager" || userRole == "administrator")
	if !isManager && p.SupplierID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You do not have permission to view this product"})
		return
	}

	// 3. Process Nullables
	if dbSKU.Valid {
		p.SKU = &dbSKU.String
	}
	if dbVideoURL.Valid {
		p.VideoURL = dbVideoURL.String
	}
	if dbBrandName.Valid {
		p.BrandName = dbBrandName.String
	}
	if dbComm.Valid {
		val := dbComm.Float64
		p.CommissionRate = &val
	}

	// [FIX] Always initialize arrays to avoid "null" in JSON
	p.Images = []string{}
	if len(dbImages) > 0 {
		json.Unmarshal(dbImages, &p.Images)
	}

	p.SizeChart = nil
	if len(dbSizeChart) > 0 {
		json.Unmarshal(dbSizeChart, &p.SizeChart)
	}

	p.VariationImages = make(map[string]string)
	if len(dbVariationImages) > 0 {
		json.Unmarshal(dbVariationImages, &p.VariationImages)
	}

	if dbWeight.Valid {
		val := dbWeight.Float64
		p.Weight = &val
	}

	p.PackageDimensions = &PackageDimensionsInput{Length: 0, Width: 0, Height: 0}
	if dbLen.Valid {
		p.PackageDimensions.Length = dbLen.Float64
	}
	if dbWid.Valid {
		p.PackageDimensions.Width = dbWid.Float64
	}
	if dbHgt.Valid {
		p.PackageDimensions.Height = dbHgt.Float64
	}

	// 4. Fetch Linked Categories (Robust)
	p.CategoryIDs = []int64{} // Init empty
	catRows, err := h.DB.Query("SELECT category_id FROM product_categories WHERE product_id = ?", p.ID)
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var cid int64
			catRows.Scan(&cid)
			p.CategoryIDs = append(p.CategoryIDs, cid)
		}
	}

	// 5. Fetch Brand ID
	h.DB.QueryRow("SELECT brand_id FROM product_brands WHERE product_id = ?", p.ID).Scan(&p.BrandID)

	// 6. Fetch Variants
	p.Variants = []VariantInput{} // Init empty
	if p.IsVariable {
		vRows, err := h.DB.Query(`
			SELECT sku, price_to_tts, stock_quantity, options, commission_rate 
			FROM product_variants WHERE product_id = ?`, p.ID)
		if err == nil {
			defer vRows.Close()
			for vRows.Next() {
				var v VariantInput
				var vOpts []byte
				var vComm sql.NullFloat64
				var vSku sql.NullString

				vRows.Scan(&vSku, &v.Price, &v.Stock, &vOpts, &vComm)

				json.Unmarshal(vOpts, &v.Options)
				if vComm.Valid {
					val := vComm.Float64
					v.CommissionRate = &val
				}
				if vSku.Valid {
					v.SKU = vSku.String
				}

				p.Variants = append(p.Variants, v)
			}
		}
	}

	// 7. Return Final JSON
	c.JSON(http.StatusOK, gin.H{"product": p})
}
