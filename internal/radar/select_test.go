package radar

import "testing"

func TestNearestSite(t *testing.T) {
	// Near Oklahoma City; the nearest WSR-88D site is KTLX ("TLX").
	const lat, lon = 35.47, -97.51

	got := NearestSite(lat, lon)
	if got != "TLX" {
		t.Errorf("NearestSite(%v, %v) = %q, want %q", lat, lon, got, "TLX")
	}
}

func TestIsValidSite(t *testing.T) {
	tests := []struct {
		name string
		code string
		want bool
	}{
		{"valid site code", "TLX", true},
		{"path traversal attempt", "../etc", false},
		{"well-formed but unknown code", "ZZZ", false},
		{"wrong case", "tlx", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidSite(tt.code)
			if got != tt.want {
				t.Errorf("IsValidSite(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}
