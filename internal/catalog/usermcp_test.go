package catalog

import (
	"reflect"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/jsonx"
)

func TestUserServerCopy(t *testing.T) {
	tests := []struct {
		name    string
		def     map[string]any
		literal []string
		want    string
		problem string
	}{
		{"references and known keys only", map[string]any{
			"command": "c", "args": []any{"${A:-x}"}, "type": "stdio", "env": map[string]any{"B": "${B}", "A": "pre-${A}"}, "disabled": true,
		}, nil, `{"type":"stdio","command":"c","args":["${A:-x}"],"env":{"A":"pre-${A}","B":"${B}"}}`, ""},
		{"http with headers and helper", map[string]any{
			"type": "http", "url": "https://h/${T}", "headers": map[string]any{"Authorization": "Bearer ${K}"}, "headersHelper": "/bin/h",
		}, nil, `{"type":"http","url":"https://h/${T}","headers":{"Authorization":"Bearer ${K}"},"headersHelper":"/bin/h"}`, ""},
		{"literal env", map[string]any{"command": "c", "env": map[string]any{"TOKEN": "secret-value"}}, nil, "",
			`user mcp "s" has a literal value in env TOKEN: use ${VAR}, or list s/TOKEN in literal_env of ~/.jitso/jitso.yaml`},
		{"literal header", map[string]any{"url": "u", "headers": map[string]any{"X": "v"}}, nil, "",
			`user mcp "s" has a literal value in headers X: use ${VAR}, or list s/X in literal_env of ~/.jitso/jitso.yaml`},
		{"a number is a literal", map[string]any{"command": "c", "env": map[string]any{"N": 3.0}}, nil, "",
			`user mcp "s" has a literal value in env N: use ${VAR}, or list s/N in literal_env of ~/.jitso/jitso.yaml`},
		{"$VAR without braces is a literal", map[string]any{"command": "c", "env": map[string]any{"T": "$TOKEN"}}, nil, "",
			`user mcp "s" has a literal value in env T: use ${VAR}, or list s/T in literal_env of ~/.jitso/jitso.yaml`},
		{"listed literal", map[string]any{"command": "c", "env": map[string]any{"MODE": "v"}}, []string{"s/MODE"}, `{"command":"c","env":{"MODE":"v"}}`, ""},
		{"listed for another server", map[string]any{"command": "c", "env": map[string]any{"MODE": "v"}}, []string{"t/MODE"}, "",
			`user mcp "s" has a literal value in env MODE: use ${VAR}, or list s/MODE in literal_env of ~/.jitso/jitso.yaml`},
		{"oauth", map[string]any{"url": "u", "oauth": map[string]any{}}, nil, "", `user mcp "s" uses OAuth, which a launch cannot carry`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, problem := UserServerCopy("s", tt.def, tt.literal)
			if problem != tt.problem {
				t.Fatalf("problem %q, want %q", problem, tt.problem)
			}
			if problem != "" {
				if got != nil {
					t.Error("a refused server has no copy")
				}
				if strings.Contains(problem, "secret-value") {
					t.Error("the value leaked")
				}
				return
			}
			data, _ := jsonx.Marshal(got)
			if string(data) != tt.want {
				t.Errorf("got  %s\nwant %s", data, tt.want)
			}
		})
	}
}

func TestReferences(t *testing.T) {
	def, _ := UserServerCopy("s", map[string]any{
		"command": "${CMD}", "args": []any{"${A:-x}", "${B}"}, "env": map[string]any{"Z": "${B}", "Y": "${C}"},
	}, nil)
	if got := References(def); !reflect.DeepEqual(got, []string{"CMD", "B", "C"}) {
		t.Errorf("got %v", got)
	}
}
