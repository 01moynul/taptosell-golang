package models

import (
	"database/sql"
	"time"
)

// --- Domain Models ---

type Category struct {
	ID        int64         `json:"id" db:"id"`
	Name      string        `json:"name" db:"name"`
	Slug      string        `json:"slug" db:"slug"`
	ParentID  sql.NullInt64 `json:"parentId,omitempty" db:"parent_id"`
	CreatedAt time.Time     `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time     `json:"updatedAt" db:"updated_at"`

	// Virtual Field (Not in DB) - Used for constructing the Tree View in the UI
	Children []Category `json:"children,omitempty" db:"-"`
}

type Brand struct {
	ID        int64     `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Slug      string    `json:"slug" db:"slug"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// --- API Input/Output Structs ---

type CreateCategoryInput struct {
	Name     string `json:"name" binding:"required"`
	ParentID *int64 `json:"parentId"` // Pointer allows sending null for root categories
}

type CreateBrandInput struct {
	Name string `json:"name" binding:"required"`
}
