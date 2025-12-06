package config

import (
	"os"
	"testing"
)

func TestGetDatabaseConfig_FullURL(t *testing.T) {
	// Set full connection string
	os.Setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/testdb")
	defer os.Unsetenv("DATABASE_URL")

	url, err := GetDatabaseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "postgresql://user:pass@localhost:5432/testdb"
	if url != expected {
		t.Errorf("got %q, want %q", url, expected)
	}
}

func TestGetDatabaseConfig_Components(t *testing.T) {
	// Clear DATABASE_URL
	os.Unsetenv("DATABASE_URL")

	// Set individual components
	os.Setenv("DATABASE_HOST", "postgres")
	os.Setenv("DATABASE_PORT", "5433")
	os.Setenv("DATABASE_USERNAME", "tempest")
	os.Setenv("DATABASE_PASSWORD", "secret")
	os.Setenv("DATABASE_NAME", "weather")
	os.Setenv("DATABASE_SSLMODE", "require")
	defer func() {
		os.Unsetenv("DATABASE_HOST")
		os.Unsetenv("DATABASE_PORT")
		os.Unsetenv("DATABASE_USERNAME")
		os.Unsetenv("DATABASE_PASSWORD")
		os.Unsetenv("DATABASE_NAME")
		os.Unsetenv("DATABASE_SSLMODE")
	}()

	url, err := GetDatabaseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "postgresql://tempest:secret@postgres:5433/weather?sslmode=require"
	if url != expected {
		t.Errorf("got %q, want %q", url, expected)
	}
}

func TestGetDatabaseConfig_ComponentDefaults(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	os.Setenv("DATABASE_HOST", "postgres")
	os.Setenv("DATABASE_USERNAME", "user")
	os.Setenv("DATABASE_PASSWORD", "pass")
	os.Setenv("DATABASE_NAME", "db")
	// Don't set PORT or SSLMODE - should use defaults
	defer func() {
		os.Unsetenv("DATABASE_HOST")
		os.Unsetenv("DATABASE_USERNAME")
		os.Unsetenv("DATABASE_PASSWORD")
		os.Unsetenv("DATABASE_NAME")
	}()

	url, err := GetDatabaseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "postgresql://user:pass@postgres:5432/db?sslmode=disable"
	if url != expected {
		t.Errorf("got %q, want %q", url, expected)
	}
}

func TestGetDatabaseConfig_NoConfig(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("DATABASE_HOST")

	url, err := GetDatabaseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if url != "" {
		t.Errorf("expected empty string when no config, got %q", url)
	}
}

func TestGetDatabaseConfig_MissingRequired(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
	}{
		{
			name: "missing username",
			envVars: map[string]string{
				"DATABASE_HOST":     "postgres",
				"DATABASE_PASSWORD": "pass",
				"DATABASE_NAME":     "db",
			},
		},
		{
			name: "missing password",
			envVars: map[string]string{
				"DATABASE_HOST":     "postgres",
				"DATABASE_USERNAME": "user",
				"DATABASE_NAME":     "db",
			},
		},
		{
			name: "missing database name",
			envVars: map[string]string{
				"DATABASE_HOST":     "postgres",
				"DATABASE_USERNAME": "user",
				"DATABASE_PASSWORD": "pass",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("DATABASE_URL")

			// Set provided vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			_, err := GetDatabaseConfig()
			if err == nil {
				t.Error("expected error for missing required field, got nil")
			}
		})
	}
}
