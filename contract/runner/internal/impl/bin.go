package impl

import (
	"fmt"
	"os"
	"path/filepath"

	"jitso-contract-runner/internal/sandbox"
)

// Bin is any executable: `<bin> <args...>` for the CLI, `<bin> hook <name>` for a hook.
// It is judged by the `go` status key, the binary this adapter exists for.
type Bin struct{ path string }

// NewBin checks that path is an executable file.
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
	if !st.Mode().IsRegular() || st.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("--bin: %s is not an executable file", abs)
	}
	return &Bin{path: abs}, nil
}

func (b *Bin) Name() string                   { return "bin" }
func (b *Bin) StatusKey() string              { return "go" }
func (b *Bin) Prepare(*sandbox.Sandbox) error { return nil }

// AroundStep is a no-op: the cases are written in the schema the binary speaks.
func (b *Bin) AroundStep(*sandbox.Sandbox) (func() error, error) {
	return func() error { return nil }, nil
}

func (b *Bin) Command(_ *sandbox.Sandbox, args []string) ([]string, error) {
	return append([]string{b.path}, args...), nil
}

// Launcher is the absolute path given in --bin.
func (b *Bin) Launcher(*sandbox.Sandbox) string { return b.path }

// HookCommand: the hook subcommand name is a proposal of the Go design (PLAN, "Open").
func (b *Bin) HookCommand(_ *sandbox.Sandbox, hook string) ([]string, error) {
	return []string{b.path, "hook", hook}, nil
}
