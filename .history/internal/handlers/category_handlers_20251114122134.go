package handlers

import (
	"net/http"
	"time"

	"github.com/01moynul/taptosell-golang/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gosimple/slug" // Our new slug library
)

// CreateCategoryInput defines the JSON input for creating a category
type CreateCategoryInput struct {
	Name     string `json:"name" binding:"required"`
	ParentID *int64 `json:"parentId"` // Pointer to handle null/omitted value
}

// CreateCategory is the handler for POST /v1/categories
// TODO: We will add ManagerMiddleware protection to this route later.
func (h *Handlers) CreateCategory(c *gin.Context) {
	// 1. --- Define Input ---
	var input CreateCategoryInput

	// 2. --- Bind & Validate JSON ---
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Create Category Model ---
	category := &models.Category{
		Name:      input.Name,
		Slug:      slug.Make(input.Name), // Generate slug from name
		ParentID:  input.ParentID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 4. --- Save to Database ---
	query := `
		INSERT INTO categories
		(name, slug, parent_id, created_at, updated_at)
		VALUES
		(?, ?, ?, ?, ?)`

	args := []interface{}{
		category.Name,
		category.Slug,
		category.ParentID,
		category.CreatedAt,
		category.UpdatedAt,
	}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		// This error will likely be for a "UNIQUE constraint failed" on 'name' or 'slug'
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create category, it may already exist."})
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get new category ID"})
		return
	}
	category.ID = id

	// 5. --- Send Success Response ---
	c.JSON(http.StatusCreated, gin.H{
		"message":  "Category created successfully",
		"category": category,
	})
}

// GetAllCategories is the handler for GET /v1/categories
func (h *Handlers) GetAllCategories(c *gin.Context) {
	// 1. --- Query Database ---
	query := "SELECT id, name, slug, parent_id, created_at, updated_at FROM categories ORDER BY name ASC"

	rows, err := h.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	// 2. --- Scan Rows into Slice ---
	var categories []*models.Category
	for rows.Next() {
		var category models.Category
		if err := rows.Scan(
			&category.ID,
			&category.Name,
			&category.Slug,
			&category.ParentID,
			&category.CreatedAt,
			&category.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan category row"})
			return
		}
		categories = append(categories, &category)
	}

	// 3. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"categories": categories,
	})
}
