package shell

import (
	"regexp"
	"strings"
)

// The character classes are spelled out instead of using (?i): Go folds case
// with Unicode rules, so (?i)[A-Z] would also match U+017F and U+212A, which the
// oracle's JavaScript patterns (no u flag) reject.
var (
	launchID = regexp.MustCompile(`^L[0-9]{8}T[0-9]{6}-[0-9a-f]{4}$`)
	orcaID   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)
)

// launchFields is the allowlist per launcher field (the oracle's
// checkLaunchField table): plain characters only, never starting with "-".
var launchFields = map[string]*regexp.Regexp{
	"build":       regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`),
	"run":         orcaID,
	"task":        orcaID,
	"parent":      launchID,
	"worktree":    regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,511}$`),
	"mcp-exclude": regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`),
}

// CheckLaunchField returns "" when value is acceptable for the launcher field
// name (without its "--"), otherwise the reason it is not:
// `unknown launcher field "<name>"` or `invalid value for --<name>`.
// A value holding ".." is always invalid.
func CheckLaunchField(name, value string) string {
	pattern, ok := launchFields[name]
	if !ok {
		return `unknown launcher field "` + name + `"`
	}
	if !pattern.MatchString(value) || strings.Contains(value, "..") {
		return "invalid value for --" + name
	}
	return ""
}
