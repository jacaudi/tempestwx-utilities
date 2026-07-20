package tempestudp

import (
	"fmt"
	"math"
	"testing"
)

// TestDewPointC checks the Magnus-Tetens dew point formula against a
// published reference: NOAA's online dew point calculator
// (https://www.wpc.ncep.noaa.gov/html/dewpoint.shtml) and
// bmcnoldy.earth.miami.edu's Humidity calculator (which implements this same
// a=17.625, b=243.04 formulation) both report ~48.7°F (~9.28°C) dew point
// for 68°F (20°C) air temperature at 50% relative humidity.
func TestDewPointC(t *testing.T) {
	tests := []struct {
		temperatureC    float64
		humidityPercent float64
		want            float64
	}{
		{20, 50, 9.28},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			got := DewPointC(tt.temperatureC, tt.humidityPercent)
			if math.Abs(got-tt.want) > 0.1 {
				t.Errorf("DewPointC(%v, %v) = %0.2f, want %v", tt.temperatureC, tt.humidityPercent, got, tt.want)
			}
		})
	}
}

// TestHeatIndexC checks the NWS Rothfusz regression against a value derived
// directly from the coefficients published at
// https://www.wpc.ncep.noaa.gov/html/heatindex_equation.shtml: at
// T=90°F (32.2°C), RH=70%, the regression (no low/high-RH adjustment
// applies at RH=70) evaluates to HI=105.9°F = 41.06°C.
func TestHeatIndexC(t *testing.T) {
	tests := []struct {
		temperatureC    float64
		humidityPercent float64
		want            float64
	}{
		{32.2222, 70, 41.06},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			got := HeatIndexC(tt.temperatureC, tt.humidityPercent)
			if math.Abs(got-tt.want) > 0.2 {
				t.Errorf("HeatIndexC(%v, %v) = %0.2f, want %v", tt.temperatureC, tt.humidityPercent, got, tt.want)
			}
		})
	}
}

// TestHeatIndexC_BelowThresholdReturnsAirTemp verifies the NWS convention
// that heat index below ~80°F (26.7°C) is reported as the air temperature
// itself (see derived.go's doc comment on the simplification this takes
// versus the full NWS averaged-simple-formula check).
func TestHeatIndexC_BelowThresholdReturnsAirTemp(t *testing.T) {
	got := HeatIndexC(20, 50)
	if math.Abs(got-20) > 0.001 {
		t.Errorf("HeatIndexC(20, 50) = %v, want 20 (air temp, below 80°F threshold)", got)
	}
}
