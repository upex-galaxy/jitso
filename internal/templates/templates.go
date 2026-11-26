// Package templates holds the text a new build starts from: one template per
// tier and the jitso/jitso.yaml skeleton, embedded in the binary. Every
// template is a draft (status: draft) whose allowlist-bound values are empty,
// so a validator names each one until the author fills it in
// (docs/decisions.md, defect N-14).
package templates

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

//go:embed worker.yaml
var worker string

//go:embed conductor.yaml
var conductor string

//go:embed subagent.yaml
var subagent string

//go:embed jitso.yaml
var config string

// IDLine is the one line of a template that Build replaces with the real id.
const IDLine = "id: <id>"

// Tiers are the tiers a template exists for, in the order the usage lists them.
var Tiers = []string{"worker", "conductor", "subagent"}

// idPattern is the id of a launchable build: kebab-case, at most 64 characters.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// Build is the template of tier with its `id: <id>` line replaced by
// `id: <id>` of the given id. An unknown tier or an id that is not kebab-case
// is an error, so the text can never carry a second line or a stray key.
func Build(tier, id string) (string, error) {
	var text string
	switch tier {
	case "worker":
		text = worker
	case "conductor":
		text = conductor
	case "subagent":
		text = subagent
	default:
		return "", fmt.Errorf("no template for tier %q", tier)
	}
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("build id %q must be kebab-case", id)
	}
	return strings.Replace(text, IDLine+"\n", "id: "+id+"\n", 1), nil
}

// Config is the skeleton of jitso/jitso.yaml: the version and comments on the
// optional allowlists, none written, so every one is open.
func Config() string {
	return config
}
