package adopt

import (
	"strings"
	"testing"
)

// writtenHooks stands in for the test settingsjson gives: the lines of a
// jitso group.
func writtenHooks(line string) bool {
	switch line {
	case `"hooks": {`, `"hooks": [`, `"type": "command"`, `"command": "jitso hook guard"`, `"matcher": "Bash"`:
		return true
	}
	return false
}

func TestUnifiedSensitive(t *testing.T) {
	const (
		comma   = "~ (a line of the existing file changes only by a comma; not shown)\n"
		removed = "- (a line of the existing file is removed; not shown)\n"
		added   = "+ (a line is added that jitso did not write; not shown)\n"
		hdr     = "--- a/s.json\n+++ b/s.json\n"
	)
	tests := []struct {
		name     string
		old, new string
		want     string
	}{
		{
			"a user line that only gains a comma is one marker and its text never prints",
			"{\n  \"k\": \"SECRET\"\n}\n",
			"{\n  \"k\": \"SECRET\",\n  \"hooks\": {\n    \"type\": \"command\",\n    \"command\": \"jitso hook guard\"\n  }\n}\n",
			hdr + "@@ -2 +2,5 @@\n" + comma + "+  \"hooks\": {\n+    \"type\": \"command\",\n+    \"command\": \"jitso hook guard\"\n+  }\n",
		},
		{
			"a user line that loses a comma is one marker",
			"{\n  \"k\": \"SECRET\",\n  \"hooks\": {\n    \"type\": \"command\",\n    \"command\": \"jitso hook guard\"\n  }\n}\n",
			"{\n  \"k\": \"SECRET\"\n}\n",
			hdr + "@@ -2,5 +2 @@\n" + comma + "-  \"hooks\": {\n-    \"type\": \"command\",\n-    \"command\": \"jitso hook guard\"\n-  }\n",
		},
		{
			"structural lines print",
			"{\n  \"a\": {\n    \"x\": 1\n  }\n}\n",
			"{\n  \"a\": {\n    \"x\": 1\n  },\n  \"hooks\": {\n    \"type\": \"command\"\n  }\n}\n",
			hdr + "@@ -3,0 +4,3 @@\n+  },\n+  \"hooks\": {\n+    \"type\": \"command\"\n",
		},
		{
			"a removed user line without a counterpart is masked",
			"{\n  \"a\": \"SECRET\",\n  \"hooks\": {\n    \"type\": \"command\"\n  }\n}\n",
			"{\n  \"hooks\": {\n    \"type\": \"command\"\n  }\n}\n",
			hdr + "@@ -2 +1,0 @@\n" + removed,
		},
		{
			"an added line that jitso does not write is masked",
			"{\n  \"hooks\": {\n    \"type\": \"command\"\n  }\n}\n",
			"{\n  \"hooks\": {\n    \"type\": \"command\"\n  },\n  \"x\": \"SECRET\"\n}\n",
			hdr + "@@ -4 +4,2 @@\n-  }\n+  },\n" + added,
		},
		{
			"a text jitso writes and the user has too is still printed",
			"{\n  \"type\": \"command\",\n  \"command\": \"jitso hook guard\"\n}\n",
			"{\n  \"type\": \"command\",\n  \"command\": \"jitso hook guard\",\n  \"hooks\": {\n  }\n}\n",
			hdr + "@@ -3 +3,3 @@\n-  \"command\": \"jitso hook guard\"\n+  \"command\": \"jitso hook guard\",\n+  \"hooks\": {\n+  }\n",
		},
		{
			"a user line that merely mentions jitso is not jitso's",
			"{\n  \"c\": \"./x.sh jitso hook guard SECRET\"\n}\n",
			"{\n  \"c\": \"./x.sh jitso hook guard SECRET\",\n  \"hooks\": {\n  }\n}\n",
			hdr + "@@ -2 +2,3 @@\n" + comma + "+  \"hooks\": {\n+  }\n",
		},
		{
			"a new file prints in full",
			"",
			"{\n  \"hooks\": {\n    \"type\": \"command\"\n  }\n}\n",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var old []byte
			if tt.old != "" {
				old = []byte(tt.old)
			}
			got := UnifiedSensitive("s.json", old, []byte(tt.new), writtenHooks)
			if strings.Contains(got, "SECRET") {
				t.Errorf("a user line was printed:\n%s", got)
			}
			if tt.want == "" {
				// A new file: every added line is jitso's, so none is masked.
				if strings.Contains(got, "not shown") || !strings.Contains(got, "+++ b/s.json") {
					t.Errorf("got\n%s", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestUnifiedSensitiveUnchangedIsEmpty(t *testing.T) {
	if got := UnifiedSensitive("s.json", []byte("{}\n"), []byte("{}\n"), nil); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestUnifiedSensitiveCompact(t *testing.T) {
	const fallback = "(s.json: the file is compact JSON; its changes are not shown)\n"
	tests := []struct {
		name     string
		old, new string
		want     string
	}{
		{"one line with a secret", "{\"k\":\"SECRET\"}\n", "{\n  \"k\": \"SECRET\",\n  \"hooks\": {}\n}\n", fallback},
		{"one line without a final newline", "{\"k\":\"SECRET\"}", "{\n  \"hooks\": {}\n}\n", fallback},
		{"two lines with a secret", "{\"k\":\n\"SECRET\"}\n", "{}\n", fallback},
		{"removing a compact file", "{\"k\":\"SECRET\"}\n", "", fallback},
		{"three lines are not compact", "{\n  \"k\": \"SECRET\"\n}\n", "{\n  \"k\": \"SECRET\",\n  \"hooks\": {}\n}\n", "--- a/s.json\n+++ b/s.json\n@@ -2 +2,2 @@\n~ (a line of the existing file changes only by a comma; not shown)\n+  \"hooks\": {}\n"},
		{"a bare {} hides nothing, so its diff prints", "{}\n", "{\n  \"hooks\": {}\n}\n", "--- a/s.json\n+++ b/s.json\n@@ -1 +1,3 @@\n-{}\n+{\n+  \"hooks\": {}\n+}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nw []byte
			if tt.new != "" {
				nw = []byte(tt.new)
			}
			got := UnifiedSensitive("s.json", []byte(tt.old), nw, func(l string) bool { return l == `"hooks": {}` })
			if got != tt.want {
				t.Errorf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestUnifiedSensitiveTooLarge(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\n"
	new := "a\nb\nc\nd\nE1\nE2\nE3\nE4\nE5\nj\nk\nl\n"
	got := unifiedSensitive("s.json", []byte(old), []byte(new), nil, 10)
	if got != "(s.json: 10 lines change, too large to show)\n" {
		t.Errorf("got %q", got)
	}
}

// A file that is not sensitive keeps every line of its diff, a comma-only pair
// and the unchanged lines around it included.
func TestNonSensitiveDiffIsUnchanged(t *testing.T) {
	old := "{\n  \"env\": {\"T\": \"s3cret\"},\n  \"k\": \"v\"\n}\n"
	new := "{\n  \"env\": {\"T\": \"s3cret\"},\n  \"k\": \"v\",\n  \"x\": 1\n}\n"
	want := "--- a/c.json\n+++ b/c.json\n@@ -1,4 +1,5 @@\n {\n   \"env\": {\"T\": \"s3cret\"},\n-  \"k\": \"v\"\n+  \"k\": \"v\",\n+  \"x\": 1\n }\n"
	if got := Unified("c.json", []byte(old), []byte(new), 3); got != want {
		t.Errorf("Unified:\n%q\nwant\n%q", got, want)
	}
	var b strings.Builder
	Preview(&b, Plan{Changes: []Change{{Display: "c.json", Kind: KindFile, Old: []byte(old), New: []byte(new)}}})
	if b.String() != want {
		t.Errorf("Preview:\n%q\nwant\n%q", b.String(), want)
	}
}
