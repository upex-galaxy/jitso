package card

import (
	"math"
	"testing"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want string
	}{
		{"direct, no lists", Input{ID: "L20261113T142233-a3f9", Role: "operator", Type: "explorer", Mode: "direct", Preset: "haiku-explorer",
			PresetVersion: "1", Model: "claude-haiku-5-5", Effort: "low", RouteScope: []string{"none"}, LaunchCommand: CommandPlain,
			BudgetTokens: 250000, BudgetUSD: 0.5},
			"JITSO L20261113T142233-a3f9 | operator/explorer | mode direct (asks you) | preset haiku-explorer v1\n" +
				"model claude-haiku-5-5 effort low | bypass | MCP none | skill none\n" +
				"subagents unrestricted (harness default)\n" +
				"ROUTE-SCOPE: none\n" +
				"launch: jitso launch\n" +
				"budget 250k tokens (100%), list cap US$ 0.50 | reports to: you | log .jitso/launches.jsonl"},
		{"fleet, lists", Input{ID: "L1", Role: "operator", Type: "automator", Mode: "fleet", Preset: "sonnet-automator", PresetVersion: "2",
			Model: "m", Effort: "medium", MCP: []string{"dbhub", "openapi"}, SkillsFirst: []string{"a", "b"}, Subagents: []string{"plan", "code"},
			RouteScope: []string{"code-quickref", "ticket-work"}, LaunchCommand: CommandBunRun, BudgetTokens: 400000, BudgetUSD: 6},
			"JITSO L1 | operator/automator | mode fleet (asks your conductor) | preset sonnet-automator v2\n" +
				"model m effort medium | bypass | MCP dbhub,openapi | skill a,b\n" +
				"subagents plan,code (suggested; others allowed)\n" +
				"ROUTE-SCOPE: code-quickref,ticket-work\n" +
				"launch: bun run jitso launch\n" +
				"budget 400k tokens (100%), list cap US$ 6.00 | reports to: your conductor | log .jitso/launches.jsonl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Render(tt.in); got != tt.want {
				t.Errorf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestScopeLine(t *testing.T) {
	for _, tt := range []struct {
		ids  []string
		want string
	}{
		{nil, "ROUTE-SCOPE: none"},
		{[]string{"none"}, "ROUTE-SCOPE: none"},
		{[]string{"a", "b"}, "ROUTE-SCOPE: a,b"},
	} {
		if got := ScopeLine(tt.ids); got != tt.want {
			t.Errorf("ScopeLine(%q) = %q, want %q", tt.ids, got, tt.want)
		}
	}
}

func TestLaunchCommand(t *testing.T) {
	for _, tt := range []struct {
		pkg  string
		want string
	}{
		{`{"scripts": {"jitso": "bun cli/jitso/index.ts"}}`, CommandBunRun},
		{`{"scripts": {"jitso": ""}}`, CommandBunRun},
		{`{"scripts": {}}`, CommandPlain},
		{`{"scripts": {"jitso": 5}}`, CommandPlain},
		{`{"scripts": null}`, CommandPlain},
		{`[]`, CommandPlain},
		{`null`, CommandPlain},
		{`{not json`, CommandPlain},
		{``, CommandPlain},
	} {
		if got := LaunchCommand([]byte(tt.pkg)); got != tt.want {
			t.Errorf("LaunchCommand(%q) = %q, want %q", tt.pkg, got, tt.want)
		}
	}
}

func TestJSRound(t *testing.T) {
	for in, want := range map[float64]float64{250: 250, 0.5: 1, 1.4999: 1, 2.5: 3, -2.5: -2, -2.6: -3, 0: 0} {
		if got := jsRound(in); got != want {
			t.Errorf("jsRound(%v) = %v, want %v", in, got, want)
		}
	}
	if !math.IsNaN(jsRound(math.NaN())) {
		t.Error("NaN must stay NaN")
	}
}

func TestToFixed2(t *testing.T) {
	for in, want := range map[float64]string{0.5: "0.50", 6: "6.00", 15: "15.00", 0.125: "0.13", 0.375: "0.38", -0.125: "-0.13",
		1.005: "1.00", 2.675: "2.67", 0.999: "1.00", -0.001: "-0.00", math.NaN(): "NaN"} {
		if got := toFixed2(in); got != want {
			t.Errorf("toFixed2(%v) = %q, want %q", in, got, want)
		}
	}
}
