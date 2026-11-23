package instructions

import "strings"

// Imports reports whether a Claude Code memory file imports AGENTS.md of the
// checkout: target is the path that reaches it from that file (ImportPath),
// and the file has `@<target>` (or `@./<target>`) as a word of its own,
// outside fenced code blocks and inline code spans, which Claude Code does not
// follow. A word is a run of non-blank characters: `@AGENTS.md.` does not
// count, so a doubtful file is treated as not importing and gets a warning.
func Imports(data []byte, target string) bool {
	var f fences
	for _, l := range splitLines(data) {
		if f.step(l) {
			continue
		}
		for _, word := range strings.Fields(stripCodeSpans(l.text)) {
			if word == "@"+target || word == "@./"+target {
				return true
			}
		}
	}
	return false
}

// stripCodeSpans replaces every inline code span of one line with a space. A
// span opens with a run of backticks and closes with the next run of exactly
// the same length; a run with no partner is plain text.
func stripCodeSpans(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if text[i] != '`' {
			b.WriteByte(text[i])
			i++
			continue
		}
		j := i
		for j < len(text) && text[j] == '`' {
			j++
		}
		n := j - i
		k, closed := j, -1
		for k < len(text) {
			if text[k] != '`' {
				k++
				continue
			}
			m := k
			for m < len(text) && text[m] == '`' {
				m++
			}
			if m-k == n {
				closed = m
				break
			}
			k = m
		}
		if closed < 0 {
			b.WriteString(text[i:j])
			i = j
			continue
		}
		b.WriteByte(' ')
		i = closed
	}
	return b.String()
}
