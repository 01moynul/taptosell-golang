package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// UploadFile handles POST /v1/upload
// It saves the file to a local "uploads" folder and returns the URL.
func (h *Handlers) UploadFile(c *gin.Context) {
	// 1. Get the file from the request
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	// 2. Create "uploads" directory if it doesn't exist
	uploadPath := "./uploads"
	if _, err := os.Stat(uploadPath); os.IsNotExist(err) {
		os.Mkdir(uploadPath, 0755)
	}

	// 3. Generate a safe unique filename (uuid + extension)
	ext := filepath.Ext(file.Filename)
	newFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	savePath := filepath.Join(uploadPath, newFilename)

	// 4. Save the file
	if err := c.SaveUploadedFile(file, savePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// 5. Return the public URL
	// Get the dynamic base URL from .env
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	// Construct the URL dynamically
	publicURL := fmt.Sprintf("%s/uploads/%s", baseURL, newFilename)

	c.JSON(http.StatusOK, gin.H{
		"url": publicURL,
	})
}
