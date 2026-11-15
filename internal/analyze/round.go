package analyze

import "math"

// jsRound is JavaScript Math.round: halves round toward positive infinity
// (so -2.5 is -2), unlike math.Round, which rounds them away from zero.
func jsRound(x float64) float64 {
	f := math.Floor(x)
	if x-f >= 0.5 {
		return f + 1
	}
	return f
}

// round3 rounds to three decimals the way the oracle does.
func round3(x float64) float64 { return jsRound(x*1000) / 1000 }
