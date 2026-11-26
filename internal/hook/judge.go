package hook

import (
	"slices"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// judged is a line judge has already read, so that a text which reaches the
// guard by two ways is read once.
type judged struct {
	line string
	fed  bool
}

// judge is the denial of line read with the lexer, each command on its own
// (each part of a list or a pipeline, and each substitution):
//
//   - the orca launch verbs are looked for in the command. A data command
//     (dataPositions) has its quoted text masked; any other command is read as
//     its words with the quotes removed, because a program may run one of them;
//   - the program of the command is judged: a harness in an invocation that can
//     start a session (startsSession);
//   - the command string of a shell -c, eval and env -S is a line of its own,
//     and so is the --command value of `orca terminal create`;
//   - text a shell reads as commands is live. When a command takes its commands
//     from its input (readsInput), or fed is true (line is itself such text),
//     the heredoc bodies, the here-strings and the quoted text of the data
//     commands are lines of their own. When a variable is run as the program,
//     the heredoc bodies and the here-strings are.
//
// depth counts the lines line is nested in; maxInnerDepth ends the descent. A
// line that does not lex is judged by the old splitter (segmentDenial).
func (l Launcher) judge(line string, depth int, fed bool, seen map[judged]bool) Denial {
	key := judged{line, fed}
	if seen[key] {
		return Denial{}
	}
	seen[key] = true
	cmds, docs, ok := lex(line)
	if !ok {
		return l.segmentDenial(line)
	}
	inner := func(text string, fed bool) Denial {
		if depth >= maxInnerDepth {
			return Denial{}
		}
		return l.judge(text, depth+1, fed, seen)
	}
	reads, runsVariable := fed, false
	var quoted, hereStrings []string
	for _, c := range cmds {
		words := c.texts()
		data, isData := dataPositions(words)
		var text string
		if isData {
			text = matchText(words, data)
			for _, k := range data {
				if strings.IndexFunc(words[k], jsconv.IsSpace) >= 0 {
					quoted = append(quoted, words[k])
				}
			}
		} else {
			text = plainText(words)
		}
		if verb := l.LaunchVerb(text); verb != "" {
			return Denial{Verb: verb}
		}
		for i := 0; i+1 < len(words); i++ {
			if words[i] == "<<<" {
				hereStrings = append(hereStrings, words[i+1])
			}
		}
		if !isData && terminalMake.MatchString(text) {
			for _, script := range commandValues(words) {
				if d := inner(script, false); d.Verb != "" {
					return d
				}
			}
		}
		prog, args, nested := Program(words)
		if directHarnesses[prog] && startsSession(prog, args) {
			return Denial{Verb: prog}
		}
		for _, cmd := range nested {
			script := commandString(prog, cmd)
			if isVariable(script) {
				runsVariable = true
			}
			// A substitution inside the command string builds what runs.
			built := strings.Contains(script, "$(") || strings.Contains(script, "`")
			if d := inner(script, built); d.Verb != "" {
				return d
			}
		}
		plain := withoutRedirects(words)
		if readsInput(plain) {
			reads = true
		}
		prog, args, _ = Program(plain)
		if prog == "" {
			continue
		}
		if isVariable(plain[len(plain)-len(args)-1]) {
			runsVariable = true
		}
		// A process substitution that a shell runs as its script.
		if shells[prog] || prog == "source" || prog == "." {
			for _, a := range args {
				if strings.HasPrefix(a, "<(") && strings.HasSuffix(a, ")") {
					if d := inner(a[2:len(a)-1], true); d.Verb != "" {
						return d
					}
				}
			}
		}
	}
	var live []string
	if reads || runsVariable {
		live = slices.Concat(docs, hereStrings)
	}
	if reads {
		live = append(live, quoted...)
	}
	for _, text := range live {
		if d := inner(text, false); d.Verb != "" {
			return d
		}
	}
	return Denial{}
}

// plainText is a command that is not a data command as the matchers read it:
// its words joined, every quote removed and the whitespace collapsed.
func plainText(words []string) string {
	text := strings.NewReplacer("'", "", `"`, "").Replace(strings.Join(words, " "))
	return jsconv.Trim(jsconv.CollapseSpace(text))
}

// commandString is the line that prog (a shell, eval or env -S) runs, given the
// words Program handed back: a shell runs the first one that is not an option,
// the others join all.
func commandString(prog string, words []string) string {
	if !shells[prog] {
		return strings.Join(words, " ")
	}
	for _, w := range words {
		if !strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "+") {
			return w
		}
	}
	return ""
}

// commandValues are the values of the --command flags among words: the lines a
// terminal that orca creates runs.
func commandValues(words []string) []string {
	var out []string
	for i, w := range words {
		if value, ok := strings.CutPrefix(w, "--command="); ok {
			out = append(out, value)
		} else if w == "--command" && i+1 < len(words) {
			out = append(out, words[i+1])
		}
	}
	return out
}

// isVariable reports whether word is nothing but one variable: $X or ${X}.
func isVariable(word string) bool {
	name, ok := strings.CutPrefix(word, "$")
	if !ok {
		return false
	}
	if in, ok := strings.CutPrefix(name, "{"); ok {
		if name, ok = strings.CutSuffix(in, "}"); !ok {
			return false
		}
	}
	return name != "" && strings.IndexFunc(name, func(r rune) bool {
		return !(r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
	}) < 0
}

// stdinNames are the paths that name the standard input.
var stdinNames = map[string]bool{"-": true, "/dev/stdin": true, "/dev/fd/0": true}

// readsInput reports whether a command, given without its redirections, takes
// commands from its input: a shell with no -c word and no script, any shell
// that xargs runs, source of the standard input, and `sudo -s` or `sudo -i`
// with no command.
func readsInput(words []string) bool {
	prog, args, nested := Program(words)
	switch {
	case shells[prog]:
		if slices.ContainsFunc(words[:len(words)-len(args)-1], func(w string) bool { return Name(w) == "xargs" }) {
			return true
		}
		return nested == nil && noScript(args)
	case prog == "source" || prog == ".":
		return len(args) > 0 && stdinNames[args[0]]
	case prog == "":
		at := slices.IndexFunc(words, func(w string) bool { return Name(w) == "sudo" })
		return at >= 0 && slices.ContainsFunc(words[at+1:], func(w string) bool {
			return w == "-s" || w == "-i" || w == "--shell" || w == "--login"
		})
	}
	return false
}

// noScript reports whether the arguments of a shell without -c leave it to read
// its commands from the standard input.
func noScript(args []string) bool {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--version" || a == "--help":
			return false
		case stdinNames[a]:
			return true
		case a == "-o" || a == "+o":
			i++
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "s"):
			return true
		case strings.HasPrefix(a, "-") || strings.HasPrefix(a, "+"):
		default:
			return false
		}
	}
	return true
}
