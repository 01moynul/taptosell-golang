package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/01moynul/taptosell-golang/internal/models"
	"github.com/gin-gonic/gin"
)

//
// --- Supplier Wallet Handlers ---
//

// GetSupplierWallet is the handler for GET /v1/supplier/wallet
// It returns the supplier's available balance and pending balance.
func (h *Handlers) GetSupplierWallet(c *gin.Context) {
	// 1. --- Get Supplier ID ---
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)

	// 2. --- Get Available Balance ---
	// "Available" balance is their current wallet balance.
	availableBalance, err := h.GetWalletBalance(h.DB, supplierID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get available wallet balance"})
		return
	}

	// 3. --- Get Pending Balance (from 'shipped' orders) ---
	// "Pending" balance is the total value of orders that have been
	// marked as 'shipped' but not yet 'completed' (i.e., not yet paid out).
	// We need to query the 'order_items' and 'orders' tables.
	var pendingBalance sql.NullFloat64
	query := `
		SELECT SUM(oi.unit_price * oi.quantity)
		FROM order_items oi
		JOIN orders o ON oi.order_id = o.id
		JOIN products p ON oi.product_id = p.id
		WHERE p.supplier_id = ? AND o.status = 'shipped'
	`
	// We'll also need to factor in commission here in the future,
	// but for now, this gets the total value.

	err = h.DB.QueryRow(query, supplierID).Scan(&pendingBalance)
	if err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get pending balance"})
		return
	}

	// 4. --- Get Withdrawal History ---
	// We'll also fetch recent withdrawal requests
	historyQuery := `
		SELECT id, amount, status, rejection_reason, created_at
		FROM withdrawal_requests
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT 20
	`
	rows, err := h.DB.Query(historyQuery, supplierID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get withdrawal history"})
		return
	}
	defer rows.Close()

	var history []models.WithdrawalRequest
	for rows.Next() {
		var req models.WithdrawalRequest
		if err := rows.Scan(&req.ID, &req.Amount, &req.Status, &req.RejectionReason, &req.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan withdrawal history"})
			return
		}
		history = append(history, req)
	}

	// 5. --- Send Response ---
	c.JSON(http.StatusOK, gin.H{
		"availableBalance": availableBalance,
		"pendingBalance":   pendingBalance.Float64, // Will be 0.0 if NULL
		"history":          history,
	})
}

// RequestWithdrawalInput defines the JSON for a withdrawal request
type RequestWithdrawalInput struct {
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	BankDetails string  `json:"bankDetails" binding:"required"`
}

// RequestWithdrawal is the handler for POST /v1/supplier/wallet/request-withdrawal
func (h *Handlers) RequestWithdrawal(c *gin.Context) {
	// 1. --- Get Supplier ID ---
	userID_raw, _ := c.Get("userID")
	supplierID := userID_raw.(int64)

	// 2. --- Bind & Validate JSON ---
	var input RequestWithdrawalInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. --- Begin Transaction ---
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// 4. --- Check Available Balance ---
	// We pass the transaction 'tx' to GetWalletBalance to ensure
	// our balance check is part of the atomic operation.
	availableBalance, err := h.GetWalletBalance(tx, supplierID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get wallet balance"})
		return
	}

	if availableBalance < input.Amount {
		c.JSON(http.StatusConflict, gin.H{"error": "Insufficient funds. Your available balance is lower than the requested amount."})
		return
	}

	// 5. --- Create 'withdrawal_requests' Record ---
	reqQuery := `
		INSERT INTO withdrawal_requests
		(user_id, amount, status, bank_details, created_at, updated_at)
		VALUES (?, ?, 'pending', ?, ?, ?)`

	now := time.Now()
	result, err := tx.Exec(reqQuery, supplierID, input.Amount, input.BankDetails, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create withdrawal request"})
		return
	}
	requestID, _ := result.LastInsertId()

	// 6. --- Add Negative Wallet Transaction ---
	// As per our WP audit, this deducts the funds from the "available"
	// balance immediately, holding them in "pending" status.
	details := fmt.Sprintf("Pending withdrawal (Request ID: %d)", requestID)
	err = h.AddWalletTransaction(tx, supplierID, "withdrawal", -input.Amount, details)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add wallet transaction"})
		return
	}

	// 7. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// 8. --- Send Success Response ---
	c.JSON(http.StatusCreated, gin.H{
		"message": "Withdrawal request submitted successfully. The funds have been deducted from your available balance and are now pending review.",
	})
}

