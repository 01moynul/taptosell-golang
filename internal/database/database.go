package database

import (
	"database/sql"
	"log"
	"os" // ADDED: To read the primary DSN from the environment
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// OpenDB initializes and returns the primary Read/Write connection pool.
// It now reads the DSN from the environment variable (or hardcoded fallback).
func OpenDB() (*sql.DB, error) {
	// 1. Define the Data Source Name (DSN)
	// We read the DSN from an environment variable first, falling back to the hardcoded string.
	// NOTE: You should create a DB_DSN environment variable for the primary connection later.
	dsn := os.Getenv("DB_DSN_PRIMARY")
	if dsn == "" {
		// FALLBACK: Use the hardcoded DSN if environment variable is not set.
		dsn = "root:X4#j$Ds2N749bruqtnm%MMNx1xvzrSZQwyYw33FT1%y7v!4CzPRdVr6L$nJnzcbv@tcp(127.0.0.1:3306)/taptosell_golang?parseTime=true"
	}

	// Delegate the rest of the setup to the generic function
	return OpenDBWithDSN(dsn)
}

// OpenDBWithDSN is a generic function to create and configure a DB connection pool
// using any provided DSN string. This is used for BOTH the primary and read-only pools.
func OpenDBWithDSN(dsn string) (*sql.DB, error) {
	// 2. Open a new connection pool.
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// 3. Configure the connection pool settings.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 4. Ping the database to verify the connection.
	err = db.Ping()
	if err != nil {
		log.Printf("Error connecting to database with DSN: %v", err)
		return nil, err
	}

	log.Println("Database connection pool established successfully (Generic DSN)")
	return db, nil
}
