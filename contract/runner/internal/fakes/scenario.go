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
	Cwd          *string                    `json:"cwd"`
	Screens      []string                   `json:"screens"`
	WaitTimesOut bool                       `json:"wait_times_out"`
	FailOn       []string                   `json:"fail_on"`
	Errors       map[string]json.RawMessage `json:"errors"`
	// Listed, when nil, is the default: the scenario handle until it is closed.
	Listed    []string                   `json:"listed"`
	Responses map[string]json.RawMessage `json:"responses"`
	// CloseKeepsListed keeps a closed handle in `terminal list`.
	CloseKeepsListed bool `json:"close_keeps_listed"`
	// Claude holds the fake claude knobs, from the case field `claude`.
	Claude ClaudeScenario `json:"claude"`
}

// ClaudeScenario is the case field `claude` (SPEC 5.2).
type ClaudeScenario struct {
	// Exit is the code the fake claude exits with, after it wrote its dump.
	Exit int `json:"exit"`
	// Missing leaves `claude` off PATH: the runner does not install the fake.
	Missing bool `json:"missing"`
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
	if err := jsonx.Keys(obj, "claude", "exit", "missing"); err != nil {
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
	if cs.Missing && cs.Exit != 0 {
		return cs, fmt.Errorf("claude: missing and exit exclude each other")
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
	if err := jsonx.Keys(obj, "orca", "handle", "cwd", "screens", "wait_times_out", "fail_on", "errors", "listed", "responses", "close_keeps_listed"); err != nil {
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
	if raw, ok := obj["close_keeps_listed"]; ok {
		if sc.CloseKeepsListed, err = jsonx.Bool(raw, "orca.close_keeps_listed"); err != nil {
			return sc, err
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
