package main

import (
	"log" // Import the standard 'log' package for logging errors
	"net/http"

	"github.com/01moynul/taptosell-golang/internal/database" // Import our new database package
	"github.com/gin-gonic/gin"
)

func main() {
	// --- Database Connection ---
	// Call our OpenDB() function from the 'database' package.
	db, err := database.OpenDB()
	if err != nil {
		// If we can't connect to the DB, our app can't run.
		// log.Fatal() prints the error and stops the application.
		log.Fatal(err)
	}
	// 'defer' schedules this to run just before the 'main' function exits.
	// This ensures our database connection pool is always closed gracefully.
	defer db.Close()

	// --- Server Setup ---
	// 1. Create a new Gin router
	router := gin.Default()

	// 2. Define our /v1/ping route
	router.GET("/v1/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong! The TapToSell v2 API is running.",
		})
	})

	// 3. Start the web server
	log.Println("Starting TapToSell v2 API server on port 8080...")
	router.Run(":8080")
}
