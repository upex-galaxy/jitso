package catalog

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsonx"
)

// varRef is one ${VAR} or ${VAR:-default} reference, as Claude Code expands
// it in an MCP server definition.
var varRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-[^}]*)?\}`)

// UserServerCopy is the definition of the user MCP server name as a launch
// writes it into mcp.json, or the problem that refuses it. command, args,
// type, url and headersHelper are copied as written; each env and headers
// value only when it holds a ${VAR} reference or its key is listed as
// <name>/<KEY> in literal_env; every other key is dropped. A literal value is
// never copied and never named: the problem names its key. A server with an
// oauth key is refused, because a copied definition cannot carry its login.
func UserServerCopy(name string, def map[string]any, literalEnv []string) (jsonx.Object, string) {
	if _, ok := def["oauth"]; ok {
		return nil, `user mcp "` + name + `" uses OAuth, which a launch cannot carry`
	}
	if flags := LiteralArgFlags(name, def, literalEnv); len(flags) > 0 {
		return nil, `user mcp "` + name + `" has a literal value in args after ` + flags[0] + `: use ${VAR}, or list ` + name + `/` + flags[0] + ` in literal_env of ` + UserConfigDisplay
	}
	var out jsonx.Object
	for _, key := range []string{"type", "command", "args", "url"} {
		if v, ok := def[key]; ok {
			out = append(out, jsonx.Field{Key: key, Value: v})
		}
	}
	for _, field := range []string{"env", "headers"} {
		v, ok := def[field]
		if !ok {
			continue
		}
		table, ok := v.(map[string]any)
		if !ok {
			continue
		}
		keys := make([]string, 0, len(table))
		for k := range table {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		values := jsonx.Object{}
		for _, k := range keys {
			value := table[k]
			s, isString := value.(string)
			if !(isString && varRef.MatchString(s)) && !slices.Contains(literalEnv, name+"/"+k) {
				return nil, `user mcp "` + name + `" has a literal value in ` + field + ` ` + k + `: use ${VAR}, or list ` + name + `/` + k + ` in literal_env of ` + UserConfigDisplay
			}
			values = append(values, jsonx.Field{Key: k, Value: value})
		}
		out = append(out, jsonx.Field{Key: field, Value: values})
	}
	if v, ok := def["headersHelper"]; ok {
		out = append(out, jsonx.Field{Key: "headersHelper", Value: v})
	}
	return out, ""
}

// LiteralKeys are the env and headers keys of a user server definition whose
// value is literal and not listed in literal_env: the keys that refuse it.
// Only key names are returned.
func LiteralKeys(name string, def map[string]any, literalEnv []string) (env, headers []string) {
	for _, field := range []string{"env", "headers"} {
		table, _ := def[field].(map[string]any)
		var keys []string
		for k, v := range table {
			s, isString := v.(string)
			if !(isString && varRef.MatchString(s)) && !slices.Contains(literalEnv, name+"/"+k) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		if field == "env" {
			env = keys
		} else {
			headers = keys
		}
	}
	return env, headers
}

var secretFlagWords = []string{"key", "token", "secret", "password", "passwd", "auth"}

var flagName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

var pureRef = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*(:-[^}]*)?\}$`)

func secretFlag(flag string) bool {
	if !flagName.MatchString(flag) {
		return false
	}
	lower := strings.ToLower(strings.TrimLeft(flag, "-"))
	for _, word := range secretFlagWords {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// LiteralArgFlags are the flags, as written in args, whose literal value
// refuses a user server: the value after a flag whose name holds key, token,
// secret, password, passwd or auth, or the value of such a flag written as
// flag=value, unless the value is a lone ${VAR} reference or <server>/<flag>
// is listed in literal_env. Only flag names are returned, in order of first
// use.
func LiteralArgFlags(name string, def map[string]any, literalEnv []string) []string {
	args, _ := def["args"].([]any)
	var flags []string
	add := func(flag string) {
		if !slices.Contains(literalEnv, name+"/"+flag) && !slices.Contains(flags, flag) {
			flags = append(flags, flag)
		}
	}
	for i := 0; i < len(args); i++ {
		arg, ok := args[i].(string)
		if !ok {
			continue
		}
		if flag, value, found := strings.Cut(arg, "="); found {
			if secretFlag(flag) && !pureRef.MatchString(value) {
				add(flag)
			}
			continue
		}
		if !strings.HasPrefix(arg, "-") || !secretFlag(arg) || i+1 >= len(args) {
			continue
		}
		value, isString := args[i+1].(string)
		if isString && (strings.HasPrefix(value, "-") || pureRef.MatchString(value)) {
			continue
		}
		add(arg)
	}
	return flags
}

// References are the variable names a copied definition refers to without a
// default, in order of first use: the ones Claude Code leaves unexpanded when
// they are unset.
func References(def jsonx.Object) []string {
	var names []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			for _, m := range varRef.FindAllStringSubmatch(x, -1) {
				if m[2] == "" && !slices.Contains(names, m[1]) {
					names = append(names, m[1])
				}
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		case jsonx.Object:
			for _, f := range x {
				walk(f.Value)
			}
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(x[k])
			}
		}
	}
	walk(def)
	return names
}
