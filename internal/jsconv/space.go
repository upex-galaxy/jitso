package jsconv

import "strings"

// IsSpace reports whether r is in JavaScript's \s class (WhiteSpace and
// LineTerminator). It differs from unicode.IsSpace: it has U+FEFF and lacks
// U+0085.
func IsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// Trim is String.prototype.trim: it removes leading and trailing IsSpace runes.
func Trim(s string) string {
	return strings.TrimFunc(s, IsSpace)
}

// CollapseSpace is s.replaceAll(/\s+/g, " "): every run of IsSpace runes
// becomes one space.
func CollapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inRun := false
	for _, r := range s {
		if IsSpace(r) {
			if !inRun {
				b.WriteByte(' ')
			}
			inRun = true
			continue
		}
		inRun = false
		b.WriteRune(r)
	}
	return b.String()
}
