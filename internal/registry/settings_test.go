package registry

import (
	"encoding/json"
	"testing"
)

func TestSettingsMarshal(t *testing.T) {
	tests := []struct {
		name string
		in   Settings
		want string
	}{
		{"copy", Settings{Copied: []string{"env"}, Cut: []string{"model"}}, `{"copied":["env"],"cut":["model"]}`},
		{"copy with nothing", Settings{}, `{"copied":[],"cut":[]}`},
		{"user layer", Settings{Mode: ModeUserLayer, Overrides: []string{"a", "b"}, Cloud: []string{"Supabase"}}, `{"mode":"user-layer","overrides":["a","b"],"cloud":["Supabase"]}`},
		{"user layer with nothing", Settings{Mode: ModeUserLayer}, `{"mode":"user-layer","overrides":[],"cloud":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(&tt.in)
			if err != nil || string(got) != tt.want {
				t.Errorf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}
