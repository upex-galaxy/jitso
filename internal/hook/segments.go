package hook

import (
	"path"
	"regexp"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// harnesses are the programs a terminal may not be started with outside jitso.
var harnesses = map[string]bool{"claude": true, "codex": true, "opencode": true, "pi": true}

var (
	// separators split a shell line into the commands that run: ; && || | &
	// newline $( a backtick and parentheses.
	separators = regexp.MustCompile("\\$\\(|[;&|\n`()]")

	workerStart   = regexp.MustCompile(`\borchestration worker-start\b`)
	dispatchVerb  = regexp.MustCompile(`\borchestration dispatch`)
	agentLaunch   = regexp.MustCompile(`\bagent launch\b`)
	worktreeMake  = regexp.MustCompile(`\bworktree create\b`)
	agentFlag     = regexp.MustCompile(`(?:^| )--agent(?:[ =]|$)`)
	terminalMake  = regexp.MustCompile(`\bterminal create\b`)
	commandFlag   = regexp.MustCompile(`(?:^| )--command(?:[ =]|$)`)
	commandPrefix = regexp.MustCompile(`^ ?--command[ =]?`)
)

// Segments splits a shell line into its commands: cut at every separator,
// quotes removed, runs of whitespace collapsed, empty parts dropped.
func Segments(command string) []string {
	var out []string
	for _, part := range separators.Split(command, -1) {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "'", ""), `"`, "")
		part = jsconv.Trim(jsconv.CollapseSpace(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// Launcher tells which words start the jitso launcher.
type Launcher struct {
	// IsOwn reports whether a word is the path of this very executable.
	IsOwn func(word string) bool
}

// Is reports whether segment is a `<launcher> launch` command: the launcher in
// one of its three forms (this binary's own path, `jitso`, `bun run jitso`)
// after any number of NAME=value words, then the word launch.
func (l Launcher) Is(segment string) bool {
	words := strings.Split(segment, " ")
	i := 0
	for i < len(words) && isAssignment(words[i]) {
		i++
	}
	rest := words[i:]
	switch {
	case len(rest) >= 2 && rest[0] == "jitso":
		return rest[1] == "launch"
	case len(rest) >= 4 && rest[0] == "bun" && rest[1] == "run" && rest[2] == "jitso":
		return rest[3] == "launch"
	case len(rest) >= 2 && l.IsOwn != nil && l.IsOwn(rest[0]):
		return rest[1] == "launch"
	}
	return false
}

// isAssignment is `[^\s=]+=\S+` for one word: a name, an equal sign, a value.
func isAssignment(word string) bool {
	k := strings.IndexByte(word, '=')
	return k >= 1 && k < len(word)-1
}

// LaunchVerb is the way segment launches an agent outside jitso, "" when it
// does not. A `<launcher> launch` segment never does.
func (l Launcher) LaunchVerb(segment string) string {
	if l.Is(segment) {
		return ""
	}
	if workerStart.MatchString(segment) {
		return "orchestration worker-start"
	}
	if dispatchEndsWord(segment) {
		return "orchestration dispatch"
	}
	if agentLaunch.MatchString(segment) {
		return "agent launch"
	}
	if worktreeMake.MatchString(segment) && agentFlag.MatchString(segment) {
		return "worktree create --agent"
	}
	if terminalMake.MatchString(segment) {
		if at := commandFlag.FindStringIndex(segment); at != nil {
			rest := commandPrefix.ReplaceAllString(segment[at[0]:], "")
			if startsHarness(strings.Split(rest, " ")) {
				return "terminal create --command <harness>"
			}
		}
	}
	return ""
}

// dispatchEndsWord is `\borchestration dispatch(?![\w-])`, which RE2 has no
// lookahead for.
func dispatchEndsWord(segment string) bool {
	for _, m := range dispatchVerb.FindAllStringIndex(segment, -1) {
		if m[1] == len(segment) || !isWordOrDash(segment[m[1]]) {
			return true
		}
	}
	return false
}

func isWordOrDash(b byte) bool {
	return b == '-' || b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// startsHarness reports whether one of the first three words is a harness.
func startsHarness(words []string) bool {
	for _, w := range words[:min(3, len(words))] {
		if harnesses[path.Base(w)] {
			return true
		}
	}
	return false
}

// BashVerb is the launch verb of the first segment of command that launches
// an agent outside jitso, "" when none does.
func (l Launcher) BashVerb(command string) string {
	for _, segment := range Segments(command) {
		if verb := l.LaunchVerb(segment); verb != "" {
			return verb
		}
	}
	return ""
}
