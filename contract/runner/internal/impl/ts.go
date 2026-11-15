package impl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"jitso-contract-runner/internal/sandbox"
)

// TS is the frozen TypeScript oracle. The TS code finds its project from its own file
// location, so each sandbox gets its own copy of `cli/jitso/` (tests left out), of
// `cli/lib/worktree.ts` and of the two hooks; `node_modules` is a symlink to the checkout's.
type TS struct {
	src   string
	bun   string
	node  string
	cache string
}

var tsFiles = []string{"cli/lib/worktree.ts", ".agents/hooks/jitso-guard.mjs", ".agents/hooks/jitso-dispatch-guard.mjs"}

// NewTS checks the checkout and finds bun and node, failing early with a clear message.
func NewTS(src string) (*TS, error) {
	if src == "" {
		return nil, fmt.Errorf("--impl ts needs --ts-src <boilerplate checkout>")
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		return nil, err
	}
	if abs, err = filepath.EvalSymlinks(abs); err != nil {
		return nil, fmt.Errorf("--ts-src: %v", err)
	}
	for _, rel := range append([]string{"cli/jitso/index.ts", "node_modules"}, tsFiles...) {
		if _, err := os.Stat(filepath.Join(abs, rel)); err != nil {
			return nil, fmt.Errorf("--ts-src %s: missing %s", abs, rel)
		}
	}
	t := &TS{src: abs, cache: filepath.Join(os.TempDir(), "jitso-contract-bun-cache")}
	for _, tool := range []struct {
		name string
		dst  *string
	}{{"bun", &t.bun}, {"node", &t.node}} {
		path, err := exec.LookPath(tool.name)
		if err != nil {
			return nil, fmt.Errorf("--impl ts needs %s on PATH: %v", tool.name, err)
		}
		if *tool.dst, err = filepath.Abs(path); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func (t *TS) Name() string      { return "ts" }
func (t *TS) StatusKey() string { return "ts" }

// Prepare copies the oracle into the project and links bun and node into tools/.
func (t *TS) Prepare(sb *sandbox.Sandbox) error {
	skipTests := func(rel string) bool { return strings.HasSuffix(rel, ".test.ts") }
	if err := sandbox.CopyTree(filepath.Join(t.src, "cli", "jitso"), filepath.Join(sb.Project, "cli", "jitso"), skipTests); err != nil {
		return err
	}
	for _, rel := range tsFiles {
		dst := filepath.Join(sb.Project, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(t.src, rel))
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	if err := os.Symlink(filepath.Join(t.src, "node_modules"), filepath.Join(sb.Project, "node_modules")); err != nil {
		return err
	}
	if err := sb.LinkTool("bun", t.bun); err != nil {
		return err
	}
	if err := sb.LinkTool("node", t.node); err != nil {
		return err
	}
	if err := linkPresets(sb.Project); err != nil {
		return err
	}
	// bun keeps a transpiler cache under $HOME by default; keep it out of the sandbox HOME.
	sb.SetEnv("BUN_RUNTIME_TRANSPILER_CACHE_PATH", t.cache)
	return nil
}

// linkPresets makes the oracle see a project that uses the new folder name: when
// `jitso/builds` is a folder and `jitso/presets` does not exist, `jitso/presets` becomes a
// relative symlink to `builds`. A symlink, not a copy, so a `write` step that adds a build
// mid-case is seen. Removed together with the oracle.
func linkPresets(project string) error {
	dir := filepath.Join(project, "jitso")
	if info, err := os.Stat(filepath.Join(dir, "builds")); err != nil || !info.IsDir() {
		return nil
	}
	link := filepath.Join(dir, "presets")
	if _, err := os.Lstat(link); err == nil {
		return nil
	}
	return os.Symlink("builds", link)
}

// oracleArgs translates the new build vocabulary into the oracle's old one, in that one
// direction only: every word equal to `--build` becomes `--preset` when the command is
// `launch`, and a leading `build list` becomes `presets list`. Every other word, the old
// spellings included, passes through unchanged.
func oracleArgs(args []string) []string {
	out := append([]string(nil), args...)
	switch {
	case len(out) > 0 && out[0] == "launch":
		for i, a := range out {
			if a == "--build" {
				out[i] = "--preset"
			}
		}
	case len(out) > 1 && out[0] == "build" && out[1] == "list":
		out[0] = "presets"
	}
	return out
}

// Command runs the sandbox copy. It refuses a script outside the sandbox: the TS code
// resolves its project from its own location, so running the checkout's copy would write
// into the real repo.
func (t *TS) Command(sb *sandbox.Sandbox, args []string) ([]string, error) {
	script := filepath.Join(sb.Project, "cli", "jitso", "index.ts")
	if !sb.Inside(script) {
		return nil, fmt.Errorf("refusing to run %s: not inside the sandbox", script)
	}
	return append([]string{filepath.Join(sb.Tools, "bun"), script}, oracleArgs(args)...), nil
}

// Launcher is `bun <project>/cli/jitso/index.ts`: a bare `bun`, because that is the word
// the dispatcher prompt and the dispatch guard expect, and the project copy, because the
// dispatch guard allows only the launcher next to itself.
func (t *TS) Launcher(sb *sandbox.Sandbox) string {
	return "bun " + filepath.Join(sb.Project, "cli", "jitso", "index.ts")
}

// HookCommand runs the sandbox copy of the hook, for the same reason as Command: the
// dispatch guard derives the one launcher it allows from its own location.
func (t *TS) HookCommand(sb *sandbox.Sandbox, hook string) ([]string, error) {
	script := filepath.Join(sb.Project, ".agents", "hooks", "jitso-"+hook+".mjs")
	if !sb.Inside(script) {
		return nil, fmt.Errorf("refusing to run %s: not inside the sandbox", script)
	}
	return []string{filepath.Join(sb.Tools, "node"), script}, nil
}
