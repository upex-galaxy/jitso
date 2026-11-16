package tree

import (
	"reflect"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/registry"
)

type spec struct {
	id, parent string // parent "" is null
	cost       float64
	tokens     [4]float64
	peak, pct  float64
	verdict    string // "" means no analyzed event
	breaks     int
	ended      bool
	noBuild    bool
}

func launches(specs ...spec) []registry.Launch {
	var out []registry.Launch
	for _, s := range specs {
		l := registry.Launch{ID: s.id, Launch: registry.Event{"event": "launch", "id": s.id, "parent": nil}}
		if s.parent != "" {
			l.Launch["parent"] = s.parent
		}
		if !s.noBuild {
			l.Launch["preset"] = "sonnet-tester"
		}
		if s.verdict != "" {
			l.Analyzed = registry.Event{
				"event": "analyzed", "id": s.id, "cost_usd_list": s.cost, "peak_context": s.peak, "verdict": s.verdict,
				"tokens": map[string]any{"input": s.tokens[0], "cache_read": s.tokens[1], "cache_write": s.tokens[2], "output": s.tokens[3]},
			}
			if s.pct != 0 {
				l.Analyzed["budget_pct"] = s.pct
			}
		}
		for i := 0; i < s.breaks; i++ {
			l.RuleBreaks = append(l.RuleBreaks, registry.Event{"event": "rule_break", "id": s.id, "kind": "budget_over"})
		}
		if s.ended {
			l.End = registry.Event{"event": "end", "id": s.id}
		}
		out = append(out, l)
	}
	return out
}

// fleet is the table of the oracle's tree.test.ts: R with children A and B,
// and C under A.
func fleet() []registry.Launch {
	return launches(
		spec{id: "R", cost: 1, peak: 100, pct: 40, tokens: [4]float64{1, 10, 100, 1000}, verdict: "success"},
		spec{id: "A", parent: "R", cost: 2, peak: 300, pct: 120, tokens: [4]float64{2, 20, 200, 2000}, verdict: "unknown", breaks: 1},
		spec{id: "B", parent: "R", cost: 4, peak: 50, pct: 20, tokens: [4]float64{4, 40, 400, 4000}, verdict: "fail", breaks: 2, ended: true},
		spec{id: "C", parent: "A", cost: 0.5, peak: 80, pct: 32, tokens: [4]float64{1, 1, 1, 1}, verdict: "success"},
	)
}

func ids(nodes []*Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.ID)
	}
	return out
}

func TestBuildNestsByParent(t *testing.T) {
	roots := Build(fleet(), "R")
	if len(roots) != 1 || roots[0].ID != "R" || roots[0].Depth != 0 {
		t.Fatalf("roots %v", ids(roots))
	}
	root := roots[0]
	if got := ids(root.Children); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Errorf("children of R %v", got)
	}
	if got := ids(root.Children[0].Children); !reflect.DeepEqual(got, []string{"C"}) || root.Children[0].Children[0].Depth != 2 {
		t.Errorf("children of A %v", got)
	}
}

func TestRollups(t *testing.T) {
	root := Build(fleet(), "R")[0]
	a := root.Children[0]
	tests := []struct {
		name string
		got  Metrics
		want Metrics
	}{
		{"root", root.Total, Metrics{CostUSDList: 7.5, Tokens: 7781, PeakContext: 300, BudgetPct: 120, RuleBreaks: 3, Verdict: "fail", Nodes: 4, Analyzed: 4}},
		{"A and C", a.Total, Metrics{CostUSDList: 2.5, Tokens: 2226, PeakContext: 300, BudgetPct: 120, RuleBreaks: 1, Verdict: "unknown", Nodes: 2, Analyzed: 2}},
		{"B alone", root.Children[1].Total, Metrics{CostUSDList: 4, Tokens: 4444, PeakContext: 50, BudgetPct: 20, RuleBreaks: 2, Verdict: "fail", Nodes: 1, Analyzed: 1}},
		{"C alone", a.Children[0].Total, Metrics{CostUSDList: 0.5, Tokens: 4, PeakContext: 80, BudgetPct: 32, Verdict: "success", Nodes: 1, Analyzed: 1}},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, tt.got, tt.want)
		}
	}
}

func TestPendingNodeAddsNothing(t *testing.T) {
	l := launches(spec{id: "R", cost: 1, peak: 100, verdict: "success"}, spec{id: "A", parent: "R"})
	root := Build(l, "R")[0]
	if root.Total.CostUSDList != 1 || root.Total.Nodes != 2 || root.Total.Analyzed != 1 || root.Total.Verdict != "success" {
		t.Errorf("root %+v", root.Total)
	}
	if a := root.Children[0].Total; a.Verdict != "" || a.PeakContext != 0 || a.Analyzed != 0 {
		t.Errorf("a pending node %+v", a)
	}
}

