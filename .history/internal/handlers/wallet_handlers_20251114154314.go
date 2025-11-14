package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

//
// --- Wallet Core Functions ---
//
// These are the internal functions our app will use.
// They are not handlers themselves.
//

// GetWalletBalance calculates a user's current wallet balance.
// It accepts either a *sql.DB or *sql.Tx so it can be used
// inside or outside of a transaction.
func (h *Handlers) GetWalletBalance(db *sql.DB, userID int64) (float64, error) {
	var balance sql.NullFloat64 // Use NullFloat64 to handle users with 0 transactions

	query := "SELECT SUM(amount) FROM wallet_transactions WHERE user_id = ?"

	err := db.QueryRow(query, userID).Scan(&balance)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0.0, nil // No rows means 0 balance
		}
		return 0.0, err
	}

	if !balance.Valid {
		return 0.0, nil // SUM(NULL) is NULL, so treat as 0
	}

	return balance.Float64, nil
}

// AddWalletTransaction creates a new transaction record.
// This is the *only* function that should be used to modify a balance.
// It MUST be called from within a transaction (tx).
func (h *Handlers) AddWalletTransaction(tx *sql.Tx, userID int64, txType string, amount float64, details string) error {

	// Add a safeguard: order transactions must be negative
	if txType == "order" && amount > 0 {
		amount = -amount
	}

	query := `
		INSERT INTO wallet_transactions
		(user_id, type, amount, details, created_at)
		VALUES (?, ?, ?, ?, ?)`

	_, err := tx.Exec(query, userID, txType, amount, details, time.Now())
	if err != nil {
		return fmt.Errorf("failed to add wallet transaction: %w", err)
	}

	// Note: We are NOT caching the balance as in the WP blueprint.
	// A live SUM() query is safer and fast enough with proper indexing.

	return nil
}

//
// --- Wallet HTTP Handlers ---
//

// GetMyWallet is the handler for GET /v1/dropshipper/wallet
// It returns the user's current balance and transaction history.
func (h *Handlers) GetMyWallet(c *gin.Context) {
	// 1. --- Get User ID ---
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)

	// 2. --- Get Current Balance ---
	balance, err := h.GetWalletBalance(h.DB, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get wallet balance"})
		return
	}

	// 3. --- Get Transaction History ---
	// (We will add transaction history retrieval here in a future step)

	// 4. --- Send Response ---
	c.JSON(http.StatusOK, gin.H{
		"currentBalance": balance,
		"transactions":   []string{}, // Placeholder
	})
}
