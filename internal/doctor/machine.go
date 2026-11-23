package doctor

import (
	"fmt"
	"path/filepath"
	"strings"

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
		return Line{Fail, "orca", "orca is not on PATH; install Orca from " + orca.InstallURL + " and open it once"}
	}
	return Line{OK, "orca", found + from}
}

// checkClaude finds claude and asks its version. A program that answers with
// nothing, fails or hangs is still a claude that is there. Without claude it
// fails only when no other harness (codex, opencode, pi) is on PATH either.
func checkClaude(p Ports) Line {
	found, err := p.LookPath("claude")
	if err != nil {
		// A build may run in another harness: without claude, the machine
		// can still launch those builds.
		var others []string
		for _, h := range []string{"codex", "opencode", "pi"} {
			if _, err := p.LookPath(h); err == nil {
				others = append(others, h)
			}
		}
		if len(others) > 0 {
			return Line{Warn, "claude", "claude is not on PATH; builds with harness: claude cannot launch here (found: " + strings.Join(others, ", ") + ")"}
		}
		return Line{Fail, "claude", "claude is not on PATH"}
	}
	res := p.Run("", found, "--version")
	if v := firstLine(res.Stdout); res.Err == nil && res.Exit == 0 && v != "" {
		return Line{OK, "claude", found + ", " + v}
	}
	return Line{OK, "claude", found + ", version unknown"}
}

// checkProject resolves the project of the working folder. ok is false
// outside a git checkout: the project checks are left out.
func checkProject(p Ports) (line Line, root project.Root, ok bool) {
	root, err := p.Resolve()
	if err != nil {
		return Line{Warn, "project", message(err) + "; the project checks are skipped"}, project.Root{}, false
	}
	detail := root.Primary
	if root.Linked {
		detail += ", checkout " + root.Checkout + " (linked worktree)"
	}
	return Line{OK, "project", detail}, root, true
}
