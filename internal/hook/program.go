package hook

import (
	"path"
	"strings"
)

// directHarnesses are the programs a session may not run as a program of its
// own (defect N-24): the four harnesses `jitso setup hook` guards (claude,
// codex, opencode, pi). It holds the same names as `harnesses`, the agents of
// `orca terminal create --command`, but the two rules are judged apart.
var directHarnesses = map[string]bool{"claude": true, "codex": true, "opencode": true, "pi": true}

// maxInnerDepth is how many -c, eval or env -S levels HarnessProgram follows.
const maxInnerDepth = 8

// keywords are the shell words that start a command without being one.
var keywords = map[string]bool{"!": true, "{": true, "if": true, "then": true, "else": true, "elif": true, "do": true, "while": true, "until": true}

// shells are the programs that run the command string after a -c word.
var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "mksh": true, "fish": true}

// wrappers are the programs that only run another program: the options each
// skips are in skipWrapper.
var wrappers = map[string]bool{
	"env": true, "command": true, "exec": true, "nohup": true, "time": true, "sudo": true, "nice": true,
	"timeout": true, "stdbuf": true, "xargs": true, "caffeinate": true,
}

// Name is how a word reads when it is matched against a program name:
// backslashes removed, the path cut to its base name, ASCII lowercased (macOS's
// default volume is case-insensitive, so CLAUDE runs claude).
func Name(word string) string {
	word = strings.ReplaceAll(word, `\`, "")
	word = path.Base(word)
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}, word)
}

// Program is the program a segment runs and its arguments: the first word
// after NAME=value words, shell keywords and the wrappers (env, command, exec,
// nohup, time, sudo, nice, timeout, stdbuf, xargs, caffeinate, each with the
// options it takes), as Name reads it. A shell with a -c word, eval and env -S
// hand the words that follow back as inner, a command to judge the same way
// (HarnessProgram follows at most maxInnerDepth levels); prog is then the shell
// or eval itself. prog is "" when no program is left (`command -v claude` is a
// lookup, and so is a line that is only keywords and wrappers). The words are
// those of one segment (Segments), split on spaces.
func Program(words []string) (prog string, args []string, inner [][]string) {
	i := 0
	for i < len(words) {
		w := words[i]
		name := Name(w)
		switch {
		case isAssignment(w), keywords[w]:
			i++
		case name == "eval":
			return name, words[i+1:], [][]string{words[i+1:]}
		case shells[name]:
			rest := words[i+1:]
			if cmd, ok := shellCommand(rest); ok {
				return name, rest, [][]string{cmd}
			}
			return name, rest, nil
		case wrappers[name]:
			next, cmd, lookup := skipWrapper(name, words[i+1:])
			if lookup {
				return "", nil, nil
			}
			if cmd != nil {
				return name, words[i+1:], [][]string{cmd}
			}
			i = len(words) - len(next)
		default:
			return name, words[i+1:], nil
		}
	}
	return "", nil, nil
}

// shellCommand is the words after the first -c word of a shell's options: a
// single-dash word that holds a c (-c, -lc, -ec), where -o takes the next
// word. Quotes are already gone, so every word after it is the command string.
// ok is false when a word that is not an option comes first (a script path).
func shellCommand(words []string) (cmd []string, ok bool) {
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch {
		case w == "-o" || w == "+o":
			i++
		case strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "--") && strings.Contains(w, "c"):
			return words[i+1:], true
		case strings.HasPrefix(w, "-") || strings.HasPrefix(w, "+"):
		default:
			return nil, false
		}
	}
	return nil, false
}

// skipWrapper skips the options of wrapper name in words, the words after the
// wrapper, and returns what is left (the program and its arguments). For env,
// cmd is the rest after -S or --split-string, a command line to judge. lookup
// is true for `command -v` and `command -V`, which only look a name up.
func skipWrapper(name string, words []string) (next, cmd []string, lookup bool) {
	// takes are the options that consume the next word.
	var takes map[string]bool
	switch name {
	case "env":
		takes = map[string]bool{"-u": true, "-C": true}
	case "exec":
		takes = map[string]bool{"-a": true}
	case "time":
		takes = map[string]bool{"-f": true, "-o": true}
	case "sudo":
		takes = map[string]bool{"-u": true, "-g": true, "-h": true, "-p": true, "-C": true, "-D": true, "-r": true, "-t": true, "-U": true, "-T": true}
	case "nice":
		takes = map[string]bool{"-n": true}
	case "timeout":
		takes = map[string]bool{"-s": true, "-k": true}
	case "stdbuf":
		takes = map[string]bool{"-i": true, "-o": true, "-e": true}
	case "xargs":
		takes = map[string]bool{"-I": true, "-L": true, "-n": true, "-P": true, "-s": true, "-d": true, "-E": true, "-a": true}
	case "caffeinate":
		takes = map[string]bool{"-t": true, "-w": true}
	}
	i := 0
	for i < len(words) {
		w := words[i]
		switch {
		case name == "command" && strings.HasPrefix(w, "-") && w != "-" && strings.ContainsAny(w, "vV"):
			return nil, nil, true
		case name == "env" && (w == "-S" || w == "--split-string"):
			return nil, words[i+1:], false
		case name == "sudo" && w == "--":
			i++
			return words[i:], nil, false
		case name == "env" && isAssignment(w):
			i++
		case strings.HasPrefix(w, "-") && w != "-":
			i++
			if takes[w] {
				i++
			}
		case name == "sudo" && isAssignment(w):
			i++
		default:
			if name == "timeout" {
				// the one duration word before the program
				i++
			}
			return words[min(i, len(words)):], nil, false
		}
	}
	return nil, nil, false
}

// HarnessProgram is the harness a segment runs as a program of its own: the
// program of its words, or of an inner -c, eval or env -S command up to
// maxInnerDepth levels deep, when it is claude, codex, opencode or pi. name is
// that program as Name reads it.
func HarnessProgram(words []string) (name string, ok bool) {
	return harnessAt(words, 0)
}

func harnessAt(words []string, depth int) (string, bool) {
	prog, _, inner := Program(words)
	if directHarnesses[prog] {
		return prog, true
	}
	if depth >= maxInnerDepth {
		return "", false
	}
	for _, cmd := range inner {
		if name, ok := harnessAt(cmd, depth+1); ok {
			return name, true
		}
	}
	return "", false
}
