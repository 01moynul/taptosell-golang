package handlers

import (
	"database/sql"

	"github.com/01moynul/taptosell-golang/internal/ai" // ADDED: Import AI package
)

// Handlers struct holds all dependencies for our handlers.
type Handlers struct {
	DB         *sql.DB       // Primary Read/Write connection
	DBReadOnly *sql.DB       // Read-Only connection
	AIService  *ai.AIService // ADDED: The new AI service instance for core AI logic
}
