package config

import (
	"strings"
	"testing"
)

func TestParseBoolEnv(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		unset   bool
		want    bool
		wantErr bool
	}{
		{name: "unset key", key: "PARSE_BOOL_ENV_TEST_UNSET", unset: true, want: false, wantErr: false},
		{name: "true", key: "PARSE_BOOL_ENV_TEST_TRUE", value: "true", want: true, wantErr: false},
		{name: "1", key: "PARSE_BOOL_ENV_TEST_ONE", value: "1", want: true, wantErr: false},
		{name: "yes is invalid", key: "PARSE_BOOL_ENV_TEST_YES", value: "yes", want: false, wantErr: true},
		{name: "nonsense is invalid", key: "PARSE_BOOL_ENV_TEST_NONSENSE", value: "nonsense", want: false, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.unset {
				t.Setenv(tt.key, tt.value)
			}

			got, err := ParseBoolEnv(tt.key)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseBoolEnv(%q) = %v, nil; want error", tt.key, got)
				}
				if !strings.Contains(err.Error(), tt.key) {
					t.Errorf("ParseBoolEnv(%q) error %q does not name the key", tt.key, err.Error())
				}
				if !strings.Contains(err.Error(), tt.value) {
					t.Errorf("ParseBoolEnv(%q) error %q does not name the offending value %q", tt.key, err.Error(), tt.value)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseBoolEnv(%q) unexpected error: %v", tt.key, err)
			}
			if got != tt.want {
				t.Errorf("ParseBoolEnv(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
