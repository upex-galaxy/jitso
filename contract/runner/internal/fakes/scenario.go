// Package fakes holds the fake `orca` and fake `claude` (SPEC 5.1, 5.2). The runner binary
// links itself into the sandbox under those names and main dispatches on os.Args[0].
package fakes

import (
	"encoding/json"
	"fmt"

	"jitso-contract-runner/internal/jsonx"
)

// DefaultHandle is the handle `terminal create` returns unless the scenario names one.
const DefaultHandle = "term_fake"

// DefaultScreen is one idle Claude prompt screen.
const DefaultScreen = "❯ \n medium · /effort\n bypass permissions on"

// Scenario is the normalized fake orca scenario the runner writes to JITSO_CONTRACT_SCENARIO.
type Scenario struct {
	Handle string `json:"handle"`
	// Cwd is the folder in `worktreeId`; nil omits `worktreeId`.
	Cwd          *string  `json:"cwd"`
	Screens      []string `json:"screens"`
	WaitTimesOut bool     `json:"wait_times_out"`
	// WaitBlocked is one blockedReason per successive `terminal wait`; a wait past the end is idle.
	WaitBlocked []string                   `json:"wait_blocked"`
	FailOn      []string                   `json:"fail_on"`
	Errors      map[string]json.RawMessage `json:"errors"`
	// Listed, when nil, is the default: the scenario handle until it is closed.
	Listed    []string                   `json:"listed"`
	Responses map[string]json.RawMessage `json:"responses"`
	// CloseKeepsListed keeps a closed handle in `terminal list`.
	CloseKeepsListed bool `json:"close_keeps_listed"`
	// DelayMS is how long the fake waits before it answers a verb, after it
	// logged the call.
	DelayMS map[string]int `json:"delay_ms"`
	// Claude holds the fake claude knobs, from the case field `claude`.
	Claude ClaudeScenario `json:"claude"`
}

// ClaudeScenario is the case field `claude` (SPEC 5.2).
type ClaudeScenario struct {
	// Exit is the code the fake claude exits with, after it wrote its dump.
	Exit int `json:"exit"`
	// Missing leaves `claude` off PATH: the runner does not install the fake.
	Missing bool `json:"missing"`
	// MCPList is what `claude mcp list` prints on stdout, MCPListExit the code
	// it exits with.
	MCPList     string `json:"mcp_list"`
	MCPListExit int    `json:"mcp_list_exit"`
}

// ParseClaudeScenario reads the case field `claude` (nil = absent).
func ParseClaudeScenario(v any) (ClaudeScenario, error) {
	var cs ClaudeScenario
	if v == nil {
		return cs, nil
	}
	obj, err := jsonx.Object(v, "claude")
	if err != nil {
		return cs, err
	}
	if err := jsonx.Keys(obj, "claude", "exit", "missing", "mcp_list", "mcp_list_exit"); err != nil {
		return cs, err
	}
	if raw, ok := obj["exit"]; ok {
		if cs.Exit, err = jsonx.Int(raw, "claude.exit"); err != nil {
			return cs, err
		}
		if cs.Exit < 0 || cs.Exit > 255 {
			return cs, fmt.Errorf("claude.exit: want 0 to 255, got %d", cs.Exit)
		}
	}
	if raw, ok := obj["missing"]; ok {
		if cs.Missing, err = jsonx.Bool(raw, "claude.missing"); err != nil {
			return cs, err
		}
	}
	if raw, ok := obj["mcp_list"]; ok {
		if cs.MCPList, err = jsonx.String(raw, "claude.mcp_list"); err != nil {
			return cs, err
		}
	}
	if raw, ok := obj["mcp_list_exit"]; ok {
		if cs.MCPListExit, err = jsonx.Int(raw, "claude.mcp_list_exit"); err != nil {
			return cs, err
		}
		if cs.MCPListExit < 0 || cs.MCPListExit > 255 {
			return cs, fmt.Errorf("claude.mcp_list_exit: want 0 to 255, got %d", cs.MCPListExit)
		}
	}
	if cs.Missing && (cs.Exit != 0 || cs.MCPList != "" || cs.MCPListExit != 0) {
		return cs, fmt.Errorf("claude: missing excludes exit, mcp_list and mcp_list_exit")
	}
	return cs, nil
}

