package config

import (
	"fmt"
	"os"
	"strconv"
)

// ParseBoolEnv reads the named environment variable and parses it as a
// boolean using strconv.ParseBool. An unset or empty variable is treated as
// false with no error. A value that strconv.ParseBool cannot parse (e.g. a
// typo like "ture") is a fatal configuration error rather than a silently
// disabled feature: the returned error names both the variable and the
// offending value.
func ParseBoolEnv(key string) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return false, nil
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid boolean value %q for %s: %w", v, key, err)
	}
	return b, nil
}
