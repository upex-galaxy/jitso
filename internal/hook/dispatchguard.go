package hook

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var (
	// plainWord is one word made only of plain characters: no quote, space, `;`,
	// `&`, `|`, `$`, backtick, redirect or newline.
	plainWord = regexp.MustCompile(`^[\w@%+=:,./-]+$`)
	// quotedText is what a single-quoted word may hold: plain runs separated by
	// single spaces, at least one space (a quote appears only where a space
	// needs it).
	quotedText = regexp.MustCompile(`^[\w@%+=:,./-]+(?: [\w@%+=:,./-]+)+$`)
	// briefName is a markdown file name that does not start with a dot.
	briefName = regexp.MustCompile(`(?i)^[a-z0-9][\w.-]*\.md$`)
	// valueFlags are the flags of `launch` that take a value, which the
	// dispatcher may use. `--dry-run` takes none.
	valueFlags = map[string]bool{"--build": true, "--brief": true, "--mode": true, "--field": true, "--worktree": true, "--mcp-exclude": true}
	// requiredFlags are the flags every launch line carries exactly once
	// (defect N-29), in the order a missing one is reported.
	requiredFlags = []string{"--build", "--brief", "--mode", "--worktree"}
)

// DispatchGuard judges what the dispatcher's claude may do: Bash for exactly
// one launcher line, Write for a new brief inside the briefs folder, nothing
// else. Everything it touches outside the input is injected.
type DispatchGuard struct {
	// DispatchDir is the fixed folder the dispatcher runs from.
	DispatchDir string
	// Launcher is the path the one allowed command must start with, as written
	// (the real path of this executable).
	Launcher string
	// IsOwn reports whether a word names this executable.
	IsOwn func(word string) bool
	// Real resolves symlinks in a path (project.Real).
	Real func(path string) string
	// IsSymlink reports whether path is a symbolic link (Lstat).
	IsSymlink func(path string) bool
	// Exists reports whether anything is at path, a link included (Lstat). Nil
	// means nothing exists.
	Exists func(path string) bool
	// Primary is the real path of the project's primary checkout, named by
	// JITSO_PROJECT: the one folder --worktree may name (defect N-23).
	Primary string
	// PrimaryProblem is why Primary is unknown, "" when it is known. A line
	// that holds --worktree is denied with it.
	PrimaryProblem string
}

// Problem is why the payload is not allowed, "" when it is the one allowed
// dispatcher action.
func (g DispatchGuard) Problem(input map[string]any) string {
	cwd, ok := text(input["cwd"])
	if !ok || g.Real(cwd) != g.Real(g.DispatchDir) {
		return "the session must run in " + g.DispatchDir
	}
	tool, _ := text(input["tool_name"])
	args := input["tool_input"]
	switch tool {
	case "Bash":
		return g.commandProblem(field(args, "command"))
	case "Write":
		return g.writeProblem(field(args, "file_path"))
	}
	return `the tool "` + toolLabel(input["tool_name"]) + `" is not available to the dispatcher`
}

// toolLabel is how a tool name reads in a reason: the name, or "undefined".
func toolLabel(v any) string {
	if s, ok := text(v); ok {
		return s
	}
	if v == nil {
		return "undefined"
	}
	return "unknown"
}

// word is one word of the launcher line. A quoted word was written inside one
// pair of single quotes and holds a space; text is what the shell would pass.
type word struct {
	text   string
	quoted bool
}

