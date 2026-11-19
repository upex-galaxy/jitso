// Package brief holds what the code enforces about a brief: the quality
// checks (pure, apart from one existence probe per cited path) and the guard
// on the brief file's path. A brief is text a model wrote; it travels as a
// file and never as an argument.
//
// The patterns spell out JavaScript's \s and case-insensitive letters instead
// of using Go's \s and (?i): Go's \s lacks \v and the Unicode spaces, and (?i)
// folds U+017F to s and U+212A to k, which JavaScript patterns without the u
// flag do not.
package brief

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Word limits of a brief.
const (
	MinWords = 40
	MaxWords = 800
)

// RequiredSections are the headings every brief must have, in this order.
var RequiredSections = []string{"Goal", "Scope", "Done when", "Report format"}

// The check names, in the order failures are reported.
const (
	CheckSections = "sections"
	CheckLength   = "length"
	CheckLanguage = "language"
	CheckPaths    = "paths"
)

// The --brief-language values. Only LanguageEnglish runs the language check.
const (
	LanguageEnglish = "en"
	LanguageSpanish = "es"
	LanguageAny     = "any"
)

// Failure is one failed check.
type Failure struct {
	Check   string
	Message string
}

// jsSpace is JavaScript's \s class.
const jsSpace = `[\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}]`

var (
	letters      = regexp.MustCompile(`\p{L}+`)
	wordSpace    = regexp.MustCompile(jsSpace + `+`)
	hasLetter    = regexp.MustCompile(`\p{L}`)
	headingLine  = regexp.MustCompile(`^#{1,6}(?:[^#]|$)`)
	backticked   = regexp.MustCompile("`([^`" + jsSpace[1:len(jsSpace)-1] + "]+)`")
	urlScheme    = regexp.MustCompile(`^[A-Za-z]+://`)
	homeOrVar    = regexp.MustCompile(`^[~$]`)
	globOrHolder = regexp.MustCompile(`[*<>{}]`)
	fileName     = regexp.MustCompile(`\.[A-Za-z][A-Za-z0-9]{0,9}$`)
	outputLine   = regexp.MustCompile(`^` + jsSpace + `*(?:[-*]` + jsSpace + `*)?(?:` + anyCase("output") + `[Ss]?|` + anyCase("create") + `[Ss]?)` + jsSpace + `*:`)
	sections     = sectionPatterns()
)

var english = set("the", "a", "an", "of", "to", "and", "in", "is", "for", "on", "with", "that", "it", "as", "be", "are", "this", "by", "at", "from", "or", "may", "not", "you", "every", "then")

// Spanish stopwords: the language check only detects Spanish briefs, so this
// data must stay Spanish.
var spanish = set("el", "la", "los", "las", "de", "del", "que", "en", "y", "un", "una", "por", "para", "con", "es", "se", "lo", "al", "como", "pero", "su", "está", "puedes", "cada", "ni")

// Check runs every brief check and returns the failures in the fixed order
// sections, length, language, paths; none means the brief is acceptable.
// exists reports whether a cited path exists (see Exists).
func Check(text, language string, exists func(path string) bool) []Failure {
	var failures []Failure

	var missing []string
	for i, name := range RequiredSections {
		if !sections[i].MatchString(text) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		failures = append(failures, Failure{CheckSections, "missing section: " + strings.Join(missing, ", ")})
	}

	// The length check counts words (defect N-27); the
	// language check keeps its letter runs of the whole lowered text.
	if n := countWords(text); n < MinWords || n > MaxWords {
		failures = append(failures, Failure{CheckLength, strconv.Itoa(n) + " words, expected " + strconv.Itoa(MinWords) + " to " + strconv.Itoa(MaxWords)})
	}
	if notEnglish(text, language) {
		failures = append(failures, Failure{CheckLanguage, "the brief must be written in English"})
	}

	outputs := declaredOutputs(text)
	var ghosts []string
	for _, path := range citedPaths(text) {
		if slices.Contains(ghosts, path) || outputs[path] || exists(path) {
			continue
		}
		ghosts = append(ghosts, path)
	}
	if len(ghosts) > 0 {
		failures = append(failures, Failure{CheckPaths, "cited path does not exist: " + strings.Join(ghosts, ", ")})
	}
	return failures
}

