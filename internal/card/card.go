// Package card renders the launch card: six lines the launcher prints, saves
// as card.txt and puts at the top of the brief the session reads. It is pure.
//
// The fourth line is the ROUTE-SCOPE line (defect S1): it starts a line of its
// own with "ROUTE-SCOPE:", the form the instruction router reads.
package card

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// Input is what the card shows. Text fields are printed as given.
type Input struct {
	ID   string
	Role string
	Type string
	// Mode is direct or fleet.
	Mode          string
	Preset        string
	PresetVersion string
	Model         string
	Effort        string
	MCP           []string
	SkillsFirst   []string
	Subagents     []string
	RouteScope    []string
	// LaunchCommand starts another session in the project (LaunchCommand).
	LaunchCommand string
	// BudgetTokens and BudgetUSD are the build's budget.
	BudgetTokens float64
	BudgetUSD    float64
}

// Render is the card text, six lines without a trailing newline:
//
//	JITSO <id> | <role>/<type> | mode <mode> (<who it asks>) | preset <preset> v<version>
//	model <model> effort <effort> | bypass | MCP <list> | skill <list>
//	subagents <names or unrestricted>
//	ROUTE-SCOPE: <ids>
//	launch: <command>
//	budget <n>k tokens (100%), list cap US$ <x.xx> | reports to: <whom> | log .jitso/launches.jsonl
func Render(in Input) string {
	asks, reports := "asks your conductor", "your conductor"
	if in.Mode == "direct" {
		asks, reports = "asks you", "you"
	}
	return strings.Join([]string{
		"JITSO " + in.ID + " | " + in.Role + "/" + in.Type + " | mode " + in.Mode + " (" + asks + ") | preset " + in.Preset + " v" + in.PresetVersion,
		"model " + in.Model + " effort " + in.Effort + " | bypass | MCP " + list(in.MCP) + " | skill " + list(in.SkillsFirst),
		"subagents " + subagents(in.Subagents),
		ScopeLine(in.RouteScope),
		"launch: " + in.LaunchCommand,
		"budget " + jsconv.Number(jsRound(in.BudgetTokens/1000)) + "k tokens (100%), list cap US$ " + toFixed2(in.BudgetUSD) +
			" | reports to: " + reports + " | log .jitso/launches.jsonl",
	}, "\n")
}

// ScopeLine is the route scope line the router reads:
// "ROUTE-SCOPE: <ids joined by ,>", or "ROUTE-SCOPE: none" for no ids. The
// card and the first prompt of a direct launch use it.
func ScopeLine(ids []string) string {
	return "ROUTE-SCOPE: " + list(ids)
}

// The launch commands the card can name.
const (
	CommandBunRun = "bun run jitso launch"
	CommandPlain  = "jitso launch"
)

// LaunchCommand is the command that starts another session in the project,
// from the content of its package.json: `bun run jitso launch` when the file
// declares a string `jitso` script (any string, even empty), else
// `jitso launch`. A missing or unusable file is the plain command.
func LaunchCommand(packageJSON []byte) string {
	var pkg any
	if json.Unmarshal(packageJSON, &pkg) != nil {
		return CommandPlain
	}
	root, _ := pkg.(map[string]any)
	scripts, _ := root["scripts"].(map[string]any)
	if _, ok := scripts["jitso"].(string); ok {
		return CommandBunRun
	}
	return CommandPlain
}

func list(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ",")
}

// subagents is "unrestricted (harness default)" for none; a listed set is a
// suggestion, never a limit.
func subagents(names []string) string {
	if len(names) == 0 {
		return "unrestricted (harness default)"
	}
	return strings.Join(names, ",") + " (suggested; others allowed)"
}

// jsRound is JavaScript Math.round: the nearest integer, halves toward +Inf.
func jsRound(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	r := math.Floor(x)
	if x-r >= 0.5 {
		r++
	}
	return r
}

// toFixed2 is JavaScript Number.prototype.toFixed(2). It differs from
// strconv's 'f' rounding only on an exact tie (0.125 is "0.13" in
// JavaScript, which takes the larger candidate, and "0.12" in Go).
func toFixed2(x float64) string {
	if math.IsNaN(x) {
		return "NaN"
	}
	if math.IsInf(x, 0) {
		return jsconv.Number(x)
	}
	if abs := math.Abs(x); abs*8 == math.Trunc(abs*8) && abs*4 != math.Trunc(abs*4) {
		// An exact tie at the third decimal: round the magnitude up.
		s := strconv.FormatFloat(abs+0.001, 'f', 2, 64)
		if x < 0 {
			return "-" + s
		}
		return s
	}
	return strconv.FormatFloat(x, 'f', 2, 64)
}
