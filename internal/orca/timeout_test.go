package orca

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// sleeper is the variable that turns this test binary into an orca that answers
// after the duration it holds, so the limit is tested on every OS.
const sleeper = "JITSO_TEST_ORCA_SLEEP"

func TestMain(m *testing.M) {
	if wait, err := time.ParseDuration(os.Getenv(sleeper)); err == nil {
		time.Sleep(wait)
		fmt.Print(`{"ok":true,"result":{"terminals":[]}}`)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestExecRunnerTimeout(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sleeper, "0s")
	if res := ExecRunnerTimeout(self, time.Minute)([]string{"terminal", "list"}); res.Status != 0 {
		t.Fatalf("an answer in time: %+v", res)
	}
	t.Setenv(sleeper, "1m")
	c := NewClient(ExecRunnerTimeout(self, 300*time.Millisecond))
	calls := map[string]func() error{
		"orca terminal list did not answer within 0.3 s":       func() error { _, err := c.ListTerminals(); return err },
		"orca orchestration inbox did not answer within 0.3 s": func() error { _, err := c.Messages(1); return err },
		"orca terminal switch did not answer within 0.3 s":     func() error { return c.SwitchTerminal("term_a") },
	}
	for want, call := range calls {
		start := time.Now()
		err := call()
		var failed *Error
		if !errors.As(err, &failed) || failed.Code != CodeTimeout || failed.Message != want {
			t.Errorf("got %v, want %q", err, want)
		}
		if took := time.Since(start); took > 10*time.Second {
			t.Errorf("%s took %v", want, took)
		}
	}
}

// A program that leaves a child holding its output is given up all the same.
func TestExecRunnerTimeoutWithAChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
	prog := filepath.Join(t.TempDir(), "orca")
	if err := os.WriteFile(prog, []byte("#!/bin/sh\nsleep 20\necho late\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	res := ExecRunnerTimeout(prog, 300*time.Millisecond)([]string{"terminal", "list"})
	if res.Status != StatusTimeout || res.Stdout != "" {
		t.Errorf("result %+v", res)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("took %v", took)
	}
}
