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

// userSettingsShown is the file `jitso setup hook --scope user` writes, as printed.
const userSettingsShown = "~/" + settingsRel

// hooks reads what .claude/settings.json says about the guard, and whether the
// command it registers can start. The file is read from the checkout of the
// working folder (B4-8); settings.local.json is never opened.
func (c *projectChecks) hooks() Line {
	data, err := c.p.ReadFile(filepath.Join(c.root.Checkout, filepath.FromSlash(settingsRel)))
	if errors.Is(err, fs.ErrNotExist) {
		return c.unregistered()
	}
	if err != nil {
		return Line{Fail, "hooks", settingsRel + " cannot be read: " + message(err)}
	}
	if data == nil {
		data = []byte{}
	}
	st := settingsjson.State(data)
	if st.Err != nil {
		return Line{Fail, "hooks", problemText(st.Err, settingsRel)}
	}
	if st.None() {
		return c.unregistered()
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
		parts = append(parts, partial(st, settingsRel)+": run jitso setup hook")
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

// unregistered is the project line when the checkout's file has no JITSO guard.
// A guarded own session is not a problem: launched sessions do not read the
// project file for the guard, so the line only warns when nothing guards the
// owner's session either.
func (c *projectChecks) unregistered() Line {
	if c.userGuard {
		return Line{OK, "hooks", "not registered in this project; your own session is guarded by jitso hook guard in " + userSettingsShown}
	}
	return Line{Warn, "hooks", "not registered: the owner's own session in this project is not guarded (run jitso setup hook --scope user)"}
}

// checkHooksUser reads ~/.claude/settings.json the way hooks reads the
// project's file. The bool is true when the guard is fully registered there
// and the file does not switch every hook off. Not registered is ok: the user
// scope is optional, and the project line carries the warning.
func checkHooksUser(p Ports) (Line, bool) {
	const check = "hooks-user"
	if p.Home == "" {
		return Line{OK, check, "HOME is not set, so there is no user settings file"}, false
	}
	data, err := p.ReadFile(filepath.Join(p.Home, filepath.FromSlash(settingsRel)))
	none := Line{OK, check, "not registered in " + userSettingsShown + " (optional: run jitso setup hook --scope user to guard your own session)"}
	if errors.Is(err, fs.ErrNotExist) {
		return none, false
	}
	if err != nil {
		return Line{Fail, check, userSettingsShown + " cannot be read: " + message(err)}, false
	}
	if data == nil {
		data = []byte{}
	}
	st := settingsjson.State(data)
	if st.Err != nil {
		return Line{Fail, check, problemText(st.Err, userSettingsShown)}, false
	}
	if st.None() {
		return none, false
	}
	var broken []string
	for _, cmd := range st.Commands {
		if why := resolveHook(p, cmd); why != "" {
			broken = append(broken, why)
		}
	}
	if len(broken) > 0 {
		return Line{Fail, check, strings.Join(broken, "; ")}, false
	}
	if !st.Registered() {
		return Line{Warn, check, partial(st, userSettingsShown) + ": run jitso setup hook --scope user"}, false
	}
	detail := "jitso hook guard is registered on PreToolUse (Bash, orca MCP) and SessionEnd in " + userSettingsShown
	if st.DisableAllHooks {
		return Line{Warn, check, detail + "; disableAllHooks is true in " + userSettingsShown + ", so Claude Code runs no hook"}, false
	}
	return Line{OK, check, detail}, true
}

// partial says which of the three slots the guard is on.
func partial(st settingsjson.HookState, shown string) string {
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
		return "jitso hook guard is in " + shown + " but on none of PreToolUse Bash, PreToolUse orca MCP and SessionEnd as a group of its own"
	}
	return "registered on " + strings.Join(have, ", ") + " only (missing " + strings.Join(missing, ", ") + ")"
}

// problemText is a settings problem as one sentence, without the refusal
// ending the writing commands add.
func problemText(err error, shown string) string {
	var p *settingsjson.Problem
	if errors.As(err, &p) {
		return strings.TrimSuffix(p.Message(shown), "; nothing written")
	}
	return shown + ": " + err.Error()
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
func (c *projectChecks) resolveHook(command string) string { return resolveHook(c.p, command) }

func resolveHook(p Ports, command string) string {
	word := firstWord(command)
	if word == "" {
		return "the hook command is empty"
	}
	if !filepath.IsAbs(word) && !strings.ContainsAny(word, `/\`) {
		if _, err := p.LookPath(word); err != nil {
			return word + " is not on PATH"
		}
		return ""
	}
	info, err := p.Stat(word)
	switch {
	case err != nil:
		return word + " does not exist"
	case info.IsDir():
		return word + " is a folder, not a program"
	case p.GOOS != "windows" && info.Mode()&0o100 == 0:
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
