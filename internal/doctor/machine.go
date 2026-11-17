package doctor

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/project"
)

// checkBinary names this binary: its version and the real path of the
// executable. self is that path, "" when it cannot be resolved.
func checkBinary(p Ports) (line Line, self string) {
	self, err := project.Self(p.Executable)
	if err != nil {
		return Line{Fail, "binary", "cannot resolve the path of this binary: " + message(err)}, ""
	}
	return Line{OK, "binary", fmt.Sprintf("jitso %s at %s", p.Version, self)}, self
}

// checkPath compares the jitso a shell would start with this binary: the hook
// command and the instructions block both say `jitso`.
func checkPath(p Ports, self string) Line {
	found, err := p.LookPath("jitso")
	if err != nil {
		return Line{Warn, "path", "jitso is not on PATH"}
	}
	if abs, err := filepath.Abs(found); err == nil {
		found = abs
	}
	real := project.Real(found)
	if self == "" || real == self {
		if self == "" {
			return Line{OK, "path", "jitso on PATH is " + found}
		}
		return Line{OK, "path", "jitso on PATH is this binary"}
	}
	return Line{Warn, "path", fmt.Sprintf("jitso on PATH is %s, not this binary (%s)", found, self)}
}

// checkGit runs `git --version`: every project command needs git.
func checkGit(p Ports) Line {
	if _, err := p.LookPath("git"); err != nil {
		return Line{Fail, "git", "git is not on PATH"}
	}
	res := p.Run("", "git", "--version")
	if res.Err != nil || res.Exit != 0 {
		return Line{Fail, "git", "git --version failed: " + runProblem(res)}
	}
	return Line{OK, "git", orDefault(firstLine(res.Stdout), "git is on PATH")}
}

// runProblem says why a run failed.
func runProblem(res Result) string {
	if res.Err != nil {
		return message(res.Err)
	}
	return fmt.Sprintf("exit %d", res.Exit)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// checkShell: `launch` runs through /bin/sh.
func checkShell(p Ports) Line {
	if p.GOOS == "windows" {
		return Line{Warn, "shell", "launch needs /bin/sh and does not run on windows"}
	}
	if _, err := p.Stat("/bin/sh"); err != nil {
		return Line{Fail, "shell", "/bin/sh is missing: launch needs it"}
	}
	return Line{OK, "shell", "/bin/sh is present"}
}

// checkOrca looks the orca program up on PATH and never runs it: a call is
// logged by the contract's fake, and a real orca may start an app.
func checkOrca(p Ports) Line {
	name, from := orca.DefaultCommand, ""
	if v := p.Getenv(orca.CommandEnv); v != "" {
		name, from = v, " (from "+orca.CommandEnv+")"
	}
	found, err := p.LookPath(name)
	if err != nil {
		if from != "" {
			return Line{Fail, "orca", fmt.Sprintf("%s is set to %s, which is not an executable on PATH", orca.CommandEnv, name)}
		}
		return Line{Fail, "orca", "orca is not on PATH"}
	}
	return Line{OK, "orca", found + from}
}

// checkClaude finds claude and asks its version. A program that answers with
// nothing, fails or hangs is still a claude that is there.
func checkClaude(p Ports) Line {
	found, err := p.LookPath("claude")
	if err != nil {
		return Line{Fail, "claude", "claude is not on PATH"}
	}
	res := p.Run("", found, "--version")
	if v := firstLine(res.Stdout); res.Err == nil && res.Exit == 0 && v != "" {
		return Line{OK, "claude", found + ", " + v}
	}
	return Line{OK, "claude", found + ", version unknown"}
}

// userConfigShown is the file as people see it.
const userConfigShown = "~/.jitso/" + catalog.UserConfigFile

// checkUserConfig loads ~/.jitso/config.yaml through the catalog, which owns
// the format, and lists the trusted folders that do not exist (the catalog
// skips those silently).
func checkUserConfig(p Ports) Line {
	if p.Home == "" {
		return Line{Warn, "user-config", "HOME is not set, so there is no " + userConfigShown}
	}
	cfg, err := catalog.LoadUserConfig(p.Home)
	if err != nil {
		return Line{Fail, "user-config", message(err)}
	}
	data, err := p.ReadFile(catalog.UserConfigPath(p.Home))
	if errors.Is(err, fs.ErrNotExist) {
		return Line{OK, "user-config", "no " + userConfigShown}
	}
	if err != nil {
		return Line{Fail, "user-config", userConfigShown + " cannot be read: " + message(err)}
	}
	detail := userConfigShown + ": " + plural(len(cfg.TrustedParents), "trusted parent")
	if missing := missingTrusted(p, data); len(missing) > 0 {
		return Line{Warn, "user-config", detail + "; not found, so skipped: " + strings.Join(missing, ", ")}
	}
	return Line{OK, "user-config", detail}
}

// missingTrusted lists the trusted_parents entries, as written, whose folder
// does not exist. The catalog is the only package that decodes YAML, and it
// drops such an entry without a word, so this reads the one shape the config
// documents, a block list under `trusted_parents:`; a flow list
// (`[a, b]`) is not listed here. It runs only on a file the catalog accepted.
func missingTrusted(p Ports, data []byte) []string {
	var missing []string
	inList := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		item, isItem := strings.CutPrefix(trimmed, "- ")
		if indent == 0 && !isItem {
			inList = strings.HasPrefix(line, "trusted_parents:")
			continue
		}
		if !inList || !isItem {
			continue
		}
		entry := yamlScalar(item)
		dir := entry
		if rest, ok := strings.CutPrefix(entry, "~/"); ok {
			dir = filepath.Join(p.Home, filepath.FromSlash(rest))
		}
		if entry == "" || !filepath.IsAbs(dir) {
			continue
		}
		if _, err := p.Stat(dir); err != nil {
			missing = append(missing, entry)
		}
	}
	return missing
}

// yamlScalar is a list item's text without a trailing comment and quotes.
func yamlScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 0 && (s[0] == '"' || s[0] == '\'') {
		if end := strings.IndexByte(s[1:], s[0]); end >= 0 {
			return s[1 : 1+end]
		}
		return strings.Trim(s, `"'`)
	}
	if i := strings.Index(s, " #"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// checkProject resolves the project of the working folder. ok is false
// outside a git checkout: the project checks are left out.
func checkProject(p Ports) (line Line, root project.Root, ok bool) {
	root, err := p.Resolve()
	if err != nil {
		return Line{Fail, "project", message(err)}, project.Root{}, false
	}
	detail := root.Primary
	if root.Linked {
		detail += ", checkout " + root.Checkout + " (linked worktree)"
	}
	return Line{OK, "project", detail}, root, true
}
