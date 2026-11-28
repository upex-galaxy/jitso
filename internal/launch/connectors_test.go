package launch

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/harness"
)

const listing = "Checking MCP server health...\n\n" +
	"plugin:x:y: secret-cmd --token sentinel - ok\n" +
	"Claude.AI Supabase: https://mcp.supabase.com/mcp - Connected\n" +
	"claude.ai Google Drive: https://drive.invalid/mcp - Needs authentication\n" +
	"claude.ai Odd: not-a-url - ?\n" +
	"dbhub: cmd --x\n"

func TestParseConnectors(t *testing.T) {
	got := ParseConnectors(listing)
	want := []listedConnector{{"Supabase", "https://mcp.supabase.com/mcp"}, {"Google Drive", "https://drive.invalid/mcp"}, {"Odd", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got := ParseConnectors(""); got != nil {
		t.Errorf("nothing listed, got %+v", got)
	}
}

func TestResolveConnectors(t *testing.T) {
	list := func() (string, error) { return listing, nil }
	got, err := resolveConnectors([]string{"supabase", "Google Drive"}, list)
	want := []harness.CloudConnector{{Name: "supabase", URL: "https://mcp.supabase.com/mcp"}, {Name: "Google Drive", URL: "https://drive.invalid/mcp"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, %v", got, err)
	}

	_, err = resolveConnectors([]string{"Notion"}, list)
	if text := fail.Text(err); fail.Code(err) != 3 || text != `JITSO REJECTED: no claude.ai connector named "Notion" (available: Supabase, Google Drive, Odd)` {
		t.Errorf("not found: %d %q", fail.Code(err), text)
	}
	_, err = resolveConnectors([]string{"Notion"}, func() (string, error) { return "dbhub: cmd\n", nil })
	if !strings.HasSuffix(fail.Text(err), "(available: none)") {
		t.Errorf("none: %q", fail.Text(err))
	}
	_, err = resolveConnectors([]string{"Odd"}, list)
	if fail.Code(err) != 3 || !strings.Contains(fail.Text(err), `"Odd" has no URL`) {
		t.Errorf("no url: %d %q", fail.Code(err), fail.Text(err))
	}
	_, err = resolveConnectors([]string{"Supabase"}, func() (string, error) { return listing, os.ErrDeadlineExceeded })
	if fail.Code(err) != 3 || strings.Contains(fail.Text(err), "sentinel") || !strings.HasPrefix(fail.Text(err), "jitso: could not list the claude.ai connectors: ") {
		t.Errorf("list fails: %d %q", fail.Code(err), fail.Text(err))
	}
}

func TestCloudItemsAndServerNames(t *testing.T) {
	mcp := []string{"dbhub", "cloud:A", "user:engram", "cloud:B", "cloud:A"}
	if got, want := cloudItems(mcp), []string{"A", "B"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cloudItems %v", got)
	}
	if got, want := serverNames(mcp), []string{"dbhub", "engram"}; !reflect.DeepEqual(got, want) {
		t.Errorf("serverNames %v", got)
	}
	if got, want := projectNames(mcp), []string{"dbhub"}; !reflect.DeepEqual(got, want) {
		t.Errorf("projectNames %v", got)
	}
	if got, want := withoutCloud(mcp), []string{"dbhub", "user:engram"}; !reflect.DeepEqual(got, want) {
		t.Errorf("withoutCloud %v", got)
	}
}

// fakeClaude puts a shell script named claude first on PATH.
func fakeClaude(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands for claude")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin:/bin")
}

func TestRunMCPList(t *testing.T) {
	fakeClaude(t, `[ "$1 $2" = "mcp list" ] && echo "claude.ai A: https://a.invalid/mcp"; echo oops >&2`)
	out, err := runMCPList("claude", time.Minute)
	if err != nil || out != "claude.ai A: https://a.invalid/mcp\n" {
		t.Errorf("got %q, %v", out, err)
	}

	fakeClaude(t, "echo sentinel-out; echo sentinel-err >&2; exit 4")
	if _, err := runMCPList("claude", time.Minute); err == nil || err.Error() != "claude mcp list failed with exit status 4" {
		t.Errorf("exit: %v", err)
	}

	fakeClaude(t, "sleep 5")
	start := time.Now()
	if _, err := runMCPList("claude", 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), "did not answer in") || time.Since(start) > 4*time.Second {
		t.Errorf("timeout: %v after %v", err, time.Since(start))
	}

	t.Setenv("PATH", t.TempDir())
	if _, err := runMCPList("claude", time.Minute); err == nil || err.Error() != "claude is not on PATH" {
		t.Errorf("missing: %v", err)
	}
}
