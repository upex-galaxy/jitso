package jsconv

import "testing"

func TestTrim(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"ascii", "  a b \n", "a b"},
		{"nbsp and bom", "\u00a0\ufeffx\u3000", "x"},
		{"next line is not space", "\u0085x", "\u0085x"},
		{"only space", " \t\r\n ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Trim(tt.in); got != tt.want {
				t.Errorf("Trim(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCollapseSpace(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"runs", "a   b\t\tc\nd", "a b c d"},
		{"edges keep one space", "  a  ", " a "},
		{"nbsp", "a  b", "a b"},
		{"no space", "abc", "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CollapseSpace(tt.in); got != tt.want {
				t.Errorf("CollapseSpace(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