func TestRootsOrphansAndCycles(t *testing.T) {
	tests := []struct {
		name string
		l    []registry.Launch
		root string
		want []string
	}{
		{"every root, orphans included, in registry order",
			launches(spec{id: "R"}, spec{id: "A", parent: "R"}, spec{id: "S"}, spec{id: "Z", parent: "GONE"}), "", []string{"R", "S", "Z"}},
		{"an unknown root is nothing", fleet(), "NOPE", nil},
		{"a root id gives its branch only", fleet(), "A", []string{"A"}},
		{"a parent cycle prints its first launch as a root", launches(spec{id: "P", parent: "Q"}, spec{id: "Q", parent: "P"}), "", []string{"P"}},
		{"a cycle next to a normal tree", launches(spec{id: "R"}, spec{id: "P", parent: "Q"}, spec{id: "Q", parent: "P"}, spec{id: "X", parent: "Q"}), "", []string{"R", "P"}},
		{"a launch that is its own parent", launches(spec{id: "P", parent: "P"}), "", []string{"P"}},
		{"nothing", nil, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ids(Build(tt.l, tt.root)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestACycleNeverRepeatsANode(t *testing.T) {
	l := launches(spec{id: "P", parent: "Q"}, spec{id: "Q", parent: "P"}, spec{id: "X", parent: "Q"})
	roots := Build(l, "")
	seen := map[string]int{}
	var walk func(n *Node)
	walk = func(n *Node) {
		seen[n.ID]++
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	if !reflect.DeepEqual(seen, map[string]int{"P": 1, "Q": 1, "X": 1}) {
		t.Errorf("printed %v", seen)
	}
}

func TestLines(t *testing.T) {
	want := []string{
		"R sonnet-tester running: verdict fail, US$ 7.5 list, 7781 tokens, peak 300, budget 120%, 3 rule break(s), 4 of 4 analyzed",
		"  A sonnet-tester running: verdict unknown, US$ 2.5 list, 2226 tokens, peak 300, budget 120%, 1 rule break(s), 2 of 2 analyzed",
		"    C sonnet-tester running: verdict success, US$ 0.5 list, 4 tokens, peak 80, budget 32%, 0 rule break(s), 1 of 1 analyzed",
		"  B sonnet-tester ended: verdict fail, US$ 4 list, 4444 tokens, peak 50, budget 20%, 2 rule break(s), 1 of 1 analyzed",
	}
	if got := Lines(Build(fleet(), "")); !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLineDetails(t *testing.T) {
	tests := []struct {
		name string
		l    []registry.Launch
		want string
	}{
		{"pending", launches(spec{id: "R"}), "R sonnet-tester running: verdict pending, US$ 0 list, 0 tokens, peak 0, budget 0%, 0 rule break(s), 0 of 1 analyzed"},
		{"a build that is not a string", launches(spec{id: "R", noBuild: true}), "R - running: verdict pending, US$ 0 list, 0 tokens, peak 0, budget 0%, 0 rule break(s), 0 of 1 analyzed"},
		{"cost is rounded to three decimals", launches(spec{id: "R", cost: 0.1, verdict: "success"}, spec{id: "S", parent: "R", cost: 0.2, verdict: "success"}),
			"R sonnet-tester running: verdict success, US$ 0.3 list, 0 tokens, peak 0, budget 0%, 0 rule break(s), 2 of 2 analyzed"},
		{"a fractional budget", launches(spec{id: "R", pct: 77.6, verdict: "unknown"}),
			"R sonnet-tester running: verdict unknown, US$ 0 list, 0 tokens, peak 0, budget 77.6%, 0 rule break(s), 1 of 1 analyzed"},
		{"an unknown verdict is analyzed but pending", launches(spec{id: "R", verdict: "odd"}),
			"R sonnet-tester running: verdict pending, US$ 0 list, 0 tokens, peak 0, budget 0%, 0 rule break(s), 1 of 1 analyzed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Lines(Build(tt.l, ""))
			if len(got) == 0 || got[0] != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	l := launches(spec{id: "R", noBuild: true})
	l[0].Launch["preset"] = 5.0
	if got := Lines(Build(l, "")); !strings.HasPrefix(got[0], "R - running") {
		t.Errorf("a numeric build key prints -: %q", got)
	}
}

func TestWorstVerdict(t *testing.T) {
	tests := []struct{ a, b, want string }{
		{"", "", ""}, {"", "fail", "fail"}, {"success", "", "success"}, {"success", "unknown", "unknown"},
		{"unknown", "success", "unknown"}, {"unknown", "fail", "fail"}, {"fail", "success", "fail"}, {"success", "success", "success"},
	}
	for _, tt := range tests {
		if got := worst(tt.a, tt.b); got != tt.want {
			t.Errorf("worst(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}
