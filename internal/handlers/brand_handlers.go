package handlers

import (
	"net/http"
	"time"

	"github.com/01moynul/taptosell-golang/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gosimple/slug" // We'll use this again
)

// CreateBrandInput defines the JSON input for creating a brand
type CreateBrandInput struct {
	Name string `json:"name" binding:"required"`
}

// CreateBrand is the handler for POST /v1/brands
// TODO: We will add ManagerMiddleware protection to this route later.
func (h *Handlers) CreateBrand(c *gin.Context) {
	// 1. --- Define Input ---
	var input CreateBrandInput

	// 2. --- Bind & Validate JSON ---
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Create Brand Model ---
	brand := &models.Brand{
		Name:      input.Name,
		Slug:      slug.Make(input.Name), // Generate slug from name
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 4. --- Save to Database ---
	query := `
		INSERT INTO brands
		(name, slug, created_at, updated_at)
		VALUES
		(?, ?, ?, ?)`

	args := []interface{}{
		brand.Name,
		brand.Slug,
		brand.CreatedAt,
		brand.UpdatedAt,
	}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		// This error will likely be for a "UNIQUE constraint failed" on 'name' or 'slug'
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create brand, it may already exist."})
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get new brand ID"})
		return
	}
	brand.ID = id

	// 5. --- Send Success Response ---
	c.JSON(http.StatusCreated, gin.H{
		"message": "Brand created successfully",
		"brand":   brand,
	})
}

// GetAllBrands is the handler for GET /v1/brands
func (h *Handlers) GetAllBrands(c *gin.Context) {
	// 1. --- Query Database ---
	query := "SELECT id, name, slug, created_at, updated_at FROM brands ORDER BY name ASC"

	rows, err := h.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	// 2. --- Scan Rows into Slice ---
	var brands []*models.Brand
	for rows.Next() {
		var brand models.Brand
		if err := rows.Scan(
			&brand.ID,
			&brand.Name,
			&brand.Slug,
			&brand.CreatedAt,
			&brand.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan brand row"})
			return
		}
		brands = append(brands, &brand)
	}

	// 3. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"brands": brands,
	})
}
