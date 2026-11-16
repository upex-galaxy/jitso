package analyze

import (
	"strings"
	"unicode/utf8"
)

// isJSSpace is the JavaScript \s class, which the oracle's patterns use. It
// differs from unicode.IsSpace (U+FEFF is in, U+0085 is out).
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
}

// startsName is true when a name may begin at byte i: the start of the text
// or right after a space, slash, quote, equals sign or colon.
func startsName(text string, i int) bool {
	if i == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(text[:i])
	return isJSSpace(r) || strings.ContainsRune(`/'"=:`, r)
}

func runeAt(text string, i int) (rune, bool) {
	if i >= len(text) {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(text[i:])
	return r, true
}

// IsSecretPath is true when the text names a secret file: settings.local.json
// anywhere, an .auth folder, or an .env file other than .env.example and the
// *.schema files. It is the oracle's isSecretPath with its lookaheads written
// out by hand (Go's RE2 has none): for .env the name runs over [\w.-] after
// the first dot, and the text after it must be the end or a space, quote, ;,
// &, | or ). The run and the delimiter classes are disjoint, so the oracle's
// backtracking can never find a shorter match.
func IsSecretPath(text string) bool {
	if strings.Contains(text, "settings.local.json") {
		return true
	}
	for i := 0; i+5 <= len(text); i++ {
		if text[i] == '.' && strings.HasPrefix(text[i:], ".auth") && startsName(text, i) {
			if r, ok := runeAt(text, i+5); !ok || r == '/' || r == '\'' || r == '"' || isJSSpace(r) {
				return true
			}
		}
	}
	for i := 0; i+4 <= len(text); i++ {
		if text[i] != '.' || !strings.HasPrefix(text[i:], ".env") || !startsName(text, i) {
			continue
		}
		end := i + 4
		if end < len(text) && text[end] == '.' {
			end++
			for end < len(text) && (isWordByte(text[end]) || text[end] == '.' || text[end] == '-') {
				end++
			}
		}
		if r, ok := runeAt(text, end); ok && !(isJSSpace(r) || strings.ContainsRune(`'";&|)`, r)) {
			continue
		}
		if name := text[i:end]; name != ".env.example" && !strings.HasSuffix(name, ".schema") {
			return true
		}
	}
	return false
}

// pathTools maps a tool to the inputs that name the file it touches. A write
// tool counts like a read tool (TS-ODDITIES R7, defect N-28): the counter is
// still called secret_path_reads, because "read" there means "touched". The
// content of a write (content, old_string, new_string, edits, new_source) is
// never looked at.
var pathTools = map[string][]string{
	"Read":         {"file_path", "path", "pattern", "glob", "notebook_path"},
	"Grep":         {"file_path", "path", "pattern", "glob", "notebook_path"},
	"Glob":         {"file_path", "path", "pattern", "glob", "notebook_path"},
	"NotebookRead": {"file_path", "path", "pattern", "glob", "notebook_path"},
	"Write":        {"file_path"},
	"Edit":         {"file_path"},
	"MultiEdit":    {"file_path"},
	"NotebookEdit": {"notebook_path"},
}

// touchesSecret reports whether a tool call touches a secret path. It looks at
// string inputs only and returns a boolean, never the text.
func touchesSecret(name string, input any) bool {
	args, ok := input.(map[string]any)
	if !ok {
		return false
	}
	if name == "Bash" {
		cmd, ok := args["command"].(string)
		return ok && IsSecretPath(cmd)
	}
	for _, key := range pathTools[name] {
		if s, ok := args[key].(string); ok && IsSecretPath(s) {
			return true
		}
	}
	return false
}
