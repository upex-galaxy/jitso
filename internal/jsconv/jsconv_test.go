package jsconv

import (
	"math"
	"reflect"
	"testing"
)

type fakeObject struct{}

func (fakeObject) JSObject() {}

func TestString(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"undefined", Undefined, "undefined"},
		{"null", nil, "null"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"string", "claude-haiku-5-5", "claude-haiku-5-5"},
		{"int", 123, "123"},
		{"negative int", -7, "-7"},
		{"int64", int64(42), "42"},
		{"uint64", uint64(9), "9"},
		{"float integer", 1e6, "1000000"},
		{"float fraction", 0.5, "0.5"},
		{"float 0.50 literal", 0.50, "0.5"},
		{"big float", 1e21, "1e+21"},
		{"small float", 1.5e-7, "1.5e-7"},
		{"limit small", 1e-6, "0.000001"},
		{"negative zero", math.Copysign(0, -1), "0"},
		{"nan", math.NaN(), "NaN"},
		{"inf", math.Inf(1), "Infinity"},
		{"minus inf", math.Inf(-1), "-Infinity"},
		{"array", []any{"a", 1, nil, true}, "a,1,,true"},
		{"nested array", []any{"a", []any{"b", "c"}}, "a,b,c"},
		{"empty array", []any{}, ""},
		{"map", map[string]any{"a": 1}, "[object Object]"},
		{"object", fakeObject{}, "[object Object]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := String(tt.in); got != tt.want {
				t.Errorf("String(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestOf(t *testing.T) {
	if String(Of(nil, false)) != "undefined" {
		t.Error("missing must be undefined")
	}
	if String(Of(nil, true)) != "null" {
		t.Error("present nil must be null")
	}
}

func TestStrings(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{"list", []any{"a", 2, nil}, []string{"a", "2", "null"}},
		{"scalar", "a", []string{}},
		{"nil", nil, []string{}},
		{"undefined", Undefined, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Strings(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Strings = %#v, want %#v", got, tt.want)
			}
		})
	}
}
