package hook

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// plainCommand is one line of words made only of plain characters: no
	// quote, `;`, `&`, `|`, `$`, backtick, redirect or newline.
	plainCommand = regexp.MustCompile(`^[\w@%+=:,./-]+(?: [\w@%+=:,./-]+)*$`)
	// briefName is a markdown file name that does not start with a dot.
	briefName = regexp.MustCompile(`(?i)^[a-z0-9][\w.-]*\.md$`)
	// valueFlags are the flags of `launch` that take a value, which the
	// dispatcher may use. `--dry-run` takes none.
	valueFlags = map[string]bool{"--preset": true, "--brief": true, "--mode": true, "--field": true, "--worktree": true, "--mcp-exclude": true}
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

// commandProblem is "" when command is the one allowed launcher line: this
// executable, then launch, then only known flags with their values, in direct
// mode.
func (g DispatchGuard) commandProblem(raw any) string {
	command, ok := text(raw)
	if !ok || !plainCommand.MatchString(command) {
		return "the command has characters outside the plain set, or is not one line"
	}
	words := strings.Split(command, " ")
	if len(words) < 2 || !filepath.IsAbs(words[0]) || !g.IsOwn(words[0]) || words[1] != "launch" {
		return `only "` + g.Launcher + ` launch ..." may run`
	}
	for i := 2; i < len(words); i++ {
		flag := words[i]
		if flag == "--dry-run" {
			continue
		}
		if !valueFlags[flag] {
			return `flag "` + flag + `" is not allowed`
		}
		if i+1 >= len(words) || strings.HasPrefix(words[i+1], "-") {
			return flag + " needs a value"
		}
		if flag == "--mode" && words[i+1] != "direct" {
			return "the dispatcher launches in direct mode only"
		}
		i++
	}
	return ""
}

// writeProblem is "" when filePath is a new brief file directly under the
// briefs folder, neither it nor the folder a symlink.
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
