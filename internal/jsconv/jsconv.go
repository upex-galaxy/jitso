// Package jsconv reproduces the JavaScript String() coercion applied
// to catalog and JSON values, so a value prints the same text as before (for example `model "123" is not in the allowlist`, or a missing field
// printed as "undefined").
//
// Values are the generic shapes a decoder produces: nil (null), bool, the Go
// integer kinds, float64, string, []any, map[string]any, and any type that
// implements Object (an ordered mapping).
package jsconv

import (
	"math"
	"strconv"
	"strings"
)

// Object marks a mapping type that is not a map[string]any, such as the
// catalog's ordered mapping. String renders it as "[object Object]".
type Object interface {
	JSObject()
}

type undefined struct{}

// Undefined stands for a missing value (JavaScript undefined). String renders
// it as "undefined"; Join renders it as "".
var Undefined any = undefined{}

// Of returns v when present is true and Undefined otherwise, for a map lookup
// such as `v, ok := m[key]`.
func Of(v any, present bool) any {
	if !present {
		return Undefined
	}
	return v
}

// String is JavaScript String(v).
func String(v any) string {
	switch x := v.(type) {
	case undefined:
		return "undefined"
	case nil:
		return "null"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case string:
		return x
	case int:
		// Through float64, as JavaScript numbers are: precision ends at 2^53.
		return Number(float64(x))
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return Number(float64(x))
	case uint:
		return Number(float64(x))
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return Number(float64(x))
	case float32:
		return Number(float64(x))
	case float64:
		return Number(x)
	case []any:
		return Join(x, ",")
	case []string:
		return strings.Join(x, ",")
	case map[string]any, Object:
		return "[object Object]"
	default:
		return "[object Object]"
	}
}

// Join is JavaScript Array.prototype.join: null and undefined elements become
// the empty string, every other element goes through String.
func Join(list []any, sep string) string {
	parts := make([]string, len(list))
	for i, item := range list {
		switch item.(type) {
		case nil, undefined:
			parts[i] = ""
		default:
			parts[i] = String(item)
		}
	}
	return strings.Join(parts, sep)
}

// Number is JavaScript Number.prototype.toString() in base 10: integers
// without a fraction, the shortest round-trip digits, and the exponent form
// only below 1e-6 or from 1e21 on (written "1e+21", "1.5e-7").
func Number(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	abs := math.Abs(f)
	if abs >= 1e21 || abs < 1e-6 {
		s := strconv.FormatFloat(f, 'e', -1, 64)
		mant, exp, _ := strings.Cut(s, "e")
		sign := exp[:1]
		digits := strings.TrimLeft(exp[1:], "0")
		if digits == "" {
			digits = "0"
		}
		return mant + "e" + sign + digits
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// Strings is the list form of String: a list maps every element through
// String, anything else is an empty list.
func Strings(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, len(list))
	for i, item := range list {
		out[i] = String(item)
	}
	return out
}
