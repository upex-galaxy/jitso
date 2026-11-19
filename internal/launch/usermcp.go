package launch

import (
	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/jsonx"
)

// userServers are the copied definitions of the user:<name> servers among
// names, keyed by the name as written, and one warning per variable a copy
// refers to that is unset here: Claude Code would keep its text unexpanded.
// Only the presence of a variable is checked; its value is never read out.
func userServers(names []string, user catalog.User, lookup func(string) (string, bool)) (map[string]jsonx.Object, []string, error) {
	var out map[string]jsonx.Object
	var warnings []string
	for _, item := range names {
		name, ok := catalog.UserName(item)
		if !ok {
			continue
		}
		def, ok := user.Server(name)
		if !ok {
			return nil, nil, fail.Rejectedf(`user mcp "%s" is not defined in %s`, name, catalog.UserClaudeDisplay)
		}
		copied, problem := catalog.UserServerCopy(name, def, user.LiteralEnv)
		if problem != "" {
			return nil, nil, fail.Rejected(problem)
		}
		if out == nil {
			out = map[string]jsonx.Object{}
		}
		out[item] = copied
		for _, v := range catalog.References(copied) {
			if _, set := lookup(v); !set {
				warnings = append(warnings, `jitso: warning: user mcp "`+name+`" refers to ${`+v+`}, which is not set here; the session gets the text unexpanded`)
			}
		}
	}
	return out, warnings, nil
}

// projectNames are the names that are not user:<name> servers.
func projectNames(names []string) []string {
	var out []string
	for _, n := range names {
		if _, ok := catalog.UserName(n); !ok {
			out = append(out, n)
		}
	}
	return out
}
