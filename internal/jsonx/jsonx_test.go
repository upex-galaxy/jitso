package jsonx

import (
	"testing"
	"time"
)

func TestMarshal(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"no html escape", map[string]string{"r": "<id> & <file>"}, `{"r":"<id> & <file>"}`},
		{"float shortest", []float64{0.5, 6, 1e21, 0.1}, `[0.5,6,1e+21,0.1]`},
		{"ordered object", Object{{"z", 1}, {"a", []string{}}}, `{"z":1,"a":[]}`},
		{"empty object", Object{}, `{}`},
		{"nested object", Object{{"m", Object{{"b", nil}}}}, `{"m":{"b":null}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Marshal(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestMarshalIndent(t *testing.T) {
	got, err := MarshalIndent(Object{{"mcpServers", Object{{"dbhub", Object{{"command", "x"}, {"args", []string{}}}}}}}, "  ")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"mcpServers\": {\n    \"dbhub\": {\n      \"command\": \"x\",\n      \"args\": []\n    }\n  }\n}"
	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTime(t *testing.T) {
	loc := time.FixedZone("ART", -3*3600)
	in := time.Date(2026, 11, 13, 11, 22, 33, 5_000_000, loc)
	if got := Time(in); got != "2026-11-13T14:22:33.005Z" {
		t.Errorf("got %s", got)
	}
}
