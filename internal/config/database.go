package config

import (
	"fmt"
	"os"
)

// GetDatabaseConfig returns the PostgreSQL connection string.
// It supports two configuration methods with precedence:
// 1. DATABASE_URL (full connection string) - takes precedence
// 2. Individual components (DATABASE_HOST, DATABASE_PORT, etc.)
//
// Returns empty string if no database is configured (DATABASE_URL and DATABASE_HOST both unset).
// Returns error if DATABASE_HOST is set but required fields are missing.
func GetDatabaseConfig() (string, error) {
	// Option 1: Full connection string (takes precedence)
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		return dbURL, nil
	}

	// Option 2: Build from components
	host := os.Getenv("DATABASE_HOST")
	if host == "" {
		return "", nil // No database configured
	}

	port := os.Getenv("DATABASE_PORT")
	if port == "" {
		port = "5432"
	}

	username := os.Getenv("DATABASE_USERNAME")
	if username == "" {
		return "", fmt.Errorf("DATABASE_USERNAME required when using DATABASE_HOST")
	}

	password := os.Getenv("DATABASE_PASSWORD")
	if password == "" {
		return "", fmt.Errorf("DATABASE_PASSWORD required when using DATABASE_HOST")
	}

	dbname := os.Getenv("DATABASE_NAME")
	if dbname == "" {
		return "", fmt.Errorf("DATABASE_NAME required when using DATABASE_HOST")
	}

	sslmode := os.Getenv("DATABASE_SSLMODE")
	if sslmode == "" {
		sslmode = "disable"
	}

	return fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s",
		username, password, host, port, dbname, sslmode), nil
}
