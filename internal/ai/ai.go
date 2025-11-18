package ai

import (
	"context"
	"database/sql"
	"log"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// AIService holds the necessary clients and connections for AI operations.
// We hold DBReadOnly here because the main chat handler will use this service.
type AIService struct {
	GeminiClient *genai.Client
	DBReadOnly   *sql.DB // Read-only connection for data analysis queries
}

// NewAIService initializes the Gemini client and returns the service instance.
func NewAIService(dbReadOnly *sql.DB) *AIService {
	// Get API Key from environment (loaded in main.go)
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		// We expect this to be set after godotenv loads the .env file
		log.Fatal("CRITICAL ERROR: GEMINI_API_KEY environment variable is not set. Cannot initialize AI service.")
	}

	// Initialize the Gemini client using the API Key
	client, err := genai.NewClient(context.Background(), option.WithAPIKey(apiKey))
	if err != nil {
		log.Fatalf("Failed to create Gemini client: %v", err)
	}

	log.Println("Gemini AI Client initialized successfully.")

	return &AIService{
		GeminiClient: client,
		DBReadOnly:   dbReadOnly,
	}
}
