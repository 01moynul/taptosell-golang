package handlers

import "database/sql"

// Handlers struct holds all dependencies for our handlers,
// such as the database connection pool(s).
type Handlers struct {
	DB         *sql.DB // Primary Read/Write connection
	DBReadOnly *sql.DB // ADDED: Read-Only connection for AI and sensitive reads
}
