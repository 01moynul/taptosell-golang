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
// --- Notification Handlers ---
//

// AddNotification is an internal helper function to create new notifications.
// It's not a handler itself but will be called by other handlers (like ApproveProduct).
// NOTE: This function must be called from within a database transaction (tx).
func (h *Handlers) AddNotification(tx *sql.Tx, userID int64, message string, link string) error {
	// Create a NullString for the link
	var nullLink sql.NullString
	if link != "" {
		nullLink = sql.NullString{String: link, Valid: true}
	} else {
		nullLink = sql.NullString{Valid: false}
	}

	query := `
		INSERT INTO notifications
		(user_id, message, link, is_read, created_at)
		VALUES (?, ?, ?, 0, ?)`

	_, err := tx.Exec(query, userID, message, nullLink, time.Now())
	if err != nil {
		// We return a wrapped error to provide more context
		return fmt.Errorf("failed to add notification: %w", err)
	}

	return nil
}

// GetMyNotifications is the handler for GET /v1/notifications
// It retrieves all notifications for the logged-in user, newest first.
func (h *Handlers) GetMyNotifications(c *gin.Context) {
	// 1. --- Get User ID ---
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)

	// 2. --- Query Database ---
	// We'll get all notifications, with unread and newest first
	query := `
		SELECT id, user_id, message, link, is_read, created_at
		FROM notifications
		WHERE user_id = ?
		ORDER BY is_read ASC, created_at DESC
		LIMIT 50` // Limit to 50 to avoid performance issues

	rows, err := h.DB.Query(query, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	// 3. --- Scan Rows into Slice ---
	var notifications []*models.Notification
	for rows.Next() {
		var notif models.Notification
		if err := rows.Scan(
			&notif.ID,
			&notif.UserID,
			&notif.Message,
			&notif.Link,
			&notif.IsRead,
			&notif.CreatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan notification row"})
			return
		}
		notifications = append(notifications, &notif)
	}

	if err = rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error iterating notification rows"})
		return
	}

	// 4. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"notifications": notifications,
	})
}

// MarkNotificationAsRead is the handler for PATCH /v1/notifications/:id/read
// It marks a single notification as read.
func (h *Handlers) MarkNotificationAsRead(c *gin.Context) {
	// 1. --- Get IDs ---
	userID_raw, _ := c.Get("userID")
	userID := userID_raw.(int64)
	notificationID := c.Param("id")

	// 2. --- Execute Update ---
	// We update the row *only if* the notification ID matches
	// AND it belongs to the currently logged-in user.
	// This prevents a user from marking another user's notifications as read.
	query := `
		UPDATE notifications
		SET is_read = 1
		WHERE id = ? AND user_id = ?`

	result, err := h.DB.Exec(query, notificationID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update notification"})
		return
	}

	// 3. --- Check Rows Affected ---
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check affected rows"})
		return
	}

	// If 0 rows were affected, the notification either didn't exist
	// or didn't belong to this user.
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found or you do not have permission to update it"})
		return
	}

	// 4. --- Send Success Response ---
	c.JSON(http.StatusOK, gin.H{
		"message": "Notification marked as read",
	})
}
