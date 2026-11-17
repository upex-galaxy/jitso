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

	// buildSave is the words `jitso build save` as a word of their own: at the
	// start of a segment, or after a space or a slash (a path ending in jitso).
	// Segments has already removed the quotes, so `sh -c 'jitso build save x'`
	// reads `sh -c jitso build save x`.
	buildSave = regexp.MustCompile(`(?:^|[ /])jitso build save\b`)
	// adoption is the words `jitso setup` or `jitso install` as a word of their
	// own, found like buildSave. The verb is a whole word: `jitso setup-x` is
	// another command.
	adoption = regexp.MustCompile(`(?:^|[ /])jitso (setup|install)(?: |$)`)
	// buildIDWord is a word that can be a build id.
	buildIDWord = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
)

// VerbBuildSave is the verb the guard names when it denies `jitso build save`,
// as `(denied: jitso build save)` ends the reason.
const VerbBuildSave = "jitso build save"

// The verbs the guard names when it denies an adoption command, as
// `(denied: jitso setup)` or `(denied: jitso install)` ends the reason.
const (
	VerbSetup   = "jitso setup"
	VerbInstall = "jitso install"
)

// dryRunFlag is the word that makes an adoption command only read.
const dryRunFlag = "--dry-run"

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

// BuildSave reports whether segment runs `jitso build save`: the words
// `jitso build save` anywhere in it (the bare word, `bun run jitso`, a path
// ending in jitso, after NAME=value words, inside `sh -c '...'`), or a word
// that is this executable followed by `build save`. The text is judged, not the
// program, so a commit message that holds those words matches too. id is the
// build id the command names, "<id>" when it names none or something that is
// not an id, so a reason never echoes arbitrary text.
func (l Launcher) BuildSave(segment string) (id string, ok bool) {
	if at := buildSave.FindStringIndex(segment); at != nil {
		return saveID(strings.Fields(segment[at[1]:])), true
	}
	words := strings.Split(segment, " ")
	for i, w := range words {
		if l.IsOwn != nil && i+2 < len(words) && words[i+1] == "build" && words[i+2] == "save" && l.IsOwn(w) {
			return saveID(words[i+3:]), true
		}
	}
	return "", false
}

// Adoption reports whether segment runs `jitso setup` or `jitso install`, the
// commands only the owner runs: they change the project's hooks, instruction
// files and skill files. It finds them like BuildSave (the bare word,
// `bun run jitso`, a path ending in jitso, this executable's path, after
// NAME=value words, inside `sh -c '...'`) and judges the text, not the program.
// verb is VerbSetup or VerbInstall. A segment that runs jitso directly and
// holds the standalone word --dry-run after the command is a dry run, which
// writes nothing, so it is not an adoption; behind `sh -c` or another
// wrapper the flag proves nothing and the segment is still denied.
func (l Launcher) Adoption(segment string) (verb string, ok bool) {
	var rest string
	direct := false
	if at := adoption.FindStringSubmatchIndex(segment); at != nil {
		verb, rest = "jitso "+segment[at[2]:at[3]], segment[at[1]:]
		direct = directPrefix(segment[:at[0]], segment[at[0]] == '/')
	} else {
		words := strings.Split(segment, " ")
		for i, w := range words {
			if l.IsOwn != nil && i+1 < len(words) && (words[i+1] == "setup" || words[i+1] == "install") && l.IsOwn(w) {
				verb, rest = "jitso "+words[i+1], strings.Join(words[i+2:], " ")
				direct = directPrefix(strings.Join(words[:i], " "), false)
				break
			}
		}
	}
	if verb == "" {
		return "", false
	}
	if direct {
		for _, w := range strings.Fields(rest) {
			if w == dryRunFlag {
				return "", false
			}
		}
	}
	return verb, true
}

// directPrefix reports whether the text before the launcher word runs jitso
// itself, so a --dry-run among its arguments reaches jitso: only NAME=value
// words, `bun run`, and (when the launcher word is a path, pathTail) the
// folder part of that path. Anything else (`sh -c`, `eval`, `xargs`) may hand
// the arguments to another program, so it never earns the dry-run exemption.
func directPrefix(prefix string, pathTail bool) bool {
	words := strings.Fields(prefix)
	for i, w := range words {
		switch {
		case isAssignment(w), w == "bun", w == "run":
		case pathTail && i == len(words)-1 && (strings.HasPrefix(w, "/") || strings.HasPrefix(w, ".") || strings.HasPrefix(w, "~")):
		default:
			return false
		}
	}
	return true
}

// saveID is the draft id among the words after `build save`: the first word
// that is not a flag (or the value of --confirm) and looks like an id.
func saveID(words []string) string {
	for i := 0; i < len(words); i++ {
		switch w := words[i]; {
		case w == "--confirm":
			i++
		case strings.HasPrefix(w, "-"):
		case buildIDWord.MatchString(w):
			return w
		default:
			return "<id>"
		}
	}
	return "<id>"
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
	// SaveID is the build id of a denied `jitso build save`.
	SaveID string
}

// BashDenial is the denial of the first segment of command that launches an
// agent outside jitso, runs a harness (claude, codex or opencode) as a program
// of its own, runs `jitso build save`, `jitso setup` or `jitso install`
// (other than a dry run); the zero Denial when none does.
func (l Launcher) BashDenial(command string) Denial {
	for _, segment := range Segments(command) {
		if id, ok := l.BuildSave(segment); ok {
			return Denial{Verb: VerbBuildSave, SaveID: id}
		}
		if verb, ok := l.Adoption(segment); ok {
			return Denial{Verb: verb}
		}
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

// BashVerb is the verb of BashDenial: "" when no segment is denied.
func (l Launcher) BashVerb(command string) string {
	return l.BashDenial(command).Verb
}
