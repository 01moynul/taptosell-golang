package models

import "time"

// Cart defines the struct for the 'carts' table
type Cart struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"userId" db:"user_id"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// CartItem defines the struct for the 'cart_items' table
type CartItem struct {
	ID        int64     `json:"id" db:"id"`
	CartID    int64     `json:"cartId" db:"cart_id"`
	ProductID int64     `json:"productId" db:"product_id"`
	Quantity  int       `json:"quantity" db:"quantity"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}
