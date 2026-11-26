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

// DefaultHarness is the harness of a build that has no harness key.
const DefaultHarness = "claude"

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

// FieldName is the shape of a field name: the keys of the fields mapping of
// jitso/jitso.yaml and each name a build lists in requires.
var FieldName = regexp.MustCompile(`^[a-z][a-z_]*$`)

var (
	kebab = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// ValidateBuild checks one decoded build file of a saved layer (the project's
// own) against the allowlists and the route ids. See ValidateBuildIn.
func ValidateBuild(raw any, cfg Config, routeIDs []string) []string {
	return ValidateBuildIn(raw, cfg, routeIDs, LayerRepo)
}

// ValidateBuildIn checks one decoded build file of the given layer against the
// allowlists and the route ids. It returns every problem, in rule order,
// followed by the tier rules; an empty list means valid.
// The messages use the String() coercion of values.
//
// The layer decides one rule, the status: a saved layer takes candidate,
// approved or retired. The draft layer takes draft or no status key, which
// means draft (defect N-15), so a draft cannot pass as a saved build and a
// saved file cannot claim to be a draft.
//
// Worker and conductor builds are validated identically. route_scope is
// optional on every tier: it is checked only when present. A
// subagent-tier build never launches alone, so skills_first, mcp, requires and
// subagents must be empty (docs/decisions.md, tier matrix, defect N-11).
func ValidateBuildIn(raw any, cfg Config, routeIDs []string, layer Layer) []string {
	p, ok := raw.(*Map)
	if !ok {
		return []string{"build is not a mapping"}
	}
	var errs []string
	add := func(msg string) { errs = append(errs, msg) }

	if id, ok := p.Value("id").(string); !ok || !kebab.MatchString(id) {
		add("id must be kebab-case")
	}
	written, hasStatus := p.Get("status")
	status, isString := written.(string)
	switch {
	case layer == LayerDraft:
		if hasStatus && (!isString || status != StatusDraft) {
			add("status must be draft in a draft")
		}
	case !isString || !slices.Contains(Statuses, status):
		add("status must be one of " + strings.Join(Statuses, ", "))
	}
	for _, v := range []struct {
		key  string
		list []string
	}{{"model", cfg.Models}, {"effort", cfg.Efforts}} {
		if problem := valueProblem(v.key, p.Value(v.key), v.list); problem != "" {
			add(problem)
		}
	}
	for _, skill := range jsconv.Strings(p.Value("skills_first")) {
		switch {
		case IsScoped(skill):
			if problem := scopedSkillProblem(skill, cfg); problem != "" {
				add(problem)
			}
		case skill == JitsoSkill:
		case Open(cfg.Skills):
			if !slices.Contains(cfg.ProjectSkillDirs, skill) {
				add(`skill "` + skill + `" has no folder under .agents/skills or .claude/skills`)
			}
		case !slices.Contains(cfg.Skills, skill):
			add(`skill "` + skill + `" is not in the allowlist`)
		}
	}
	for _, name := range jsconv.Strings(p.Value("mcp")) {
		switch {
		case IsScoped(name):
			if problem := scopedMCPProblem(name, cfg); problem != "" {
				add(problem)
			}
		case cfg.AllowsMCP(name):
		case Open(cfg.MCP):
			add(`mcp "` + name + `" is not defined in .mcp.json`)
		default:
			add(`mcp "` + name + `" is not in the allowlist`)
		}
	}
	tools := jsconv.Strings(p.Value("tools"))
	if Open(cfg.Tools) {
		if slices.ContainsFunc(tools, func(t string) bool { return !usableText(t) }) {
			add("tools must hold non-empty values with no control character")
		}
	} else {
		for _, tool := range tools {
			if !slices.Contains(cfg.Tools, tool) {
				add(`tool "` + tool + `" is not in the allowlist`)
			}
		}
	}
	if v, present := p.Get("subagents"); present && !kebabList(v) {
		add("subagents must be a list of kebab-case names")
	}
	for _, id := range jsconv.Strings(p.Value("route_scope")) {
		if id != "none" && !slices.Contains(routeIDs, id) {
			add(`route_scope id "` + id + `" is not an instruction section`)
		}
	}
	for _, field := range jsconv.Strings(p.Value("requires")) {
		if !FieldName.MatchString(field) {
			add(`required field "` + field + `" is not a field name`)
		}
	}
	return append(errs, tierProblems(p)...)
}

// valueProblem is the problem of the model or the effort of a build, "" for
// none. A restricting list names the value it refuses, and says a key the file
// lacks is missing. An open list takes any usable text or number, and its
// problem never repeats the value.
func valueProblem(key string, v any, list []string) string {
	text := jsconv.String(v)
	if !Open(list) {
		if v == jsconv.Undefined {
			return key + " is missing"
		}
		if slices.Contains(list, text) {
			return ""
		}
		return key + ` "` + text + `" is not in the allowlist`
	}
	switch v.(type) {
	case string, int, int64, uint64, float64:
		if v != jsconv.Undefined && usableText(text) {
			return ""
		}
	}
	return key + " must be a non-empty value with no control character"
}

// tierOf is the tier a build file declares: worker when the key is absent or
// not a known tier (tierProblems reports the latter).
func tierOf(p *Map) string {
	if s, ok := p.Value("tier").(string); ok && slices.Contains(Tiers, s) {
		return s
	}
	return TierWorker
}

// tierProblems are the rules of the tier field, the value, what a subagent-tier build must carry to reach claude as an
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
