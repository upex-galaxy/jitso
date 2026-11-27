package claude

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOwnerInbound(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		want    string
		warning bool
	}{
		{"accept", `{"crossSessionInbound":"accept"}`, "accept", false},
		{"hold among other keys", `{"env":{"A":"secret"},"crossSessionInbound":"hold"}`, "hold", false},
		{"refuse", `{"crossSessionInbound":"refuse"}`, "refuse", false},
		{"another word", `{"crossSessionInbound":"ask"}`, "", false},
		{"not a string", `{"crossSessionInbound":true}`, "", false},
		{"null", `{"crossSessionInbound":null}`, "", false},
		{"no key", `{"CrossSessionInbound":"accept","x":{"crossSessionInbound":"accept"}}`, "", false},
		{"not an object", `["crossSessionInbound"]`, "", false},
		{"not JSON", `{"crossSessionInbound":"accept","token":"secret",`, "", true},
		{"empty file", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := settingsFile(t, filepath.Join(home, ".claude"), tt.file)
			got, warning := Adapter{}.OwnerInbound(home, noEnv)
			if got != tt.want || (warning != "") != tt.warning {
				t.Fatalf("got %q, warning %q", got, warning)
			}
			if tt.warning && warning != "jitso: warning: "+path+" is not valid JSON, so the session gets no crossSessionInbound" {
				t.Errorf("warning %q", warning)
			}
			if strings.Contains(warning, "secret") {
				t.Errorf("the warning quotes the file: %q", warning)
			}
		})
	}
}

func TestOwnerInboundFolder(t *testing.T) {
	home, moved := t.TempDir(), t.TempDir()
	settingsFile(t, filepath.Join(home, ".claude"), `{"crossSessionInbound":"accept"}`)
	settingsFile(t, moved, `{"crossSessionInbound":"hold"}`)
	tests := []struct {
		name, home, configDir, want string
	}{
		{"home", home, "", "accept"},
		{"CLAUDE_CONFIG_DIR wins", home, moved, "hold"},
		{"no fallback from CLAUDE_CONFIG_DIR", home, t.TempDir(), ""},
		{"CLAUDE_CONFIG_DIR without a home", "", moved, "hold"},
		{"no file", t.TempDir(), "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := func(name string) (string, bool) { return tt.configDir, name == ConfigDirEnv }
			if got, warning := (Adapter{}).OwnerInbound(tt.home, env); got != tt.want || warning != "" {
				t.Errorf("got %q, warning %q", got, warning)
			}
		})
	}
}

// With no home the folder .claude of the working directory is the project's.
func TestOwnerInboundNoHome(t *testing.T) {
	project := t.TempDir()
	settingsFile(t, filepath.Join(project, ".claude"), `{"crossSessionInbound":"accept"}`)
	t.Chdir(project)
	if got, warning := (Adapter{}).OwnerInbound("", noEnv); got != "" || warning != "" {
		t.Errorf("got %q, warning %q", got, warning)
	}
}

func TestOwnerInboundUnreadable(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "settings.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, warning := (Adapter{}).OwnerInbound(home, noEnv); got != "" || warning != "" {
		t.Errorf("a folder: got %q, warning %q", got, warning)
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("no permission bit stops this reader")
	}
	locked := t.TempDir()
	if err := os.Chmod(settingsFile(t, filepath.Join(locked, ".claude"), `{"crossSessionInbound":"accept"}`), 0); err != nil {
		t.Fatal(err)
	}
	if got, warning := (Adapter{}).OwnerInbound(locked, noEnv); got != "" || warning != "" {
		t.Errorf("no permission: got %q, warning %q", got, warning)
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
