package catalog

import "testing"

func TestStatusSpan(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // the text of the span, "" with ok false when there is none
		ok   bool
	}{
		{"plain", "id: x\nstatus: draft\n", "draft", true},
		{"status first", "status: draft\nid: x\n", "draft", true},
		{"status last without a final newline", "id: x\nstatus: draft", "draft", true},
		{"status not first, after nested values", "id: x\nnote:\n  - { check: a }\nstatus: draft\nrole: r\n", "draft", true},
		{"a trailing comment", "id: x\nstatus: draft   # waiting\n", "draft", true},
		{"extra spaces", "id: x\nstatus:    draft\n", "draft", true},
		{"double quoted", "id: x\nstatus: \"draft\"\n", "draft", true},
		{"single quoted", "id: x\nstatus: 'draft'\n", "draft", true},
		{"quoted key", "id: x\n\"status\": draft\n", "draft", true},
		{"another value", "id: x\nstatus: approved\n", "approved", true},
		{"comments and blank lines before", "# one\n\n# two\nid: x\n\nstatus: draft\n", "draft", true},
		{"an explicit document start", "---\nid: x\nstatus: draft\n", "draft", true},
		{"CRLF line ends", "id: x\r\nstatus: draft\r\n", "draft", true},
		{"a multi-byte character before it", "id: x\ndescription: café — ✓\nstatus: draft\n", "draft", true},
		{"a byte order mark", "\xef\xbb\xbfid: x\nstatus: draft\n", "draft", true},
		{"the word elsewhere is not the status", "id: x\ndescription: the status draft\nstatus: draft\n", "draft", true},
		{"flow style mapping", "{id: x, status: draft}\n", "draft", true},
		{"nested status is not the top level one", "id: x\nnote:\n  status: draft\n", "", false},
		{"no status", "id: x\n", "", false},
		{"an empty file", "", "", false},
		{"not a mapping", "- a\n- b\n", "", false},
		{"not a document", "id: [\n", "", false},
		{"an empty status", "id: x\nstatus:\n", "", false},
		{"a tag", "id: x\nstatus: !!str draft\n", "", false},
		{"an anchor", "id: x\nstatus: &a draft\n", "", false},
		{"an alias", "id: x\nother: &a draft\nstatus: *a\n", "", false},
		{"a literal block", "id: x\nstatus: |\n  draft\n", "", false},
		{"a folded block", "id: x\nstatus: >\n  draft\n", "", false},
		{"a multi-line plain scalar", "id: x\nstatus: dra\n  ft\n", "", false},
		{"an escape in double quotes", "id: x\nstatus: \"dr\\x61ft\"\n", "", false},
		{"an escape in single quotes", "id: x\nstatus: 'it''s'\n", "", false},
		{"a number", "id: x\nstatus: 12\n", "12", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := StatusSpan([]byte(tt.src))
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (span %d:%d)", ok, tt.ok, start, end)
			}
			if !ok {
				return
			}
			if got := tt.src[start:end]; got != tt.want {
				t.Errorf("span = %q, want %q", got, tt.want)
			}
		})
	}
}

// The status of the second status-like word never moves the span: only the
// top-level key is found, wherever the word appears in the text.
func TestStatusSpanIsTheKeyNotTheWord(t *testing.T) {
	src := "description: draft status draft\nstatus: draft\n"
	start, end, ok := StatusSpan([]byte(src))
	if !ok || start != len("description: draft status draft\nstatus: ") || src[start:end] != "draft" {
		t.Fatalf("span %d:%d ok %v", start, end, ok)
	}
}

func TestOffsetAt(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		line, col int
		want      int
		wantOK    bool
	}{
		{"first character", "abc\n", 1, 1, 0, true},
		{"second line", "ab\ncd\n", 2, 2, 4, true},
		{"CRLF counts as one break", "ab\r\ncd\n", 2, 1, 4, true},
		{"a lone CR", "ab\rcd\n", 2, 1, 3, true},
		{"NEL", "ab\u0085cd\n", 2, 1, 4, true},
		{"line separator", "ab cd\n", 2, 1, 5, true},
		{"columns count characters", "ééx\n", 1, 3, 4, true},
		{"after a byte order mark", "\xef\xbb\xbfab\n", 1, 2, 4, true},
		{"the end of the text", "ab", 1, 3, 2, true},
		{"a line that is not there", "ab\n", 3, 1, 0, false},
		{"a column past the line", "ab\ncd\n", 1, 5, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := offsetAt([]byte(tt.src), tt.line, tt.col)
			if ok != tt.wantOK || (ok && got != tt.want) {
				t.Errorf("offsetAt = %d, %v; want %d, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
