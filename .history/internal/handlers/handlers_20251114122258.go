package handlers

import "database/sql"

// Handlers struct holds all dependencies for our handlers,
// such as the database connection pool.
type Handlers struct {
	DB *sql.DB
}