//
// --- Manager: Withdrawal Handlers ---
//

// GetWithdrawalRequests is the handler for GET /v1/manager/withdrawal-requests
// It retrieves all 'pending' requests for managers to review.
func (h *Handlers) GetWithdrawalRequests(c *gin.Context) {
	// 1. --- Query Database ---
	// We JOIN with the 'users' table to get the supplier's name/email
	query := `
		SELECT 
			wr.id, wr.user_id, wr.amount, wr.status, wr.bank_details, 
			wr.created_at, wr.updated_at,
			u.full_name, u.email
		FROM withdrawal_requests wr
		JOIN users u ON wr.user_id = u.id
		WHERE wr.status = 'pending'
		ORDER BY wr.created_at ASC
	`
	rows, err := h.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	// 2. --- Scan Rows ---
	var requests []*models.WithdrawalRequest
	for rows.Next() {
		var req models.WithdrawalRequest
		if err := rows.Scan(
			&req.ID,
			&req.UserID,
			&req.Amount,
			&req.Status,
			&req.BankDetails,
			&req.CreatedAt,
			&req.UpdatedAt,
			&req.SupplierName,
			&req.SupplierEmail,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan withdrawal request"})
			return
		}
		requests = append(requests, &req)
	}

	if err = rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error iterating rows"})
		return
	}

	// 3. --- Send Response ---
	c.JSON(http.StatusOK, gin.H{
		"requests": requests,
	})
}

// ProcessWithdrawalInput defines the JSON for approving/rejecting a request
type ProcessWithdrawalInput struct {
	Action          string `json:"action" binding:"required,oneof=approve reject"`
	RejectionReason string `json:"rejectionReason,omitempty"`
}

// ProcessWithdrawalRequest is the handler for PATCH /v1/manager/withdrawal-requests/:id
func (h *Handlers) ProcessWithdrawalRequest(c *gin.Context) {
	// 1. --- Get IDs & Bind Input ---
	requestID := c.Param("id")

	var input ProcessWithdrawalInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Action == "reject" && input.RejectionReason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A rejectionReason is required when rejecting a request"})
		return
	}

	// 2. --- Begin Transaction ---
	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// 3. --- Get Request Details ---
	// We must lock the row and check its status
	var req models.WithdrawalRequest
	query := "SELECT id, user_id, amount, status FROM withdrawal_requests WHERE id = ? FOR UPDATE"
	err = tx.QueryRow(query, requestID).Scan(&req.ID, &req.UserID, &req.Amount, &req.Status)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Withdrawal request not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get request details"})
		return
	}

	if req.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"error": "This request has already been processed"})
		return
	}

	// 4. --- Process Action ---
	if input.Action == "approve" {
		// Action: Approve
		// Just update the status. The funds are already deducted.
		updateQuery := "UPDATE withdrawal_requests SET status = 'approved' WHERE id = ?"
		if _, err := tx.Exec(updateQuery, requestID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve request"})
			return
		}

		// TODO: Add notification to supplier

	} else {
		// Action: Reject
		// 1. Update the request status and reason
		updateQuery := "UPDATE withdrawal_requests SET status = 'rejected', rejection_reason = ? WHERE id = ?"
		if _, err := tx.Exec(updateQuery, input.RejectionReason, requestID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject request"})
			return
		}

		// 2. Refund the money to the supplier's wallet
		// The original amount was negative, so we add a positive amount back.
		details := fmt.Sprintf("Refund for rejected withdrawal (Request ID: %d)", req.ID)
		err = h.AddWalletTransaction(tx, req.UserID, "refund", req.Amount, details)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refund wallet"})
			return
		}

		// TODO: Add notification to supplier
	}

	// 5. --- Commit Transaction ---
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// 6. --- Send Response ---
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Withdrawal request successfully %sed", input.Action),
	})
}
