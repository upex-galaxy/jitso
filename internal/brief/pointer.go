package brief

import (
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// A pointer brief is a brief file whose only non-blank line is
// `Brief: <path>`. The launcher then reads the named card instead, so a job
// already written in another format does not have to be rewritten.

var (
	edgeSpace = regexp.MustCompile(`^` + jsSpace + `+|` + jsSpace + `+$`)
)

var pointerLine = regexp.MustCompile(`^` + jsSpace + `*` + anyCase("brief") + jsSpace + `*:(.*)$`)

// The pointer target rejections (exit 2). Each is followed by `: <path as written>`
// except reasonTargetEmpty.
const (
	reasonTargetEmpty      = "brief target path is empty or contains a NUL byte"
	reasonTargetMissing    = "brief target does not exist"
	reasonTargetNotRegular = "brief target is not a regular file"
	reasonTargetOutside    = "brief target is outside the project"
	reasonTargetNotMD      = "brief target must be a .md file"
)

// Pointer reports whether text is a pointer brief and returns the target as
// written: the rest of the line after `Brief:`, trimmed, with one pair of
// backticks around it removed. The target may be empty.
func Pointer(text string) (target string, ok bool) {
	var line string
	for _, l := range strings.Split(text, "\n") {
		if wordSpace.ReplaceAllString(l, "") == "" {
			continue
		}
		if line != "" {
			return "", false
		}
		line = l
	}
	m := pointerLine.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	target = trimSpace(m[1])
	if len(target) >= 2 && strings.HasPrefix(target, "`") && strings.HasSuffix(target, "`") {
		target = target[1 : len(target)-1]
	}
	return target, true
}

func trimSpace(s string) string {
	return edgeSpace.ReplaceAllString(s, "")
}

// ResolveTarget is the guard on a pointer's target. A relative target is read
// from the project's primary checkout. Its real path (every symlink resolved)
// must be a regular file inside the primary checkout and must end in .md in any
// case, so a link to a secret file is refused. It returns the real path, or a
// Rejected error naming the target as written.
func ResolveTarget(target, root string) (string, error) {
	if target == "" || strings.ContainsRune(target, 0) {
		return "", fail.Rejected(reasonTargetEmpty)
	}
	real, err := realRegular(target, root)
	switch {
	case errors.Is(err, errMissing):
		return "", fail.Rejected(reasonTargetMissing + ": " + target)
	case errors.Is(err, errNotRegular):
		return "", fail.Rejected(reasonTargetNotRegular + ": " + target)
	}
	realRoot, rerr := filepath.EvalSymlinks(root)
	if rerr != nil || !inside(realRoot, real) {
		return "", fail.Rejected(reasonTargetOutside + ": " + target)
	}
	if !strings.HasSuffix(strings.ToLower(real), ".md") {
		return "", fail.Rejected(reasonTargetNotMD + ": " + target)
	}
	return real, nil
}

// CheckTarget runs the checks a pointer's target text owes: a minimum length
// and, for LanguageEnglish, the language test. It owes no sections, no
// maximum and no cited paths.
func CheckTarget(text, language string) []Failure {
	var failures []Failure
	if n := countWords(text); n < MinWords {
		failures = append(failures, Failure{CheckLength, strconv.Itoa(n) + " words, expected at least " + strconv.Itoa(MinWords)})
	}
	if notEnglish(text, language) {
		failures = append(failures, Failure{CheckLanguage, "the brief must be written in English"})
	}
	return failures
}

// TargetReason is the rejection text for the failures of CheckTarget:
// `brief target failed: <check> (<message>); ...`.
func TargetReason(failures []Failure) string {
	parts := make([]string, len(failures))
	for i, f := range failures {
		parts[i] = f.Check + " (" + f.Message + ")"
	}
	return "brief target failed: " + strings.Join(parts, "; ")
}
