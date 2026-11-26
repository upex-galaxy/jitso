package impl

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"jitso-contract-runner/internal/sandbox"
)

// Bin is any executable: `<bin> <args...>` for the CLI, `<bin> hook <name>` for a hook.
type Bin struct{ path string }

// NewBin checks that path is an executable file and keeps its real absolute path: the
// binary under test writes its own path resolved, and the cases compare against it.
func NewBin(path string) (*Bin, error) {
	if path == "" {
		return nil, fmt.Errorf("--impl bin needs --bin <path>")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("--bin: %v", err)
	}
	if !st.Mode().IsRegular() || (runtime.GOOS != "windows" && st.Mode()&0o111 == 0) {
		return nil, fmt.Errorf("--bin: %s is not an executable file", abs)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("--bin: %v", err)
	}
	return &Bin{path: resolved}, nil
}

func (b *Bin) Name() string                   { return "bin" }
func (b *Bin) Prepare(*sandbox.Sandbox) error { return nil }

func (b *Bin) Command(_ *sandbox.Sandbox, args []string) ([]string, error) {
	return append([]string{b.path}, args...), nil
}

// Launcher is the real absolute path of --bin.
func (b *Bin) Launcher(*sandbox.Sandbox) string { return b.path }

// HookCommand: the hook subcommand name is a proposal of the Go design (PLAN, "Open").
func (b *Bin) HookCommand(_ *sandbox.Sandbox, hook string) ([]string, error) {
	return []string{b.path, "hook", hook}, nil
}
