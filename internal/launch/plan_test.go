package launch

import "testing"

func TestBriefFile(t *testing.T) {
	tests := []struct {
		name   string
		skills []string
		brief  string
		want   string
	}{
		{"brief with a final newline", nil, "B\n", "CARD\n\nB\n\nRULES\n"},
		{"brief without a final newline", nil, "B", "CARD\n\nB\n\nRULES\n"},
		{"empty brief", nil, "", "CARD\n\n\n\nRULES\n"},
		{"skills first", []string{"a", "b"}, "B\n", "CARD\n\nLoad first: a, b\n\nB\n\nRULES\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BriefFile("CARD", tt.skills, tt.brief, "RULES"); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
