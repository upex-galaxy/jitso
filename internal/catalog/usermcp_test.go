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

func TestLiteralArgFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []any
		literal []string
		want    []string
	}{
		{"value after a secret flag", []any{"--api-key", "v"}, nil, []string{"--api-key"}},
		{"flag=value", []any{"--token=v"}, nil, []string{"--token"}},
		{"bare flag=value", []any{"password=v"}, nil, []string{"password"}},
		{"case and dashes ignored", []any{"-Auth-Token", "v", "--SECRET=v"}, nil, []string{"-Auth-Token", "--SECRET"}},
		{"reference passes", []any{"--api-key", "${K}", "--token=${T:-x}"}, nil, nil},
		{"reference with text around it is literal", []any{"--api-key", "pre-${K}"}, nil, []string{"--api-key"}},
		{"not a secret flag", []any{"--port", "9", "-k", "v", "--data=${X}", "--author", "x"}, nil, []string{"--author"}},
		{"url with auth is not a flag", []any{"https://auth.example.invalid/x?a=b", "--url=https://h/?auth=1"}, nil, nil},
		{"a flag followed by a flag takes no value", []any{"--auth", "--verbose"}, nil, nil},
		{"trailing flag takes no value", []any{"--token"}, nil, nil},
		{"literal_env allows", []any{"--token=v", "--api-key", "v"}, []string{"s/--token"}, []string{"--api-key"}},
		{"listed for another server", []any{"--token=v"}, []string{"t/--token"}, []string{"--token"}},
		{"repeated flag once", []any{"--token=a", "--token", "b"}, nil, []string{"--token"}},
		{"a number after a secret flag is a literal", []any{3.0, "--token", 4.0}, nil, []string{"--token"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LiteralArgFlags("s", map[string]any{"args": tt.args}, tt.literal)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUserServerCopyRefusesArgSecretWithoutPrintingIt(t *testing.T) {
	got, problem := UserServerCopy("s", map[string]any{"command": "c", "args": []any{"--api-key", "sk-leak-1234"}}, nil)
	want := `user mcp "s" has a literal value in args after --api-key: use ${VAR}, or list s/--api-key in literal_env of ~/.jitso/jitso.yaml`
	if problem != want || got != nil {
		t.Fatalf("problem %q, copy %v", problem, got)
	}
	if strings.Contains(problem, "sk-leak-1234") {
		t.Error("the value leaked")
	}
}
