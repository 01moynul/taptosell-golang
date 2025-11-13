package database

import (
	"database/sql" // The standard Go library for SQL operations
	"log"          // For logging errors
	"time"         // For setting connection timeouts

	_ "github.com/go-sql-driver/mysql" // The MySQL driver. Note the '_' prefix.
)

// OpenDB initializes and returns a connection pool to the database.
func OpenDB() (*sql.DB, error) {
	// 1. Define the Data Source Name (DSN)
	// This string tells the driver how to connect to our database.
	// We will hardcode it for now. In the future, we will read this from a secure .env file.
	// IMPORTANT: Replace "YOUR_64_CHAR_PASSWORD" with the actual root password you saved.
	dsn := "root:X4#j$Ds2N749bruqtnm%MMNx1xvzrSZQwyYw33FT1%y7v!4CzPRdVr6L$nJnzcbv@tcp(127.0.0.1:3306)/taptosell_golang?parseTime=true"

	// 2. Open a new connection pool.
	// 'sql.Open' doesn't actually create a connection, just prepares the pool.
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// 3. Configure the connection pool settings.
	// These are good default settings for a production web server.
	db.SetMaxOpenConns(25)                 // Max number of open connections.
	db.SetMaxIdleConns(25)                 // Max number of connections in the idle pool.
	db.SetConnMaxLifetime(5 * time.Minute) // Max time a connection can be reused.

	// 4. Ping the database to verify the connection.
	// This is the first time we actually connect to the DB.
	err = db.Ping()
	if err != nil {
		log.Printf("Error connecting to database: %v", err)
		return nil, err
	}

	log.Println("Database connection pool established successfully")
	return db, nil
}
