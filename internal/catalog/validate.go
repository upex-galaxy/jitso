package catalog

import (
	"regexp"
	"slices"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// Status values of a saved build. A draft (the draft layer, and only it) has
// the one status StatusDraft.
var Statuses = []string{"candidate", "approved", "retired"}

// StatusDraft is the status of every file of the draft layer.
const StatusDraft = "draft"

// The tiers of a build (docs/decisions.md, tiers). A build without a tier is a
// worker. A subagent-tier build is never launched alone: a worker or a
// conductor lists it by name in its subagents.
const (
	TierWorker    = "worker"
	TierConductor = "conductor"
	TierSubagent  = "subagent"
)

// Tiers lists the valid tier values.
var Tiers = []string{TierWorker, TierConductor, TierSubagent}

// SuccessChecks are the known success check names.
var SuccessChecks = []string{"answer_delivered", "jira_comment_on", "xray_execution_with_env", "no_rule_breaks"}

// fieldPatterns are the patterns of the fields a build may require. A field
// not listed here cannot be required.
var fieldPatterns = map[string]*regexp.Regexp{
	"ticket_key": regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9]+$`),
	"env":        regexp.MustCompile(`^[a-z][a-z0-9_-]*$`),
}

// FieldPattern is the pattern a required field's value must match, and false
// for a field no build may require.
func FieldPattern(name string) (*regexp.Regexp, bool) {
	p, ok := fieldPatterns[name]
	return p, ok
}

var (
	kebab       = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	placeholder = regexp.MustCompile(`\{([a-z_]+)\}`)
)

// ValidateBuild checks one decoded build file of a saved layer (the project's
// own, a parent's or the user's) against the allowlists and the route ids. See
// ValidateBuildIn.
func ValidateBuild(raw any, cfg Config, routeIDs []string) []string {
	return ValidateBuildIn(raw, cfg, routeIDs, LayerRepo)
}

// ValidateBuildIn checks one decoded build file of the given layer against the
// allowlists and the route ids. It returns every problem, in the oracle's order
// (its validatePreset), followed by the tier rules; an empty list means valid.
// The messages and the String() coercion of values are the oracle's.
//
// The layer decides one rule, the status: a saved layer takes candidate,
// approved or retired, the draft layer takes exactly draft (defect N-15), so a
// draft cannot pass as a saved build and a saved file cannot claim to be a draft.
//
// Worker and conductor builds are validated identically. A subagent-tier build
// never launches alone, so route_scope and success are optional for it (checked
// only when present, never used), and skills_first, mcp, requires and
// subagents must be empty (docs/decisions.md, tier matrix, defect N-11).
func ValidateBuildIn(raw any, cfg Config, routeIDs []string, layer Layer) []string {
	p, ok := raw.(*Map)
	if !ok {
		return []string{"build is not a mapping"}
	}
	var errs []string
	add := func(msg string) { errs = append(errs, msg) }
	subagent := tierOf(p) == TierSubagent

	if id, ok := p.Value("id").(string); !ok || !kebab.MatchString(id) {
		add("id must be kebab-case")
	}
	status, isString := p.Value("status").(string)
	switch {
	case layer == LayerDraft:
		if !isString || status != StatusDraft {
			add("status must be draft in a draft")
		}
	case !isString || !slices.Contains(Statuses, status):
		add("status must be one of " + strings.Join(Statuses, ", "))
	}
	if model := jsconv.String(p.Value("model")); !slices.Contains(cfg.Models, model) {
		add(`model "` + model + `" is not in the allowlist`)
	}
	if effort := jsconv.String(p.Value("effort")); !slices.Contains(cfg.Efforts, effort) {
		add(`effort "` + effort + `" is not in the allowlist`)
	}
	for _, skill := range jsconv.Strings(p.Value("skills_first")) {
		if !slices.Contains(cfg.Skills, skill) {
			add(`skill "` + skill + `" is not in the allowlist`)
		}
	}
	for _, name := range jsconv.Strings(p.Value("mcp")) {
		if !slices.Contains(cfg.MCP, name) {
			add(`mcp "` + name + `" is not in the allowlist`)
		}
	}
	for _, tool := range jsconv.Strings(p.Value("tools")) {
		if !slices.Contains(cfg.Tools, tool) {
			add(`tool "` + tool + `" is not in the allowlist`)
		}
	}
	if v, present := p.Get("subagents"); present && !kebabList(v) {
		add("subagents must be a list of kebab-case names")
	}
	if _, present := p.Get("route_scope"); present || !subagent {
		scope := jsconv.Strings(p.Value("route_scope"))
		if len(scope) == 0 {
			add("route_scope is empty (use [none])")
		}
		for _, id := range scope {
			if id != "none" && !slices.Contains(routeIDs, id) {
				add(`route_scope id "` + id + `" is not an instruction section`)
			}
		}
	}
	requires := jsconv.Strings(p.Value("requires"))
	for _, field := range requires {
		if _, ok := fieldPatterns[field]; !ok {
			add(`required field "` + field + `" has no pattern`)
		}
	}
	success, isList := p.Value("success").([]any)
	if _, present := p.Get("success"); (present || !subagent) && (!isList || len(success) == 0) {
		add("success must list at least one check")
	}
	for _, entry := range success {
		check := propertyOf(entry, "check")
		if s, ok := check.(string); !ok || !slices.Contains(SuccessChecks, s) {
			add(`success check "` + jsconv.String(check) + `" is unknown`)
		}
		for _, value := range valuesOf(entry) {
			for _, m := range placeholder.FindAllStringSubmatch(jsconv.String(value), -1) {
				if !slices.Contains(requires, m[1]) {
					add("success placeholder {" + m[1] + "} is not a required field")
				}
			}
		}
	}
	return append(errs, tierProblems(p)...)
}

// tierOf is the tier a build file declares: worker when the key is absent or
// not a known tier (tierProblems reports the latter).
func tierOf(p *Map) string {
	if s, ok := p.Value("tier").(string); ok && slices.Contains(Tiers, s) {
		return s
	}
	return TierWorker
}

// tierProblems are the rules of the tier field, which the oracle does not
// know: the value, what a subagent-tier build must carry to reach claude as an
// --agents entry (a description and a prompt, defect S3), and what it must not
// carry (skills_first, mcp, requires and subagents must be empty, defect N-11).
func tierProblems(p *Map) []string {
	if v, present := p.Get("tier"); present {
		if s, ok := v.(string); !ok || !slices.Contains(Tiers, s) {
			return []string{"tier must be one of " + strings.Join(Tiers, ", ")}
		}
	}
	if tierOf(p) != TierSubagent {
		return nil
	}
	var errs []string
	for _, key := range []string{"description", "prompt"} {
		if s, ok := p.Value(key).(string); !ok || strings.TrimSpace(s) == "" {
			errs = append(errs, key+" must be a non-empty string in a subagent-tier build")
		}
	}
	for _, key := range []string{"skills_first", "mcp", "requires"} {
		if len(jsconv.Strings(p.Value(key))) > 0 {
			errs = append(errs, key+" must be empty in a subagent-tier build")
		}
	}
	if list, ok := p.Value("subagents").([]any); ok && len(list) > 0 {
		errs = append(errs, "a subagent-tier build cannot list subagents")
	}
	return errs
}

func kebabList(v any) bool {
	list, ok := v.([]any)
	if !ok {
		return false
	}
	for _, item := range list {
		s, ok := item.(string)
		if !ok || !kebab.MatchString(s) {
			return false
		}
	}
	return true
}

// propertyOf is `entry.check` for any decoded value: a mapping's value, else
// undefined (the oracle crashes on a null entry; here it is undefined too).
func propertyOf(entry any, key string) any {
	if m, ok := entry.(*Map); ok {
		return m.Value(key)
	}
	return jsconv.Undefined
}

// valuesOf is Object.values(entry): a mapping's values in order, a list's
// elements, a string's characters, nothing for any other value.
func valuesOf(entry any) []any {
	switch x := entry.(type) {
	case *Map:
		return x.Values()
	case []any:
		return x
	case string:
		out := make([]any, 0, len(x))
		for _, r := range x {
			out = append(out, string(r))
		}
		return out
	}
	return nil
}
