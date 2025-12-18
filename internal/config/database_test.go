package config

import (
	"os"
	"testing"
)

func TestGetDatabaseConfig_FullURL(t *testing.T) {
	// Set full connection string
	os.Setenv("POSTGRES_URL", "postgresql://user:pass@localhost:5432/testdb")
	defer os.Unsetenv("POSTGRES_URL")

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
	// Clear POSTGRES_URL
	os.Unsetenv("POSTGRES_URL")

	// Set individual components
	os.Setenv("POSTGRES_HOST", "postgres")
	os.Setenv("POSTGRES_PORT", "5433")
	os.Setenv("POSTGRES_USERNAME", "tempest")
	os.Setenv("POSTGRES_PASSWORD", "secret")
	os.Setenv("POSTGRES_NAME", "weather")
	os.Setenv("POSTGRES_SSLMODE", "require")
	defer func() {
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USERNAME")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_NAME")
		os.Unsetenv("POSTGRES_SSLMODE")
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
	os.Unsetenv("POSTGRES_URL")
	os.Setenv("POSTGRES_HOST", "postgres")
	os.Setenv("POSTGRES_USERNAME", "user")
	os.Setenv("POSTGRES_PASSWORD", "pass")
	os.Setenv("POSTGRES_NAME", "db")
	// Don't set PORT or SSLMODE - should use defaults
	defer func() {
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_USERNAME")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_NAME")
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
	os.Unsetenv("POSTGRES_URL")
	os.Unsetenv("POSTGRES_HOST")

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
				"POSTGRES_HOST":     "postgres",
				"POSTGRES_PASSWORD": "pass",
				"POSTGRES_NAME":     "db",
			},
		},
		{
			name: "missing password",
			envVars: map[string]string{
				"POSTGRES_HOST":     "postgres",
				"POSTGRES_USERNAME": "user",
				"POSTGRES_NAME":     "db",
			},
		},
		{
			name: "missing database name",
			envVars: map[string]string{
				"POSTGRES_HOST":     "postgres",
				"POSTGRES_USERNAME": "user",
				"POSTGRES_PASSWORD": "pass",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("POSTGRES_URL")

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
