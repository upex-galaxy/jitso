package doctor

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/settingsjson"
)

// settingsRel is the file `jitso setup hook` writes, in the checkout.
const settingsRel = ".claude/settings.json"

// hooks reads what .claude/settings.json says about the guard, and whether the
// command it registers can start. The file is read from the checkout of the
// working folder (B4-8); settings.local.json is never opened.
func (c *projectChecks) hooks() Line {
	data, err := c.p.ReadFile(filepath.Join(c.root.Checkout, filepath.FromSlash(settingsRel)))
	if errors.Is(err, fs.ErrNotExist) {
		return Line{Warn, "hooks", "not registered: run jitso setup hook"}
	}
	if err != nil {
		return Line{Fail, "hooks", settingsRel + " cannot be read: " + message(err)}
	}
	if data == nil {
		data = []byte{}
	}
	st := settingsjson.State(data)
	if st.Err != nil {
		return Line{Fail, "hooks", problemText(st.Err)}
	}
	if st.None() {
		return Line{Warn, "hooks", "not registered: run jitso setup hook"}
	}

	var broken []string
	for _, cmd := range st.Commands {
		if why := c.resolveHook(cmd); why != "" {
			broken = append(broken, why)
		}
	}
	if len(broken) > 0 {
		return Line{Fail, "hooks", strings.Join(broken, "; ")}
	}

	var parts []string
	if st.Registered() {
		parts = append(parts, "jitso hook guard is registered on PreToolUse (Bash, orca MCP) and SessionEnd in "+settingsRel)
	} else {
		parts = append(parts, partial(st)+": run jitso setup hook")
	}
	warned := !st.Registered()
	if st.DisableAllHooks {
		parts = append(parts, "disableAllHooks is true in "+settingsRel+", so Claude Code runs no hook")
		warned = true
	}
	if c.untracked(settingsRel) {
		parts = append(parts, settingsRel+" is not tracked by git: commit it so linked worktrees and other clones get the guard")
		warned = true
	}
	if warned {
		return Line{Warn, "hooks", strings.Join(parts, "; ")}
	}
	return Line{OK, "hooks", parts[0]}
}

// partial says which of the three slots the guard is on.
func partial(st settingsjson.HookState) string {
	var have, missing []string
	for _, s := range []struct {
		on   bool
		name string
	}{{st.Bash, "PreToolUse Bash"}, {st.MCP, "PreToolUse orca MCP"}, {st.SessionEnd, "SessionEnd"}} {
		if s.on {
			have = append(have, s.name)
		} else {
			missing = append(missing, s.name)
		}
	}
	if len(have) == 0 {
		return "jitso hook guard is in " + settingsRel + " but on none of PreToolUse Bash, PreToolUse orca MCP and SessionEnd as a group of its own"
	}
	return "registered on " + strings.Join(have, ", ") + " only (missing " + strings.Join(missing, ", ") + ")"
}

// problemText is a settings problem as one sentence, without the refusal
// ending the writing commands add.
func problemText(err error) string {
	var p *settingsjson.Problem
	if errors.As(err, &p) {
		return strings.TrimSuffix(p.Message(settingsRel), "; nothing written")
	}
	return settingsRel + ": " + err.Error()
}

// untracked reports whether git does not track rel in the checkout. A git that
// cannot answer counts as tracked: the warning is never worth a false claim.
func (c *projectChecks) untracked(rel string) bool {
	exit, ok := c.gitSays(c.root.Checkout, "ls-files", "--error-unmatch", "--", rel)
	return ok && exit == 1
}

// resolveHook says why a hook command cannot start, "" when it can. The first
// word is the program: a bare word is looked up on PATH (the guard needs PATH
// at hook time, and doctor's own PATH is the proxy), an absolute path must be
// a file with the owner execute bit.
func (c *projectChecks) resolveHook(command string) string {
	word := firstWord(command)
	if word == "" {
		return "the hook command is empty"
	}
	if !filepath.IsAbs(word) && !strings.ContainsAny(word, `/\`) {
		if _, err := c.p.LookPath(word); err != nil {
			return word + " is not on PATH"
		}
		return ""
	}
	info, err := c.p.Stat(word)
	switch {
	case err != nil:
		return word + " does not exist"
	case info.IsDir():
		return word + " is a folder, not a program"
	case c.p.GOOS != "windows" && info.Mode()&0o100 == 0:
		return word + " is not executable"
	}
	return ""
}

// firstWord is the first word of a shell command line: single quotes, double
// quotes and the '\” idiom jitso writes for a path with a quote in it are
// removed.
func firstWord(command string) string {
	var b strings.Builder
	var quote byte
	for i := 0; i < len(command); i++ {
		ch := command[i]
		switch {
		case quote != 0:
			if ch == quote {
				quote = 0
			} else {
				b.WriteByte(ch)
			}
		case ch == '\'' || ch == '"':
			quote = ch
		case ch == '\\' && i+1 < len(command):
			i++
			b.WriteByte(command[i])
		case ch == ' ' || ch == '\t':
			return b.String()
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}
