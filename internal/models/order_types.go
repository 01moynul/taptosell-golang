package models

import (
	"database/sql"
	"time"
)

// Order is the model for the 'orders' table
type Order struct {
	ID        int64          `json:"id" db:"id"`
	UserID    int64          `json:"userId" db:"user_id"` // The Dropshipper
	Status    string         `json:"status" db:"status"`  // e.g., processing, on-hold, shipped
	Total     float64        `json:"total" db:"total"`
	CreatedAt time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time      `json:"updatedAt" db:"updated_at"`
	Tracking  sql.NullString `json:"tracking,omitempty" db:"tracking"`
}

// OrderItem is the model for the 'order_items' table
type OrderItem struct {
	ID        int64     `json:"id" db:"id"`
	OrderID   int64     `json:"orderId" db:"order_id"`
	ProductID int64     `json:"productId" db:"product_id"`
	Quantity  int       `json:"quantity" db:"quantity"`
	UnitPrice float64   `json:"unitPrice" db:"unit_price"` // Price at the time of purchase
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}
