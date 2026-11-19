package registry

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/usage"
)

func TestEndEventTokensKey(t *testing.T) {
	tests := []struct {
		name   string
		tokens *Tokens
		want   string
	}{
		{"left out", nil, ""},
		{"null", &Tokens{}, `"tokens":null`},
		{"totals", &Tokens{Totals: &usage.Totals{Input: 1, Messages: 2}}, `"tokens":{"input":1,"cache_write":0,"cache_read":0,"output":0,"messages":2,"subagent_files":0}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(&EndEvent{ID: "x", Tokens: tt.tokens})
			if err != nil {
				t.Fatal(err)
			}
			has := strings.Contains(string(raw), `"tokens"`)
			if has != (tt.want != "") || (tt.want != "" && !strings.Contains(string(raw), tt.want)) {
				t.Errorf("%s, want %q", raw, tt.want)
			}
		})
	}
}
