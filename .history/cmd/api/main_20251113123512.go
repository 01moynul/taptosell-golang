package main

import (
	"net/http" // Package for handling HTTP

	"github.com/gin-gonic/gin" // Import the Gin framework we just installed
)

// main is the entry point for every Go application
func main() {
	// 1. Create a new Gin router with default settings
	router := gin.Default()

	// 2. Define our first API route (a "GET" request)
	// When someone visits "http://localhost:8080/v1/ping"
	router.GET("/v1/ping", func(c *gin.Context) {

		// 3. Send back a JSON response
		c.JSON(http.StatusOK, gin.H{
			"message": "pong! The TapToSell v2 API is running.",
		})
	})

	// 4. Start the web server on port 8080
	router.Run(":8080") // You can change this port if 8080 is in use
}
