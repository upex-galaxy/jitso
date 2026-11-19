package launch

import (
	"testing"

	"github.com/upex-galaxy/jitso/internal/profile"
)

func TestBriefFile(t *testing.T) {
	tests := []struct {
		name   string
		skills []string
		brief  string
		block  string
		want   string
	}{
		{"brief with a final newline", nil, "B\n", "", "CARD\n\nB\n\nRULES\n"},
		{"brief without a final newline", nil, "B", "", "CARD\n\nB\n\nRULES\n"},
		{"empty brief", nil, "", "", "CARD\n\n\n\nRULES\n"},
		{"skills first", []string{"a", "b"}, "B\n", "", "CARD\n\nLoad first: a, b\n\nB\n\nRULES\n"},
		{"fields block before the rules", nil, "B", "## Fields\n- a: 1\n", "CARD\n\nB\n\n## Fields\n- a: 1\n\nRULES\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BriefFile("CARD", tt.skills, tt.brief, tt.block, "RULES"); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFieldsBlock(t *testing.T) {
	var f profile.Fields
	if got := FieldsBlock(f); got != "" {
		t.Errorf("no fields: got %q, want none", got)
	}
	f.Set("ticket_key", "BK-1")
	f.Set("env", "# not a heading")
	want := "## Fields\n- env: # not a heading\n- ticket_key: BK-1\n"
	if got := FieldsBlock(f); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
