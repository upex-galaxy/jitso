package adopt

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func lines(prefix string, from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, "%s%d\n", prefix, i)
	}
	return b.String()
}

func TestUnified(t *testing.T) {
	twenty := lines("l", 1, 20)
	changed := func(n ...int) string {
		ls := strings.SplitAfter(twenty, "\n")
		for _, i := range n {
			ls[i-1] = fmt.Sprintf("L%d\n", i)
		}
		return strings.Join(ls, "")
	}
	tests := []struct {
		name     string
		old, new []byte
		ctx      int
		want     string
	}{
		{"absent to file", nil, []byte("a\nb\n"), 3,
			"--- /dev/null\n+++ b/x\n@@ -0,0 +1,2 @@\n+a\n+b\n"},
		{"absent to empty", nil, []byte{}, 3, "--- /dev/null\n+++ b/x\n"},
		{"empty to file", []byte{}, []byte("a\n"), 3,
			"--- a/x\n+++ b/x\n@@ -0,0 +1 @@\n+a\n"},
		{"file to empty", []byte("a\n"), []byte{}, 3,
			"--- a/x\n+++ b/x\n@@ -1 +0,0 @@\n-a\n"},
		{"file removed", []byte("a\nb\n"), nil, 3,
			"--- a/x\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-a\n-b\n"},
		{"identical", []byte("a\n"), []byte("a\n"), 3, ""},
		{"both absent", nil, nil, 3, ""},
		{"both empty", []byte{}, []byte{}, 3, ""},
		{"prefix only", []byte("x\nb\nc\n"), []byte("y\nb\nc\n"), 3,
			"--- a/x\n+++ b/x\n@@ -1,3 +1,3 @@\n-x\n+y\n b\n c\n"},
		{"suffix only", []byte("a\nb\nx\n"), []byte("a\nb\ny\n"), 3,
			"--- a/x\n+++ b/x\n@@ -1,3 +1,3 @@\n a\n b\n-x\n+y\n"},
		{"append", []byte("a\n"), []byte("a\nb\n"), 3,
			"--- a/x\n+++ b/x\n@@ -1 +1,2 @@\n a\n+b\n"},
		{"insert with no context", []byte("a\nb\n"), []byte("a\nx\nb\n"), 0,
			"--- a/x\n+++ b/x\n@@ -1,0 +2 @@\n+x\n"},
		{"delete with no context", []byte("a\nx\nb\n"), []byte("a\nb\n"), 0,
			"--- a/x\n+++ b/x\n@@ -2 +1,0 @@\n-x\n"},
		{"two change points, no context", []byte(twenty), []byte(changed(3, 18)), 0,
			"--- a/x\n+++ b/x\n@@ -3 +3 @@\n-l3\n+L3\n@@ -18 +18 @@\n-l18\n+L18\n"},
		{"two change points, three lines of context", []byte(twenty), []byte(changed(3, 18)), 3,
			"--- a/x\n+++ b/x\n" +
				"@@ -1,6 +1,6 @@\n l1\n l2\n-l3\n+L3\n l4\n l5\n l6\n" +
				"@@ -15,6 +15,6 @@\n l15\n l16\n l17\n-l18\n+L18\n l19\n l20\n"},
		{"close change points share a hunk", []byte(twenty), []byte(changed(3, 8)), 3,
			"--- a/x\n+++ b/x\n@@ -1,11 +1,11 @@\n l1\n l2\n-l3\n+L3\n l4\n l5\n l6\n l7\n-l8\n+L8\n l9\n l10\n l11\n"},
		{"change points one apart do not share without context", []byte(twenty), []byte(changed(3, 5)), 0,
			"--- a/x\n+++ b/x\n@@ -3 +3 @@\n-l3\n+L3\n@@ -5 +5 @@\n-l5\n+L5\n"},
		{"CRLF is kept", []byte("a\r\nb\r\n"), []byte("a\r\nc\r\n"), 3,
			"--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n a\r\n-b\r\n+c\r\n"},
		{"no newline at the end gains one", []byte("a\nb"), []byte("a\nb\n"), 3,
			"--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+b\n"},
		{"no newline at the end kept", []byte("a\nb"), []byte("a\nc"), 3,
			"--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+c\n\\ No newline at end of file\n"},
		{"unchanged last line without newline is context", []byte("a\nb"), []byte("x\nb"), 3,
			"--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n-a\n+x\n b\n\\ No newline at end of file\n"},
		{"negative context is zero", []byte("a\nb\nc\n"), []byte("a\nX\nc\n"), -2,
			"--- a/x\n+++ b/x\n@@ -2 +2 @@\n-b\n+X\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Unified("x", tt.old, tt.new, tt.ctx); got != tt.want {
				t.Errorf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestUnifiedHidesUnchangedLinesWithNoContext(t *testing.T) {
	old := "{\n  \"env\": {\"TOKEN\": \"s3cret\"},\n  \"hooks\": {}\n}\n"
	new := "{\n  \"env\": {\"TOKEN\": \"s3cret\"},\n  \"hooks\": {\"a\": 1}\n}\n"
	got := Unified(".claude/settings.json", []byte(old), []byte(new), 0)
	if strings.Contains(got, "s3cret") {
		t.Errorf("an unchanged line was printed:\n%s", got)
	}
	if !strings.Contains(got, "+  \"hooks\": {\"a\": 1}") {
		t.Errorf("the change is missing:\n%s", got)
	}
}

func TestCellCapFallbacks(t *testing.T) {
	// 5 lines against 5 different lines is a 6x6 table, 36 cells.
	old := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\n"
	new := "a\nb\nc\nd\nE1\nE2\nE3\nE4\nE5\nj\nk\nl\n"
	// the middle of old is e..i (5 lines) and of new E1..E5 (5 lines)
	t.Run("under the cap the diff is by LCS", func(t *testing.T) {
		got := unified("x", []byte(old), []byte(new), 1, FallbackHunk, 36)
		want := "--- a/x\n+++ b/x\n@@ -4,7 +4,7 @@\n d\n-e\n-f\n-g\n-h\n-i\n+E1\n+E2\n+E3\n+E4\n+E5\n j\n"
		if got != want {
			t.Errorf("got\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("above the cap: one prefix/suffix hunk", func(t *testing.T) {
		// two changes far apart would be two hunks by LCS; the fallback is one
		o := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n"
		n := "a\nB\nc\nd\ne\nf\ng\nh\nI\nj\n"
		lcs := unified("x", []byte(o), []byte(n), 0, FallbackHunk, MaxCells)
		if strings.Count(lcs, "@@ -") != 2 {
			t.Fatalf("LCS should give two hunks:\n%s", lcs)
		}
		got := unified("x", []byte(o), []byte(n), 0, FallbackHunk, 10)
		want := "--- a/x\n+++ b/x\n@@ -2,8 +2,8 @@\n-b\n-c\n-d\n-e\n-f\n-g\n-h\n-i\n+B\n+c\n+d\n+e\n+f\n+g\n+h\n+I\n"
		if got != want {
			t.Errorf("got\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("above the cap: summary policy", func(t *testing.T) {
		o := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n"
		n := "a\nB\nc\nd\ne\nf\ng\nh\nI\nj\n"
		got := unified(".claude/settings.json", []byte(o), []byte(n), 0, FallbackSummary, 10)
		want := "(.claude/settings.json: 16 lines change, too large to show)\n"
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	})
	t.Run("the real cap, with two thousand lines each side", func(t *testing.T) {
		var o, n strings.Builder
		o.WriteString("top\n")
		n.WriteString("top\n")
		for i := 0; i < 2100; i++ {
			fmt.Fprintf(&o, "old %d\n", i)
			fmt.Fprintf(&n, "new %d\n", i)
		}
		o.WriteString("end\n")
		n.WriteString("end\n")
		hunk := Unified("big", []byte(o.String()), []byte(n.String()), 3)
		if !strings.Contains(hunk, "@@ -1,2102 +1,2102 @@\n top\n-old 0\n") || strings.Count(hunk, "@@ -") != 1 {
			t.Errorf("not one fallback hunk: %.200q", hunk)
		}
		sum := UnifiedWith("big", []byte(o.String()), []byte(n.String()), 0, FallbackSummary)
		if sum != "(big: 4200 lines change, too large to show)\n" {
			t.Errorf("summary = %q", sum)
		}
	})
	t.Run("one side empty never needs the table", func(t *testing.T) {
		var o strings.Builder
		for i := 0; i < 5000; i++ {
			fmt.Fprintf(&o, "line %d\n", i)
		}
		got := UnifiedWith("big", []byte("x\n"), []byte(o.String()), 0, FallbackSummary)
		if !strings.Contains(got, "@@ -1 +1,5000 @@") {
			t.Errorf("got %.120q", got)
		}
	})
}

func TestLineCount(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{{"", 0}, {"a", 1}, {"a\n", 1}, {"a\nb", 2}, {"a\nb\n", 2}, {"\n", 1}, {"a\r\nb\r\n", 2}}
	for _, tt := range tests {
		if got := LineCount([]byte(tt.in)); got != tt.want {
			t.Errorf("LineCount(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// TestEditScriptReplays checks on random inputs that the script is a valid
// edit: the unchanged and removed lines give the old text, the unchanged and
// added lines give the new one.
func TestEditScriptReplays(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	pick := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("%c\n", 'a'+rng.Intn(5))
		}
		return out
	}
	for i := 0; i < 300; i++ {
		a, c := pick(rng.Intn(14)), pick(rng.Intn(14))
		ops, fell := diffOps(a, c, MaxCells)
		if fell {
			t.Fatal("small input fell back")
		}
		var gotA, gotC []string
		for _, o := range ops {
			if o.kind != opIns {
				gotA = append(gotA, o.line)
			}
			if o.kind != opDel {
				gotC = append(gotC, o.line)
			}
		}
		if strings.Join(gotA, "") != strings.Join(a, "") || strings.Join(gotC, "") != strings.Join(c, "") {
			t.Fatalf("script does not replay\n a=%q\n c=%q\nops=%v", a, c, ops)
		}
	}
}
