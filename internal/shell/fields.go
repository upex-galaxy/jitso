package shell

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// The character classes are spelled out instead of using (?i): Go folds case
// with Unicode rules, so (?i)[A-Z] would also match U+017F and U+212A, which JavaScript
// patterns without the u flag reject.
var (
	launchID = regexp.MustCompile(`^L[0-9]{8}T[0-9]{6}-[0-9a-f]{4}$`)
	orcaID   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)
	// sessionName has no space, bracket or colon: a harness may rewrite them
	// in the name its peers see, and SendMessage reads a trailing " [ref]".
	sessionName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

// launchFields is the allowlist per launcher field (a launcher
// field table): plain characters only, never starting with "-".
var launchFields = map[string]*regexp.Regexp{
	"build":       regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`),
	"run":         orcaID,
	"task":        orcaID,
	"parent":      launchID,
	"name":        sessionName,
	"worktree":    regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,511}$`),
	"mcp-exclude": regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_:-]{0,63}$`),
}

// MaxRepoBytes is the longest --repo path.
const MaxRepoBytes = 1024

// checkRepo is the rule of --repo, the one launcher field that is a path of
// this machine: absolute, at most MaxRepoBytes bytes and no control
// character. Spaces are allowed: the path never reaches a shell as text.
func checkRepo(value string) bool {
	return filepath.IsAbs(value) && len(value) <= MaxRepoBytes && !strings.ContainsFunc(value, unicode.IsControl)
}

// CheckLaunchField returns "" when value is acceptable for the launcher field
// name (without its "--"), otherwise the reason it is not:
// `unknown launcher field "<name>"` or `invalid value for --<name>`.
// A value holding ".." is always invalid, except in repo, whose path is
// cleaned, and in name, which is never part of a path.
func CheckLaunchField(name, value string) string {
	if name == "repo" {
		if !checkRepo(value) {
			return "invalid value for --repo"
		}
		return ""
	}
	pattern, ok := launchFields[name]
	if !ok {
		return `unknown launcher field "` + name + `"`
	}
	if !pattern.MatchString(value) || name != "name" && strings.Contains(value, "..") {
		return "invalid value for --" + name
	}
	return ""
}
