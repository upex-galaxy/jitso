package fakes

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"jitso-contract-runner/internal/expect"
)

// Environment variables the fakes read (SPEC 5).
const (
	EnvScenario   = "JITSO_CONTRACT_SCENARIO"
	EnvOrcaLog    = "JITSO_CONTRACT_ORCA_LOG"
	EnvClaudeDump = "JITSO_CONTRACT_CLAUDE_DUMP"
)

// Orca is the fake `orca`. It keeps no state of its own: the screen index and the closed
// handles are recomputed from the call log, so every call is one process with one answer.
func Orca(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if n := len(args); n > 0 && args[n-1] == "--json" {
		args = args[:n-1]
	}
	sc, err := readScenario(getenv(EnvScenario))
	if err != nil {
		return answer(stdout, false, map[string]string{"code": "fake_scenario", "message": err.Error()})
	}
	logPath := getenv(EnvOrcaLog)
	previous, err := expect.OrcaCalls(logPath)
	if err != nil {
		fmt.Fprintln(stderr, "fake orca:", err)
		return answer(stdout, false, map[string]string{"code": "fake_log", "message": err.Error()})
	}
	if err := appendCall(logPath, args); err != nil {
		fmt.Fprintln(stderr, "fake orca:", err)
		return answer(stdout, false, map[string]string{"code": "fake_log", "message": err.Error()})
	}
	ok, body := respond(sc, args, previous)
	return answer(stdout, ok, body)
}

// respond computes the answer to one call given the calls before it.
func respond(sc Scenario, args []string, previous [][]string) (bool, any) {
	verb := expect.Verb(args)
	if body, ok := sc.Errors[verb]; ok {
		return false, body
	}
	for _, v := range sc.FailOn {
		if v == verb {
			return false, failure(verb)
		}
	}
	if verb == "terminal wait" && sc.WaitTimesOut {
		return false, failure(verb)
	}
	if result, ok := sc.Responses[verb]; ok {
		return true, result
	}
	switch verb {
	case "terminal create":
		terminal := map[string]string{"handle": sc.Handle}
		if sc.Cwd != nil {
			terminal["worktreeId"] = "fake-uuid::" + *sc.Cwd
		}
		return true, map[string]any{"terminal": terminal}
	case "terminal wait", "terminal close":
		return true, map[string]any{}
	case "terminal read":
		reads := 0
		for _, c := range previous {
			if expect.Verb(c) == "terminal read" {
				reads++
			}
		}
		screen := sc.Screens[min(reads, len(sc.Screens)-1)]
		return true, map[string]any{"terminal": map[string]any{"tail": strings.Split(screen, "\n")}}
	case "terminal list":
		return true, map[string]any{"terminals": listed(sc, previous)}
	}
	return true, map[string]any{"state": "ready"}
}

// listed is the scenario list (default: the scenario handle) minus every handle a previous
// `terminal close --terminal <h>` closed.
func listed(sc Scenario, previous [][]string) []map[string]string {
	handles := sc.Listed
	if handles == nil {
		handles = []string{sc.Handle}
	}
	closed := map[string]bool{}
	for _, c := range previous {
		if sc.CloseKeepsListed {
			break
		}
		if expect.Verb(c) != "terminal close" {
			continue
		}
		for i := 2; i+1 < len(c); i++ {
			if c[i] == "--terminal" {
				closed[c[i+1]] = true
			}
		}
	}
	out := []map[string]string{}
	for _, h := range handles {
		if !closed[h] {
			out = append(out, map[string]string{"handle": h})
		}
	}
	return out
}

func failure(verb string) map[string]string {
	return map[string]string{"code": "fake_failure", "message": verb + " failed"}
}

func answer(stdout io.Writer, ok bool, body any) int {
	msg := map[string]any{"ok": ok}
	if ok {
		msg["result"] = body
	} else {
		msg["error"] = body
	}
	data, _ := json.Marshal(msg)
	fmt.Fprintln(stdout, string(data))
	if ok {
		return 0
	}
	return 1
}

func readScenario(path string) (Scenario, error) {
	if path == "" {
		return Scenario{}, fmt.Errorf("%s is not set", EnvScenario)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, err
	}
	var sc Scenario
	if err := json.Unmarshal(data, &sc); err != nil {
		return Scenario{}, fmt.Errorf("scenario %s: %v", path, err)
	}
	if sc.Handle == "" {
		sc.Handle = DefaultHandle
	}
	if len(sc.Screens) == 0 {
		sc.Screens = []string{DefaultScreen}
	}
	return sc, nil
}

func appendCall(path string, args []string) error {
	if path == "" {
		return fmt.Errorf("%s is not set", EnvOrcaLog)
	}
	if args == nil {
		args = []string{}
	}
	line, err := json.Marshal(map[string]any{"argv": args})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
