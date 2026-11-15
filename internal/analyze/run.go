package analyze

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/jsconv"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// Usage is the usage line of the command, in every rejection of its arguments.
const Usage = "usage: jitso analyze <launch_id> | jitso analyze --transcript <file.jsonl>"

// Deps is what Run needs from the outside: where the prices are, the registry,
// the catalog (read only for a launch's success checks) and the clock.
type Deps struct {
	// PricesPath is <primary checkout>/jitso/prices.json.
	PricesPath string
	Store      registry.Store
	// Catalog loads the project's catalog; it is called only by a by-id run.
	Catalog func() (*catalog.Catalog, error)
	Now     func() time.Time
}

// Run is `jitso analyze`. It returns the text for stdout: the one-line summary
// of `analyze <launch_id>` (which also appends events) or the indented JSON of
// `analyze --transcript <file.jsonl>` (which writes nothing).
//
// The prices load first, before the arguments are looked at: a missing prices
// file is exit 3 even for a malformed id (TS-ODDITIES R6 proposes to change it;
// it is still the oracle's order).
func Run(args []string, d Deps) (string, error) {
	prices, err := LoadPrices(d.PricesPath)
	if err != nil {
		return "", err
	}
	if len(args) > 0 && args[0] == "--transcript" {
		if len(args) != 2 || !strings.HasSuffix(args[1], ".jsonl") {
			return "", fail.Rejected(Usage)
		}
		a, err := Session(args[1], prices)
		if err != nil {
			return "", fail.Rejected(err.Error())
		}
		text, err := jsonx.MarshalIndent(a, " ")
		if err != nil {
			return "", err
		}
		return string(text), nil
	}
	if len(args) != 1 || !registry.IsLaunchID(args[0]) {
		return "", fail.Rejected(Usage)
	}
	return byID(args[0], prices, d)
}

// verdictOf is the verdict of an analysis against the build's success checks:
// fail when a tool call read a secret path, success when every check of the
// build is one that can be decided here (no_rule_breaks), otherwise unknown.
func verdictOf(secretReads int, checks []catalog.SuccessCheck) (verdict, reason string) {
	evaluated := 0
	for _, c := range checks {
		if c.Check == "no_rule_breaks" {
			evaluated++
		}
	}
	if secretReads > 0 {
		return "fail", "a tool call read a secret path"
	}
	verdict = "unknown"
	if len(checks) > 0 && evaluated == len(checks) {
		verdict = "success"
	}
	return verdict, fmt.Sprintf("build success checks %d of %d evaluated here; the rest need the destination", evaluated, len(checks))
}

func byID(id string, prices Prices, d Deps) (string, error) {
	launches, err := registry.ReadLaunches(d.Store)
	if err != nil {
		return "", err
	}
	l, ok := registry.Find(launches, id)
	if !ok {
		return "", fail.Rejectedf("unknown launch %s", id)
	}
	transcript, _ := l.Launch.String("transcript_path")
	if _, err := os.Stat(transcript); transcript == "" || err != nil {
		return "", fail.Rejectedf("launch %s has no transcript on disk yet", id)
	}
	a, err := Session(transcript, prices)
	if err != nil {
		return "", err
	}
	cat, err := d.Catalog()
	if err != nil {
		return "", err
	}
	var checks []catalog.SuccessCheck
	// The registry log keeps its v1 key "preset" for the build id (decision B3-5).
	if build, _ := l.Launch.String("preset"); build != "" {
		if b, ok := cat.Find(build); ok {
			checks = b.Success
		}
	}
	verdict, reason := verdictOf(a.SecretPathReads, checks)
	ts := jsonx.Time(d.Now())

	var budgetPct *float64
	if tokens, ok := l.Launch.Number("budget.tokens"); ok && tokens > 0 {
		pct := jsRound(a.MaxMainContextTokens/tokens*1000) / 10
		budgetPct = &pct
	}

	breaks := []registry.RuleBreakCount{}
	if a.SecretPathReads > 0 {
		evidence := fmt.Sprintf("%d tool call(s) touched a secret path", a.SecretPathReads)
		breaks = append(breaks, registry.RuleBreakCount{Kind: "secret_path_read", Count: int64(a.SecretPathReads)})
		if !hasRuleBreak(l, "secret_path_read", evidence) {
			rb := &registry.RuleBreakEvent{ID: &id, TS: ts, Kind: "secret_path_read", Evidence: evidence}
			if err := d.Store.Append(rb); err != nil {
				return "", err
			}
		}
	}
	err = d.Store.Append(&registry.AnalyzedEvent{
		ID:          id,
		TS:          ts,
		Analyzer:    Analyzer,
		Turns:       int64(a.MainRequests),
		Requests:    int64(a.Requests),
		PeakContext: int64(a.MaxMainContextTokens),
		BudgetPct:   budgetPct,
		Tokens: registry.TokenCounts{
			Input:      int64(a.Tokens.Input),
			CacheRead:  int64(a.Tokens.CacheRead),
			CacheWrite: int64(a.Tokens.CacheWrite),
			Output:     int64(a.Tokens.Output),
		},
		CostUSDList:   a.CostUSDList,
		PriceTable:    prices.TableLabel(),
		DurationMin:   a.DurationMin,
		SubagentFiles: int64(a.SubagentFiles),
		ByModel:       a.ByModel,
		ToolsUsed:     a.ToolUseCounts,
		RuleBreaks:    breaks,
		Verdict:       verdict,
		VerdictReason: reason,
	})
	if err != nil {
		return "", err
	}

	line := fmt.Sprintf("jitso: analyzed %s: %d turns, peak %s tokens, US$ %s list, verdict %s",
		id, a.MainRequests, jsconv.Number(a.MaxMainContextTokens), jsconv.Number(a.CostUSDList), verdict)
	if a.SecretPathReads > 0 {
		line += fmt.Sprintf(" (%d secret-path read(s))", a.SecretPathReads)
	}
	return line, nil
}

// hasRuleBreak is true when the launch already has this rule_break line, so a
// second analysis does not repeat it.
func hasRuleBreak(l *registry.Launch, kind, evidence string) bool {
	for _, e := range l.RuleBreaks {
		k, _ := e.String("kind")
		ev, _ := e.String("evidence")
		if k == kind && ev == evidence {
			return true
		}
	}
	return false
}
