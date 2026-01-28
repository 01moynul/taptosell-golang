package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/01moynul/taptosell-golang/internal/models"
	"github.com/gin-gonic/gin"
)

// --- Helper: Slugify ---
// Converts "Men's Clothing" -> "mens-clothing"
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "'", "")
	return s
}

// --- Category Handlers ---

// CreateCategory (Manager Only)
func (h *Handlers) CreateCategory(c *gin.Context) {
	var input models.CreateCategoryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	slug := slugify(input.Name)

	// Insert into DB
	query := `INSERT INTO categories (name, slug, parent_id) VALUES (?, ?, ?)`
	res, err := h.DB.Exec(query, input.Name, slug, input.ParentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create category: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()

	// Return the full object so the UI can update the tree immediately
	newCat := models.Category{
		ID:   id,
		Name: input.Name,
		Slug: slug,
	}
	// Handle the NullInt64 for parentID manually for the response
	if input.ParentID != nil {
		newCat.ParentID = sql.NullInt64{Int64: *input.ParentID, Valid: true}
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Category created", "category": newCat})
}

// GetAllCategories (Public - Returns Tree Structure)
func (h *Handlers) GetAllCategories(c *gin.Context) {
	// 1. Fetch all categories flat
	rows, err := h.DB.Query("SELECT id, name, slug, parent_id FROM categories ORDER BY name ASC")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var allCats []models.Category
	for rows.Next() {
		var cat models.Category
		// Initialize Children as empty slice so it renders as [] in JSON instead of null
		cat.Children = []models.Category{}
		if err := rows.Scan(&cat.ID, &cat.Name, &cat.Slug, &cat.ParentID); err != nil {
			continue
		}
		allCats = append(allCats, cat)
	}

	// 2. Build the Tree (Pointer Magic)
	// Create a map to look up categories by ID instantly
	catMap := make(map[int64]*models.Category)
	for i := range allCats {
		// IMPORTANT: Store a pointer to the slice element, not a copy!
		catMap[allCats[i].ID] = &allCats[i]
	}

	// 3. Assign Children to Parents
	// We iterate over the SLICE (not the map) to ensure order
	for i := range allCats {
		cat := &allCats[i] // Pointer to current category

		if cat.ParentID.Valid {
			// If this cat has a parent, find the parent in the map
			if parent, exists := catMap[cat.ParentID.Int64]; exists {
				// Append THIS cat (by value) to the parent's children
				parent.Children = append(parent.Children, *cat)
			}
		}
	}

	// 4. Extract only the Root Categories
	// Now that the parents are fully populated with children, we grab the roots.
	var rootCats []models.Category
	for _, cat := range allCats {
		if !cat.ParentID.Valid {
			// This is a root category.
			// Since we modified the structs in 'allCats' via pointers in step 3,
			// this 'cat' now includes its Children!
			rootCats = append(rootCats, cat)
		}
	}

	c.JSON(http.StatusOK, gin.H{"categories": rootCats})
}

// DeleteCategory (Manager Only)
func (h *Handlers) DeleteCategory(c *gin.Context) {
	id := c.Param("id")

	// Note: We use ON DELETE SET NULL or CASCADE in DB, but let's be safe
	_, err := h.DB.Exec("DELETE FROM categories WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete category"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Category deleted"})
}

// --- Brand Handlers ---

// CreateBrand (Manager Only)
func (h *Handlers) CreateBrand(c *gin.Context) {
	var input models.CreateBrandInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	slug := slugify(input.Name)

	res, err := h.DB.Exec("INSERT INTO brands (name, slug) VALUES (?, ?)", input.Name, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create brand"})
		return
	}

	id, _ := res.LastInsertId()
	newBrand := models.Brand{ID: id, Name: input.Name, Slug: slug}

	c.JSON(http.StatusCreated, gin.H{"message": "Brand created", "brand": newBrand})
}

// GetAllBrands (Public)
func (h *Handlers) GetAllBrands(c *gin.Context) {
	rows, err := h.DB.Query("SELECT id, name, slug FROM brands ORDER BY name ASC")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var brands []models.Brand
	for rows.Next() {
		var b models.Brand
		if err := rows.Scan(&b.ID, &b.Name, &b.Slug); err != nil {
			continue
		}
		brands = append(brands, b)
	}

	c.JSON(http.StatusOK, gin.H{"brands": brands})
}

// DeleteBrand (Manager Only)
func (h *Handlers) DeleteBrand(c *gin.Context) {
	id := c.Param("id")
	_, err := h.DB.Exec("DELETE FROM brands WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete brand"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Brand deleted"})
}
