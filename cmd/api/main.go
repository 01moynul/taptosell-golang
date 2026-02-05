package main

import (
	"log"
	"os"
	"time"

	"github.com/01moynul/taptosell-golang/internal/ai" // ADDED: Import AI package
	"github.com/01moynul/taptosell-golang/internal/database"
	"github.com/01moynul/taptosell-golang/internal/handlers"
	"github.com/01moynul/taptosell-golang/internal/routes"
	"github.com/joho/godotenv"
)

func main() {
	// 0. --- Load Environment Variables (.env) ---
	if err := godotenv.Load(); err != nil {
		log.Println("WARNING: Could not find or load .env file. Relying on system environment variables.")
	}

	// 1. --- Main Database Connection (Read/Write) ---
	db, err := database.OpenDB()
	if err != nil {
		log.Fatalf("Failed to connect to primary database: %v", err)
	}
	defer db.Close()

	// 2. --- AI Database Connection (Read-Only) ---
	readOnlyDSN := os.Getenv("DB_DSN_READONLY")
	if readOnlyDSN == "" {
		log.Fatalf("CRITICAL ERROR: DB_DSN_READONLY environment variable is not set. Cannot run AI components.")
	}

	dbReadOnly, err := database.OpenDBWithDSN(readOnlyDSN)
	if err != nil {
		log.Fatalf("CRITICAL ERROR: Failed to connect to AI read-only database: %v", err)
	}
	defer dbReadOnly.Close()

	// 3. --- AI Service Initialization ---
	// CORRECTED: Use the NAME of the variable, not the value.
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		log.Fatal("CRITICAL ERROR: GEMINI_API_KEY environment variable is not set.")
	}

	// FIX: Pass both geminiKey AND dbReadOnly
	aiService, err := ai.NewAIService(geminiKey, dbReadOnly)
	if err != nil {
		log.Fatalf("Failed to initialize AI Service: %v", err)
	}

	// Note: Depending on implementation, we might defer closing the client here.
	// e.g., defer aiService.Client.Close()

	// --- Application Setup ---
	// We inject ALL dependencies (DBs and AI Service) into the Handlers struct.
	app := &handlers.Handlers{
		DB:         db,         // Primary Read/Write connection
		DBReadOnly: dbReadOnly, // Read-Only connection for AI security
		AIService:  aiService,  // ADDED: Injected AI Service
	}
	// --- 4. Background Workers (Cron) ---
	// Start the "Garbage Collector" in a separate thread (Goroutine).
	// It runs every 1 hour to clean up unpaid orders.
	go func() {
		// Create a ticker that ticks every 1 hour
		ticker := time.NewTicker(10 * time.Second) // TEMPORARY TEST
		defer ticker.Stop()

		log.Println("🕒 Background Worker Started: Monitoring for overdue orders...")

		for range ticker.C {
			// This code runs every time the clock hits 1 hour
			app.ProcessOverdueOrders()
		}
	}()

	// --- Router Setup ---
	router := routes.SetupRouter(app)

	// --- Start Server ---
	log.Println("Starting TapToSell v2 API server on port 8080...")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
