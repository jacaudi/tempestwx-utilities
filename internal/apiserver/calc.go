package apiserver

import "math"

// This file holds the derived meteorological quantities that the tempest-display
// SPA expects but that are not carried directly in the obs_st report: dew point,
// heat index, wind chill, and a combined "feels like". All inputs and outputs
// are in the units the obs_st report uses (°C, %, m/s).

// cToF converts Celsius to Fahrenheit.
func cToF(c float64) float64 { return c*9.0/5.0 + 32.0 }

// fToC converts Fahrenheit to Celsius.
func fToC(f float64) float64 { return (f - 32.0) * 5.0 / 9.0 }

// mpsToMph converts metres per second to miles per hour.
func mpsToMph(mps float64) float64 { return mps * 2.2369362920544 }

// DewPointC returns the dew point in °C for the given air temperature (°C) and
// relative humidity (%), using the Magnus-Tetens approximation. Relative
// humidity is clamped to [1, 100] to keep the logarithm finite.
func DewPointC(tempC, rh float64) float64 {
	if rh < 1 {
		rh = 1
	}
	if rh > 100 {
		rh = 100
	}
	const a, b = 17.62, 243.12
	gamma := math.Log(rh/100.0) + a*tempC/(b+tempC)
	return b * gamma / (a - gamma)
}

// HeatIndexC returns the NWS heat index in °C for the given air temperature (°C)
// and relative humidity (%). The heat index is only meaningful in warm
// conditions; below 80 °F (~26.7 °C) the air temperature is returned unchanged.
func HeatIndexC(tempC, rh float64) float64 {
	tf := cToF(tempC)
	if tf < 80 {
		return tempC
	}
	r := rh
	hi := -42.379 + 2.04901523*tf + 10.14333127*r - 0.22475541*tf*r -
		6.83783e-3*tf*tf - 5.481717e-2*r*r + 1.22874e-3*tf*tf*r +
		8.5282e-4*tf*r*r - 1.99e-6*tf*tf*r*r

	// Rothfusz adjustments at the humidity extremes.
	switch {
	case r < 13 && tf >= 80 && tf <= 112:
		hi -= ((13 - r) / 4) * math.Sqrt((17-math.Abs(tf-95))/17)
	case r > 85 && tf >= 80 && tf <= 87:
		hi += ((r - 85) / 10) * ((87 - tf) / 5)
	}
	return fToC(hi)
}

// WindChillC returns the NWS wind-chill temperature in °C for the given air
// temperature (°C) and wind speed (m/s). Wind chill is only defined for cold,
// breezy conditions (≤ 50 °F and wind > 3 mph); outside that range the air
// temperature is returned unchanged.
func WindChillC(tempC, windMps float64) float64 {
	tf := cToF(tempC)
	mph := mpsToMph(windMps)
	if tf > 50 || mph <= 3 {
		return tempC
	}
	v := math.Pow(mph, 0.16)
	wc := 35.74 + 0.6215*tf - 35.75*v + 0.4275*tf*v
	return fToC(wc)
}

// FeelsLikeC returns the apparent temperature in °C: the heat index in hot
// conditions, the wind chill in cold/breezy conditions, and otherwise the plain
// air temperature.
func FeelsLikeC(tempC, rh, windMps float64) float64 {
	tf := cToF(tempC)
	switch {
	case tf >= 80:
		return HeatIndexC(tempC, rh)
	case tf <= 50 && mpsToMph(windMps) > 3:
		return WindChillC(tempC, windMps)
	default:
		return tempC
	}
}

// round1 rounds to one decimal place for tidy JSON output of derived values.
func round1(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*10) / 10
}
