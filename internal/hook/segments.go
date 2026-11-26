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

// Segments is the old splitter, used for a line the lexer cannot parse: it
// splits a shell line into its commands by cutting at every separator without reading
// quotes, removes the quotes, collapses runs of whitespace and drops empty
// parts. It leans toward deny: text after ( , a backtick or a newline is read
// as a command.
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

// Denial is a Bash command or a tool call the guard refuses.
type Denial struct {
	// Verb is what was denied, the word of `(denied: <verb>)`. Empty when the
	// call is allowed.
	Verb string
}

// BashDenial is the denial of the first part of command that starts an agent
// outside jitso: an orca launch verb, or a harness (claude, codex, opencode or
// pi) run as a program of its own in an invocation that starts a session; the
// zero Denial when none does. A line the shell lexer (lex) parses is judged
// command by command (judge); a line it cannot parse is judged by the old
// splitter (segmentDenial).
func (l Launcher) BashDenial(command string) Denial {
	return l.judge(command, 0, false, map[judged]bool{})
}

// segmentDenial is the judgment of the old splitter: Segments, every quote
// removed, nothing masked.
func (l Launcher) segmentDenial(command string) Denial {
	for _, segment := range Segments(command) {
		if verb := l.LaunchVerb(segment); verb != "" {
			return Denial{Verb: verb}
		}
		words := strings.Split(segment, " ")
		if name, ok := HarnessProgram(words); ok {
			return Denial{Verb: name}
		}
	}
	return Denial{}
}

// BashVerb is the verb of BashDenial: "" when nothing is denied.
func (l Launcher) BashVerb(command string) string {
	return l.BashDenial(command).Verb
}
