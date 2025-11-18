package main

import (
	"log"
	"os"

	"github.com/01moynul/taptosell-golang/internal/database"
	"github.com/01moynul/taptosell-golang/internal/handlers"
	"github.com/01moynul/taptosell-golang/internal/routes"
	"github.com/joho/godotenv" // ADDED: Package to load .env file
)

func main() {
	// 0. --- Load Environment Variables (.env) ---
	// If the .env file exists in the root, this loads all key/value pairs
	// into the process environment (os.Getenv()).
	if err := godotenv.Load(); err != nil {
		// Log a warning if the file is missing, but don't crash,
		// as production might rely on injected environment variables.
		log.Println("WARNING: Could not find or load .env file. Relying on system environment variables.")
	}

	// 1. --- Main Database Connection (Read/Write) ---
	db, err := database.OpenDB()
	if err != nil {
		log.Fatalf("Failed to connect to primary database: %v", err)
	}
	defer db.Close()

	// 2. --- AI Database Connection (Read-Only) ---
	// This uses the restricted 'taptosell_ai_readonly' user.
	readOnlyDSN := os.Getenv("DB_DSN_READONLY")
	if readOnlyDSN == "" {
		log.Fatalf("CRITICAL ERROR: DB_DSN_READONLY environment variable is not set. Cannot run AI components.")
	}

	// We create a separate, isolated connection pool for security
	dbReadOnly, err := database.OpenDBWithDSN(readOnlyDSN)
	if err != nil {
		log.Fatalf("CRITICAL ERROR: Failed to connect to AI read-only database: %v", err)
	}
	defer dbReadOnly.Close()

	// --- Application Setup ---
	// We inject BOTH connections into the Handlers struct.
	app := &handlers.Handlers{
		DB:         db,         // Primary Read/Write connection
		DBReadOnly: dbReadOnly, // New Read-Only connection for AI security
	}

	// --- Router Setup ---
	router := routes.SetupRouter(app)

	// --- Start Server ---
	log.Println("Starting TapToSell v2 API server on port 8080...")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// Note: database.OpenDBWithDSN is assumed to be defined in internal/database/database.go
// and will be created in the next step.