// ParseScenario reads the `orca` field of a case (nil = absent) and fills the SPEC defaults.
// project is the default for `cwd`.
func ParseScenario(v any, project string) (Scenario, error) {
	sc := Scenario{Handle: DefaultHandle, Cwd: &project, Screens: []string{DefaultScreen}}
	if v == nil {
		return sc, nil
	}
	obj, err := jsonx.Object(v, "orca")
	if err != nil {
		return sc, err
	}
	if err := jsonx.Keys(obj, "orca", "handle", "cwd", "screens", "wait_times_out", "wait_blocked", "fail_on", "errors", "listed", "responses", "close_keeps_listed", "delay_ms"); err != nil {
		return sc, err
	}
	if raw, ok := obj["handle"]; ok {
		if sc.Handle, err = jsonx.String(raw, "orca.handle"); err != nil {
			return sc, err
		}
	}
	if raw, ok := obj["cwd"]; ok {
		if raw == nil {
			sc.Cwd = nil
		} else {
			s, err := jsonx.String(raw, "orca.cwd")
			if err != nil {
				return sc, err
			}
			sc.Cwd = &s
		}
	}
	if raw, ok := obj["screens"]; ok {
		if sc.Screens, err = jsonx.Strings(raw, "orca.screens"); err != nil {
			return sc, err
		}
		if len(sc.Screens) == 0 {
			return sc, fmt.Errorf("orca.screens: give at least one screen")
		}
	}
	if raw, ok := obj["wait_times_out"]; ok {
		if sc.WaitTimesOut, err = jsonx.Bool(raw, "orca.wait_times_out"); err != nil {
			return sc, err
		}
	}
	if raw, ok := obj["wait_blocked"]; ok {
		if sc.WaitBlocked, err = jsonx.Strings(raw, "orca.wait_blocked"); err != nil {
			return sc, err
		}
	}
	if raw, ok := obj["close_keeps_listed"]; ok {
		if sc.CloseKeepsListed, err = jsonx.Bool(raw, "orca.close_keeps_listed"); err != nil {
			return sc, err
		}
	}
	if raw, ok := obj["delay_ms"]; ok {
		delays, err := jsonx.Object(raw, "orca.delay_ms")
		if err != nil {
			return sc, err
		}
		sc.DelayMS = map[string]int{}
		for verb, item := range delays {
			ms, err := jsonx.Int(item, "orca.delay_ms."+verb)
			if err != nil {
				return sc, err
			}
			if ms < 0 {
				return sc, fmt.Errorf("orca.delay_ms.%s: want 0 or more, got %d", verb, ms)
			}
			sc.DelayMS[verb] = ms
		}
	}
	if raw, ok := obj["fail_on"]; ok {
		if sc.FailOn, err = jsonx.Strings(raw, "orca.fail_on"); err != nil {
			return sc, err
		}
	}
	if raw, ok := obj["listed"]; ok {
		list, err := jsonx.Strings(raw, "orca.listed")
		if err != nil {
			return sc, err
		}
		sc.Listed = append([]string{}, list...)
	}
	if sc.Errors, err = rawObjects(obj["errors"], "orca.errors", true); err != nil {
		return sc, err
	}
	if sc.Responses, err = rawObjects(obj["responses"], "orca.responses", false); err != nil {
		return sc, err
	}
	return sc, nil
}

// rawObjects reads `{verb: value}`; with objectsOnly every value must be an object.
func rawObjects(v any, what string, objectsOnly bool) (map[string]json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	obj, err := jsonx.Object(v, what)
	if err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	for verb, item := range obj {
		if objectsOnly {
			if _, err := jsonx.Object(item, what+"."+verb); err != nil {
				return nil, err
			}
		}
		data, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		out[verb] = data
	}
	return out, nil
}