// notEnglish is the language check: true when the language is
// LanguageEnglish and the text has more Spanish stopwords than English ones,
// or fewer than a tenth of its letter runs are English stopwords.
func notEnglish(text, language string) bool {
	words := letters.FindAllString(strings.ToLower(text), -1)
	if language != LanguageEnglish || len(words) == 0 {
		return false
	}
	en, es := 0, 0
	for _, w := range words {
		if english[w] {
			en++
		}
		if spanish[w] {
			es++
		}
	}
	return es > en || float64(en)/float64(len(words)) < 0.1
}

// countWords is the length of a brief in words. A word is a maximal run of
// characters that are not JavaScript whitespace and that holds at least one
// letter, so `src/sync.ts` and `don't` are one word each and `3.14`, `(7)` and
// a mid-line `#9` are none. A heading line (one to six # at the start of the
// line, as the section patterns read it) adds nothing.
func countWords(text string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if headingLine.MatchString(line) {
			continue
		}
		for _, token := range wordSpace.Split(line, -1) {
			if hasLetter.MatchString(token) {
				n++
			}
		}
	}
	return n
}

// pathsHint follows the whole reason when a cited path is missing, so a
// brief author learns how to declare a file the worker will create.
const pathsHint = "A file the worker will create goes on a line that starts with Output: or Creates:"

// Reason is the rejection text for failures:
// `brief failed: <check> (<message>); ...`, followed by pathsHint once when
// one of the failures is the paths check.
func Reason(failures []Failure) string {
	parts := make([]string, len(failures))
	hint := false
	for i, f := range failures {
		parts[i] = f.Check + " (" + f.Message + ")"
		if f.Check == CheckPaths {
			hint = true
		}
	}
	reason := "brief failed: " + strings.Join(parts, "; ")
	if hint {
		reason += ". " + pathsHint
	}
	return reason
}

// citedPaths are the backtick-quoted tokens that look like a file path: a
// slash, no URL scheme, no leading ~, $ or @, no glob or placeholder
// character, and a last segment that ends in a dot and a letter-led extension
// (so a trailing slash, a branch name or a skill name is text, not a file).
func citedPaths(text string) []string {
	var found []string
	for _, m := range backticked.FindAllStringSubmatch(text, -1) {
		v := m[1]
		if !strings.Contains(v, "/") || urlScheme.MatchString(v) || homeOrVar.MatchString(v) || globOrHolder.MatchString(v) {
			continue
		}
		if strings.HasPrefix(v, "@") || !fileName.MatchString(v[strings.LastIndex(v, "/")+1:]) {
			continue
		}
		found = append(found, v)
	}
	return found
}

// declaredOutputs are the paths cited on a line that starts with Output:,
// Outputs:, Create: or Creates: (any case, optionally a list item). Such a
// path may not exist yet, anywhere in the brief.
func declaredOutputs(text string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		if !outputLine.MatchString(line) {
			continue
		}
		for _, path := range citedPaths(line) {
			out[path] = true
		}
	}
	return out
}

// sectionPatterns are, per required section, the pattern
// /^#{1,6}\s*<name>\s*:?\s*$/im. \s may cross a line break.
func sectionPatterns() []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(RequiredSections))
	for i, name := range RequiredSections {
		out[i] = regexp.MustCompile(`(?m)^#{1,6}` + jsSpace + `*` + anyCase(name) + jsSpace + `*:?` + jsSpace + `*$`)
	}
	return out
}

// anyCase spells a literal as a case-insensitive pattern of ASCII letters.
func anyCase(literal string) string {
	var b strings.Builder
	for _, r := range literal {
		lower, upper := strings.ToLower(string(r)), strings.ToUpper(string(r))
		if lower == upper {
			b.WriteString(regexp.QuoteMeta(string(r)))
			continue
		}
		b.WriteString("[" + lower + upper + "]")
	}
	return b.String()
}

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}