// splitWords splits a one-line command into words separated by single spaces.
// A word is plain characters only, or one whole single-quoted word that holds
// a space (quotedText). Anything else (an empty word, a double or trailing
// space, a quote in the middle of a word, an unterminated quote) is not a
// command the guard can read: ok is false.
func splitWords(command string) (words []word, ok bool) {
	i := 0
	for {
		if i >= len(command) {
			return nil, false
		}
		if command[i] == '\'' {
			n := strings.IndexByte(command[i+1:], '\'')
			if n < 0 {
				return nil, false
			}
			end := i + 1 + n + 1
			content := command[i+1 : i+1+n]
			if !quotedText.MatchString(content) || (end < len(command) && command[end] != ' ') {
				return nil, false
			}
			words = append(words, word{text: content, quoted: true})
			i = end
		} else {
			end := strings.IndexByte(command[i:], ' ')
			if end < 0 {
				end = len(command)
			} else {
				end += i
			}
			if !plainWord.MatchString(command[i:end]) {
				return nil, false
			}
			words = append(words, word{text: command[i:end]})
			i = end
		}
		if i >= len(command) {
			return words, true
		}
		i++ // the single space; a trailing one leaves an empty word, caught above
	}
}

// commandProblem is "" when command is the one allowed launcher line: this
// executable, then launch, then only known flags with their values, in any
// order, in direct mode, --worktree only as path:<primary>, and --build,
// --brief, --mode and --worktree each exactly once. A word may be single-quoted
// only when it holds a space: the launcher and the value of --brief.
func (g DispatchGuard) commandProblem(raw any) string {
	command, ok := text(raw)
	if !ok {
		return "the command has characters outside the plain set, or is not one line"
	}
	words, ok := splitWords(command)
	if !ok {
		return "the command has characters outside the plain set, or is not one line"
	}
	if len(words) < 2 || words[1].quoted || !filepath.IsAbs(words[0].text) || !g.IsOwn(words[0].text) || words[1].text != "launch" {
		return `only "` + g.Launcher + ` launch ..." may run`
	}
	seen := map[string]bool{}
	for i := 2; i < len(words); i++ {
		flag := words[i]
		if flag.quoted {
			return "a quoted word is allowed only as the launcher or the value of --brief"
		}
		if flag.text == "--dry-run" {
			continue
		}
		if !valueFlags[flag.text] {
			return `flag "` + flag.text + `" is not allowed`
		}
		if slices.Contains(requiredFlags, flag.text) {
			if seen[flag.text] {
				return flag.text + " is given more than once"
			}
			seen[flag.text] = true
		}
		if i+1 >= len(words) || strings.HasPrefix(words[i+1].text, "-") {
			return flag.text + " needs a value"
		}
		value := words[i+1]
		if value.quoted && flag.text != "--brief" {
			return "a quoted word is allowed only as the launcher or the value of --brief"
		}
		if flag.text == "--mode" && value.text != "direct" {
			return "the dispatcher launches in direct mode only"
		}
		if flag.text == "--worktree" {
			if g.PrimaryProblem != "" {
				return g.PrimaryProblem
			}
			if value.text != "path:"+g.Primary {
				return "--worktree must be path:" + g.Primary
			}
		}
		i++
	}
	for _, flag := range requiredFlags {
		if !seen[flag] {
			return flag + " is required"
		}
	}
	return ""
}

// writeProblem is "" when filePath is a new brief file directly under the
// briefs folder: nothing exists at that name (a file, a folder, a link), and
// neither it nor the folder is a symlink.
func (g DispatchGuard) writeProblem(raw any) string {
	filePath, ok := text(raw)
	if !ok || !filepath.IsAbs(filePath) || strings.ContainsRune(filePath, 0) || containsDotDot(filePath) {
		return "the brief path must be absolute and plain"
	}
	briefs := filepath.Join(g.DispatchDir, "briefs")
	target := filepath.Clean(filePath)
	if filepath.Dir(target) != briefs || !briefName.MatchString(filepath.Base(target)) {
		return "Write is allowed only for <name>.md directly under " + briefs
	}
	if g.IsSymlink(briefs) || g.IsSymlink(target) {
		return "a symlink is not a valid brief location"
	}
	if g.Exists != nil && g.Exists(target) {
		return target + " already exists: write the brief to a new file name"
	}
	return ""
}

func containsDotDot(p string) bool {
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// DispatchReason prefixes a reason with the guard's name.
func DispatchReason(reason string) string { return "JITSO dispatch guard: " + reason }

// AllowedReason is the reason of the one allowed action.
const AllowedReason = "the one allowed dispatcher action"
