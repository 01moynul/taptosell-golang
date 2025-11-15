package models

import (
	"database/sql"
	"time"
)

// PriceAppeal is the model for the 'price_appeals' table
type PriceAppeal struct {
	ID              int64          `json:"id" db:"id"`
	ProductID       int64          `json:"productId" db:"product_id"`
	SupplierID      int64          `json:"supplierId" db:"supplier_id"`
	OldPrice        float64        `json:"oldPrice" db:"old_price"`
	NewPrice        float64        `json:"newPrice" db:"new_price"`
	Reason          sql.NullString `json:"reason,omitempty" db:"reason"`
	Status          string         `json:"status" db:"status"`
	RejectionReason sql.NullString `json:"rejectionReason,omitempty" db:"rejection_reason"`
	CreatedAt       time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time      `json:"updatedAt" db:"updated_at"`

	// These fields are not in the DB, but will be
	// populated by our handlers for the manager view.
	ProductName   string `json:"productName,omitempty" db:"-"`
	SupplierName  string `json:"supplierName,omitempty" db:"-"`
	SupplierEmail string `json:"supplierEmail,omitempty" db:"-"`
}
