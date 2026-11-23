package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"jitso-contract-runner/internal/fakes"
	"jitso-contract-runner/internal/sandbox"
)

// TestMain lets the test binary act as the fakes too, exactly like the runner binary does.
func TestMain(m *testing.M) {
	switch filepath.Base(os.Args[0]) {
	case "orca", "claude":
		main()
	}
	os.Exit(m.Run())
}

// TestMulticall links this binary into a sandbox under the fake names and calls them
// through PATH with the sandbox environment, the way an implementation does.
func TestMulticall(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sb, err := sandbox.New(self)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Remove()
	sc, _ := fakes.ParseScenario(nil, sb.Project)
	if err := sb.WriteScenario(sc); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(filepath.Join(sb.Bin, "orca"+exeSuffix()), "terminal", "create", "--title", "t", "--json")
	cmd.Env = sb.Env(nil)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("fake orca: %v", err)
	}
	var msg struct {
		Ok     bool `json:"ok"`
		Result struct {
			Terminal struct{ Handle, WorktreeID string } `json:"terminal"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &msg); err != nil || !msg.Ok || msg.Result.Terminal.Handle != "term_fake" {
		t.Fatalf("fake orca answer %q: %v", out, err)
	}
	log, _ := os.ReadFile(sb.OrcaLog)
	if strings.TrimSpace(string(log)) != `{"argv":["terminal","create","--title","t"]}` {
		t.Errorf("call log: %q", log)
	}

	cmd = exec.Command("/bin/sh", "-c", "claude --model m")
	cmd.Env = sb.Env(nil)
	cmd.Dir = sb.Home
	if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 {
		t.Fatalf("fake claude through PATH: %v, output %q", err, out)
	}
	var dump struct {
		Argv []string
		Cwd  string
	}
	data, _ := os.ReadFile(sb.ClaudeDump)
	if err := json.Unmarshal(data, &dump); err != nil || strings.Join(dump.Argv, " ") != "--model m" || dump.Cwd != sb.Home {
		t.Errorf("dump %s: %v", data, err)
	}
}

// exeSuffix is the executable suffix of the OS.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
