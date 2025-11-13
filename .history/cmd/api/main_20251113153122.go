package main

import (
	"log"

	"github.com/01moynul/taptosell-golang/internal/database" // Our DB connection
	"github.com/01moynul/taptosell-golang/internal/handlers" // Our new handlers
	"github.com/01moynul/taptosell-golang/internal/routes"   // Our new router
)

func main() {
	// --- Database Connection ---
	db, err := database.OpenDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// --- Application Setup ---
	// Create an instance of our 'Handlers' struct,
	// "injecting" the database pool 'db' into it.
	app := &handlers.Handlers{
		DB: db,
	}

	// --- Router Setup ---
	// Call our new SetupRouter function, passing it our
	// 'app' (which contains the DB connection).
	router := routes.SetupRouter(app)

	// --- Start Server ---
	log.Println("Starting TapToSell v2 API server on port 8080...")
	// 'router.Run' now starts the server with all routes
	// from our 'routes.go' file.
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
