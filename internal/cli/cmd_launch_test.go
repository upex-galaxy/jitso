package cli

import (
	"strings"
	"testing"
)

// A real launch on Windows stops with one line before it touches Orca or the
// project; a dry run does not stop there.
func TestLaunchOnWindows(t *testing.T) {
	old := launchGOOS
	launchGOOS = "windows"
	t.Cleanup(func() { launchGOOS = old })
	root := gitProject(t)

	code, out, errOut := runIn(t, root, "", "launch", "--build", "any", "--brief", ".jitso/briefs/b.md", "--mode", "direct")
	if code != 3 || out != "" || !strings.HasPrefix(errOut, "jitso: jitso launch does not run on Windows") || strings.Count(errOut, "\n") != 1 {
		t.Errorf("real launch: exit %d, stdout %q, stderr %q", code, out, errOut)
	}

	_, _, errOut = runIn(t, root, "", "launch", "--build", "any", "--brief", ".jitso/briefs/b.md", "--mode", "direct", "--dry-run")
	if strings.Contains(errOut, "does not run on Windows") {
		t.Errorf("dry run refused: %q", errOut)
	}
}
