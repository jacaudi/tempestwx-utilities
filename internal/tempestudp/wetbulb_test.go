package tempestudp

import (
	"fmt"
	"math"
	"testing"
)

func TestWetBulbTemperatureC(t *testing.T) {
	type args struct {
		temperatureC       float64
		humidityPercent    float64
		stationPressureHpa float64
	}
	tests := []struct {
		args args
		want float64
	}{
		{args{25, 50, 900}, 17.71},
		{args{25, 90, 900}, 23.7},
		{args{30, 33, 1050}, 18.92},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			got := WetBulbTemperatureC(tt.args.temperatureC, tt.args.humidityPercent, tt.args.stationPressureHpa)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("WetBulbTemperatureC(%v, %v, %v) = %0.2f, want %v", tt.args.temperatureC, tt.args.humidityPercent, tt.args.stationPressureHpa, got, tt.want)
			}
		})
	}
}

// TestWetBulb_NonConvergentReturnsNaN verifies that inputs which never
// satisfy the convergence tolerance return NaN instead of panicking.
//
// humidityPercent = -500 is physically impossible (relative humidity cannot
// be negative), but a malformed/corrupt UDP broadcast could still deliver an
// out-of-range value here. It drives eHumidity strongly negative while
// eWetBulb stays positive, so the search's overshoot-correction step shrinks
// toward zero without ever bringing |delta| under the 0.001 tolerance within
// 10000 iterations.
func TestWetBulb_NonConvergentReturnsNaN(t *testing.T) {
	got := WetBulbTemperatureC(25, -500, 900)
	if !math.IsNaN(got) {
		t.Errorf("WetBulbTemperatureC(25, -500, 900) = %v, want NaN", got)
	}
}
