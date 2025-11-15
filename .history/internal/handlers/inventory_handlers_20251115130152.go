package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/01moynul/taptosell-golang/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gosimple/slug"
)

//
// --- Inventory Item Handlers (Supplier-Only) ---
//

// InventoryItemInput defines the JSON for creating/updating an inventory item
type InventoryItemInput struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
	SKU         *string `json:"sku"`
	Price       float64 `json:"price" binding:"gte=0"`
	Stock       int     `json:"stock" binding:"gte=0"`
	// We will add category/brand linking later
}

// CreateInventoryItem is the handler for POST /v1/supplier/inventory
func (h *Handlers) CreateInventoryItem(c *gin.Context) {
	// 1. --- Get User ID ---
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)

	// 2. --- Bind & Validate JSON ---
	var input InventoryItemInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Create Model ---
	item := &models.InventoryItem{
		UserID:      userID,
		Name:        input.Name,
		Description: sql.NullString{String: *input.Description, Valid: input.Description != nil},
		SKU:         sql.NullString{String: *input.SKU, Valid: input.SKU != nil},
		Price:       input.Price,
		Stock:       input.Stock,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 4. --- Save to Database ---
	query := `
		INSERT INTO inventory_items
		(user_id, name, description, sku, price, stock, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := h.DB.Exec(query,
		item.UserID, item.Name, item.Description, item.SKU,
		item.Price, item.Stock, item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create inventory item"})
		return
	}
	id, _ := result.LastInsertId()
	item.ID = id

	// 5. --- Send Response ---
	c.JSON(http.StatusCreated, gin.H{
		"message": "Inventory item created successfully",
		"item":    item,
	})
}

// GetMyInventoryItems is the handler for GET /v1/supplier/inventory
func (h *Handlers) GetMyInventoryItems(c *gin.Context) {
	// 1. --- Get User ID ---
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)

	// 2. --- Query Database ---
	query := `
		SELECT id, user_id, name, description, sku, price, stock, 
		       promoted_product_id, created_at, updated_at
		FROM inventory_items
		WHERE user_id = ?
		ORDER BY created_at DESC
	`
	rows, err := h.DB.Query(query, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	// 3. --- Scan Rows ---
	var items []*models.InventoryItem
	for rows.Next() {
		var item models.InventoryItem
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Name, &item.Description, &item.SKU,
			&item.Price, &item.Stock, &item.PromotedProductID,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan inventory item"})
			return
		}
		items = append(items, &item)
	}

	// 4. --- Send Response ---
	c.JSON(http.StatusOK, gin.H{
		"items": items,
	})
}

// UpdateInventoryItem is the handler for PUT /v1/supplier/inventory/:id
func (h *Handlers) UpdateInventoryItem(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)
	itemID := c.Param("id")

	// 2. --- Bind & Validate JSON ---
	var input InventoryItemInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Execute Update ---
	// This query updates the item *only if* the ID matches AND it belongs to the user
	query := `
		UPDATE inventory_items
		SET name = ?, description = ?, sku = ?, price = ?, stock = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`
	result, err := h.DB.Exec(query,
		input.Name,
		sql.NullString{String: *input.Description, Valid: input.Description != nil},
		sql.NullString{String: *input.SKU, Valid: input.SKU != nil},
		input.Price,
		input.Stock,
		time.Now(),
		itemID,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update item"})
		return
	}

	// 4. --- Check Rows Affected ---
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found or you do not have permission to edit it"})
		return
	}

	// 5. --- Send Response ---
	c.JSON(http.StatusOK, gin.H{"message": "Inventory item updated successfully"})
}

// DeleteInventoryItem is the handler for DELETE /v1/supplier/inventory/:id
func (h *Handlers) DeleteInventoryItem(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)
	itemID := c.Param("id")

	// 2. --- Execute Delete ---
	query := "DELETE FROM inventory_items WHERE id = ? AND user_id = ?"
	result, err := h.DB.Exec(query, itemID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete item"})
		return
	}

	// 3. --- Check Rows Affected ---
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found or you do not have permission to delete it"})
		return
	}

	// 4. --- Send Response ---
	c.JSON(http.StatusOK, gin.H{"message": "Inventory item deleted successfully"})
}

//
// --- Inventory Category Handlers (Supplier-Only) ---
//

// InventoryCategoryInput defines the JSON for creating/updating a category
type InventoryCategoryInput struct {
	Name     string `json:"name" binding:"required"`
	ParentID *int64 `json:"parentId"`
}

// CreateInventoryCategory is the handler for POST /v1/supplier/inventory/categories
func (h *Handlers) CreateInventoryCategory(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)

	var input InventoryCategoryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cat := &models.InventoryCategory{
		UserID:    userID,
		Name:      input.Name,
		Slug:      slug.Make(input.Name),
		ParentID:  input.ParentID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	query := `
		INSERT INTO inventory_categories (user_id, name, slug, parent_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`

	result, err := h.DB.Exec(query, cat.UserID, cat.Name, cat.Slug, cat.ParentID, cat.CreatedAt, cat.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create inventory category"})
		return
	}
	id, _ := result.LastInsertId()
	cat.ID = id

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Inventory category created successfully",
		"category": cat,
	})
}

// GetMyInventoryCategories is the handler for GET /v1/supplier/inventory/categories
func (h *Handlers) GetMyInventoryCategories(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)

	query := `
		SELECT id, user_id, name, slug, parent_id
		FROM inventory_categories
		WHERE user_id = ?
		ORDER BY name ASC
	`
	rows, err := h.DB.Query(query, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	var categories []*models.InventoryCategory
	for rows.Next() {
		var cat models.InventoryCategory
		if err := rows.Scan(&cat.ID, &cat.UserID, &cat.Name, &cat.Slug, &cat.ParentID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan category"})
			return
		}
		categories = append(categories, &cat)
	}

	c.JSON(http.StatusOK, gin.H{
		"categories": categories,
	})
}

//
// --- Inventory Brand Handlers (Supplier-Only) ---
//

// InventoryBrandInput defines the JSON for creating/updating a brand
type InventoryBrandInput struct {
	Name string `json:"name" binding:"required"`
}

// CreateInventoryBrand is the handler for POST /v1/supplier/inventory/brands
func (h *Handlers) CreateInventoryBrand(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)

	var input InventoryBrandInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	brand := &models.InventoryBrand{
		UserID:    userID,
		Name:      input.Name,
		Slug:      slug.Make(input.Name),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	query := `
		INSERT INTO inventory_brands (user_id, name, slug, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`

	result, err := h.DB.Exec(query, brand.UserID, brand.Name, brand.Slug, brand.CreatedAt, brand.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create inventory brand"})
		return
	}
	id, _ := result.LastInsertId()
	brand.ID = id

	c.JSON(http.StatusCreated, gin.H{
		"message": "Inventory brand created successfully",
		"brand":   brand,
	})
}

// GetMyInventoryBrands is the handler for GET /v1/supplier/inventory/brands
func (h *Handlers) GetMyInventoryBrands(c *gin.Context) {
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)

	query := `
		SELECT id, user_id, name, slug
		FROM inventory_brands
		WHERE user_id = ?
		ORDER BY name ASC
	`
	rows, err := h.DB.Query(query, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	var brands []*models.InventoryBrand
	for rows.Next() {
		var brand models.InventoryBrand
		if err := rows.Scan(&brand.ID, &brand.UserID, &brand.Name, &brand.Slug); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan brand"})
			return
		}
		brands = append(brands, &brand)
	}

	c.JSON(http.StatusOK, gin.H{
		"brands": brands,
	})
}

//
// --- Inventory Promotion Handler (Supplier-Only) ---
//

// PromoteInventoryItem is the handler for POST /v1/supplier/inventory/:id/promote
// It copies a private inventory item to the public products table for approval.
func (h *Handlers) PromoteInventoryItem(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)
	inventoryItemID := c.Param("id")

	// 2. --- Begin Transaction ---
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// 3. --- Get Inventory Item & Verify Ownership ---
	var item models.InventoryItem
	query := `
		SELECT id, user_id, name, description, sku, price, stock, promoted_product_id
		FROM inventory_items
		WHERE id = ? FOR UPDATE
	`
	err = tx.QueryRow(query, inventoryItemID).Scan(
		&item.ID, &item.UserID, &item.Name, &item.Description, &item.SKU,
		&item.Price, &item.Stock, &item.PromotedProductID,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Inventory item not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get inventory item"})
		return
	}

	// Security Check
	if item.UserID != supplierID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You do not have permission to promote this item"})
		return
	}

	// Logic Check
	if item.PromotedProductID.Valid {
		c.JSON(http.StatusConflict, gin.H{"error": "This item has already been promoted"})
		return
	}

	// 4. --- Create New Public Product ---
	// We copy the details from the inventory item to a new product.
	// The new product's status is 'pending' for manager approval.
	// We'll assume 0 commission and no shipping data for now.
	now := time.Now()
	productQuery := `
		INSERT INTO products
		(supplier_id, name, description, sku, price_to_tts, stock_quantity, 
		 is_variable, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'pending', ?, ?)`

	result, err := tx.Exec(productQuery,
		supplierID, item.Name, item.Description, item.SKU,
		item.Price, item.Stock, now, now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create public product"})
		return
	}
	newProductID, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get new product ID"})
		return
	}

	// 5. --- Link Inventory Item to New Product ---
	updateQuery := `
		UPDATE inventory_items
		SET promoted_product_id = ?, updated_at = ?
		WHERE id = ?
	`
	_, err = tx.Exec(updateQuery, newProductID, now, item.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link inventory item to product"})
		return
	}

	// 6. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// 7. --- Send Response ---
	c.JSON(http.StatusCreated, gin.H{
		"message":         "Item successfully promoted to marketplace and is pending review.",
		"inventoryItemId": item.ID,
		"newlyPromotedId": newProductID,
	})
}
