package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/harness"
)

func obj(t *testing.T, text string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func keysOf(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestFilter(t *testing.T) {
	const guard = `{"type":"command","command":"jitso hook guard"}`
	tests := []struct {
		name             string
		user, project    string
		exclude, include []string
		copied, cut      []string
		check            func(t *testing.T, doc map[string]any)
	}{
		{name: "unknown and plain keys travel", user: `{"theme":"dark","futureKey":{"a":1},"crossSessionInbound":"anything"}`,
			copied: []string{"crossSessionInbound", "futureKey", "theme"}},
		{name: "context keys are cut", user: `{"enabledPlugins":{},"extraKnownMarketplaces":{},"pluginConfigs":{},"skillOverrides":{},"enableAllProjectMcpServers":true,"enabledMcpjsonServers":[],"disabledMcpjsonServers":[],"allowedMcpServers":[],"deniedMcpServers":[]}`,
			cut: []string{"allowedMcpServers", "deniedMcpServers", "disabledMcpjsonServers", "enableAllProjectMcpServers", "enabledMcpjsonServers", "enabledPlugins", "extraKnownMarketplaces", "pluginConfigs", "skillOverrides"}},
		{name: "build, per-worker and JITSO keys are cut", user: `{"model":"m","effortLevel":"low","language":"x","askUserQuestionTimeout":1,"remoteControlAtStartup":true,"skipDangerousModePermissionPrompt":false,"promptSuggestionEnabled":true}`,
			cut: []string{"askUserQuestionTimeout", "effortLevel", "language", "model", "promptSuggestionEnabled", "remoteControlAtStartup", "skipDangerousModePermissionPrompt"}},
		{name: "permissions loses only defaultMode", user: `{"permissions":{"defaultMode":"plan","allow":["a"],"deny":["b"]}}`, copied: []string{"permissions"},
			check: func(t *testing.T, doc map[string]any) {
				if p := doc["permissions"].(map[string]any); !reflect.DeepEqual(p, map[string]any{"allow": []any{"a"}, "deny": []any{"b"}}) {
					t.Errorf("permissions: %v", p)
				}
			}},
		{name: "permissions with only defaultMode is cut", user: `{"permissions":{"defaultMode":"plan"}}`, cut: []string{"permissions"}},
		{name: "an empty permissions travels", user: `{"permissions":{}}`, copied: []string{"permissions"}},
		{name: "project keys win, hooks and permissions merge", user: `{"statusLine":1,"outputStyle":"u","hooks":{"Stop":[]},"permissions":{"allow":["a"]},"other":1}`,
			project: `{"statusLine":2,"outputStyle":"p","hooks":{"Stop":[]},"permissions":{"deny":["b"]}}`,
			copied:  []string{"hooks", "other", "permissions"}, cut: []string{"outputStyle", "statusLine"}},
		{name: "env drops the variables the project defines", user: `{"env":{"A":"1","B":"2"}}`, project: `{"env":{"A":"p"}}`, copied: []string{"env"},
			check: func(t *testing.T, doc map[string]any) {
				if e := doc["env"].(map[string]any); !reflect.DeepEqual(e, map[string]any{"B": json.Number("2")}) && !reflect.DeepEqual(e, map[string]any{"B": "2"}) {
					t.Errorf("env: %v", e)
				}
			}},
		{name: "env fully defined by the project is cut", user: `{"env":{"A":"1"}}`, project: `{"env":{"A":"p"}}`, cut: []string{"env"}},
		{name: "an empty env travels", user: `{"env":{}}`, project: `{"env":{"A":"p"}}`, copied: []string{"env"}},
		{name: "exclude cuts and wins over include", user: `{"a":1,"b":2,"language":"x"}`, exclude: []string{"a", "b"}, include: []string{"b", "language"},
			copied: []string{"language"}, cut: []string{"a", "b"}},
		{name: "include lets context keys through", user: `{"enabledPlugins":{},"skillOverrides":{}}`, include: []string{"enabledPlugins", "unknown"}, copied: []string{"enabledPlugins"}, cut: []string{"skillOverrides"}},
		{name: "include never beats the build, the project or JITSO", user: `{"model":"m","effortLevel":"e","promptSuggestionEnabled":true,"outputStyle":"u","permissions":{"defaultMode":"plan"}}`,
			project: `{"outputStyle":"p"}`, include: []string{"model", "effortLevel", "promptSuggestionEnabled", "outputStyle", "permissions"},
			cut: []string{"effortLevel", "model", "outputStyle", "permissions", "promptSuggestionEnabled"}},
		{name: "guard handlers are removed, the rest of the hooks travel", user: `{"hooks":{
			"PreToolUse":[{"matcher":"Bash","hooks":[` + guard + `]},{"matcher":"Edit","hooks":[{"type":"command","command":"fmt"},{"type":"command","command":"'/a b/jitso' hook guard"}]}],
			"Stop":[{"hooks":[{"type":"command","command":"/usr/bin/jitso hook guard"}]}],
			"Empty":[],"Odd":"text","Notification":[{"hooks":[{"type":"command","command":"jitso hook other"}]}]}}`,
			copied: []string{"hooks"},
			check: func(t *testing.T, doc map[string]any) {
				want := obj(t, `{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"fmt"}]}],"Empty":[],"Odd":"text","Notification":[{"hooks":[{"type":"command","command":"jitso hook other"}]}]}`)
				if got := doc["hooks"].(map[string]any); !reflect.DeepEqual(got, want) {
					t.Errorf("hooks: %v\nwant %v", got, want)
				}
			}},
		{name: "hooks that held only the guard are cut", user: `{"hooks":{"Stop":[{"hooks":[` + guard + `]}]}}`, cut: []string{"hooks"}},
		{name: "empty file object", user: `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var project map[string]any
			if tt.project != "" {
				project = obj(t, tt.project)
			}
			doc, copied, cut := Filter(obj(t, tt.user), project, tt.exclude, tt.include)
			if !reflect.DeepEqual(copied, tt.copied) || !reflect.DeepEqual(cut, tt.cut) {
				t.Errorf("copied %v cut %v, want %v and %v", copied, cut, tt.copied, tt.cut)
			}
			if len(doc) != len(tt.copied) {
				t.Errorf("doc keys %v, copied %v", keysOf(doc), copied)
			}
			if tt.check != nil {
				tt.check(t, doc)
			}
		})
	}
}

func noEnv(string) (string, bool) { return "", false }

func settingsFile(t *testing.T, dir, text string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOwnerSettings(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		copied  []string
		warning bool
	}{
		{"keys", `{"env":{"A":"secret"},"model":"x","crossSessionInbound":"hold"}`, []string{"crossSessionInbound", "env"}, false},
		{"not an object", `["crossSessionInbound"]`, nil, false},
		{"string", `"accept"`, nil, false},
		{"null", `null`, nil, false},
		{"not JSON", `{"token":"secret",`, nil, true},
		{"empty file", "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := settingsFile(t, filepath.Join(home, ".claude"), tt.file)
			got := Adapter{}.OwnerSettings(harness.OwnerSettingsInput{Home: home, LookupEnv: noEnv})
			if !reflect.DeepEqual(got.Copied, tt.copied) || (got.Warning != "") != tt.warning {
				t.Fatalf("copied %v, warning %q", got.Copied, got.Warning)
			}
			if tt.warning && got.Warning != "jitso: warning: "+path+" is not valid JSON, so the session gets none of the settings in it" {
				t.Errorf("warning %q", got.Warning)
			}
			if strings.Contains(got.Warning, "secret") {
				t.Errorf("the warning quotes the file: %q", got.Warning)
			}
		})
	}
}

func TestOwnerSettingsWhereTheFileIs(t *testing.T) {
	home := t.TempDir()
	settingsFile(t, filepath.Join(home, ".claude"), `{"from":"home"}`)
	cfg := filepath.Join(home, "cfg")
	settingsFile(t, cfg, `{"from":"cfg"}`)
	if err := os.MkdirAll(filepath.Join(home, "bare"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := func(v string) func(string) (string, bool) {
		return func(name string) (string, bool) { return v, name == ConfigDirEnv }
	}
	if got := (Adapter{}).OwnerSettings(harness.OwnerSettingsInput{Home: home, LookupEnv: env(cfg)}); got.Doc["from"] != "cfg" {
		t.Errorf("CLAUDE_CONFIG_DIR: %v", got.Doc)
	}
	if got := (Adapter{}).OwnerSettings(harness.OwnerSettingsInput{Home: home, LookupEnv: env(filepath.Join(home, "bare"))}); len(got.Copied) != 0 {
		t.Errorf("no fallback to ~/.claude: %v", got.Doc)
	}
	if got := (Adapter{}).OwnerSettings(harness.OwnerSettingsInput{Home: home, LookupEnv: env("")}); got.Doc["from"] != "home" {
		t.Errorf("empty value is unset: %v", got.Doc)
	}
	// Without a home the relative path would be the project's own file.
	if got := (Adapter{}).OwnerSettings(harness.OwnerSettingsInput{LookupEnv: noEnv}); len(got.Copied) != 0 || got.Warning != "" {
		t.Errorf("no home: %+v", got)
	}
}

func TestOwnerSettingsProjectAndLocalFile(t *testing.T) {
	home, checkout := t.TempDir(), t.TempDir()
	settingsFile(t, filepath.Join(home, ".claude"), `{"outputStyle":"u","theme":"u"}`)
	settingsFile(t, filepath.Join(checkout, ".claude"), `{"outputStyle":"p"}`)
	if err := os.WriteFile(filepath.Join(checkout, ".claude", "settings.local.json"), []byte(`{"theme":"local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Adapter{}.OwnerSettings(harness.OwnerSettingsInput{Home: home, LookupEnv: noEnv, Checkout: checkout})
	if !reflect.DeepEqual(got.Copied, []string{"theme"}) || !reflect.DeepEqual(got.Cut, []string{"outputStyle"}) {
		t.Errorf("copied %v cut %v", got.Copied, got.Cut)
	}
}

func TestOwnerSettingsUnreadablePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a directory in place of the file reads the same everywhere; kept simple")
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "settings.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := (Adapter{}).OwnerSettings(harness.OwnerSettingsInput{Home: home, LookupEnv: noEnv}); got.Warning != "" || len(got.Copied)+len(got.Cut) != 0 {
		t.Errorf("a folder in place of the file: %+v", got)
	}
}
