package impl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jitso-contract-runner/internal/sandbox"
)

func TestDownUpLine(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		down string // DownLine(line)
		up   string // UpLine(line)
	}{
		{
			"compact launch line",
			`{"v":2,"event":"launch","id":"L1","build":"p","build_version":3}`,
			`{"v":1,"event":"launch","id":"L1","preset":"p","preset_version":3}`,
			`{"v":2,"event":"launch","id":"L1","build":"p","build_version":3}`,
		},
		{
			"spaced line keeps its spacing and key order",
			`{ "v" : 2 , "event":"launch", "build" :  "p" ,  "build_version" : 0.50 }`,
			`{ "v" : 1 , "event":"launch", "preset" :  "p" ,  "preset_version" : 0.50 }`,
			`{ "v" : 2 , "event":"launch", "build" :  "p" ,  "build_version" : 0.50 }`,
		},
		{
			"build_version string value kept",
			`{"v":2,"build":"p","build_version":"1.2"}`,
			`{"v":1,"preset":"p","preset_version":"1.2"}`,
			`{"v":2,"build":"p","build_version":"1.2"}`,
		},
		{
			"nested build and v untouched",
			`{"v":2,"orca":{"v":2,"build":"x","terminal":"t"},"build":"p","list":[{"build":"y"}]}`,
			`{"v":1,"orca":{"v":2,"build":"x","terminal":"t"},"preset":"p","list":[{"build":"y"}]}`,
			`{"v":2,"orca":{"v":2,"build":"x","terminal":"t"},"build":"p","list":[{"build":"y"}]}`,
		},
		{
			"v 1 line is untouched by Down and renamed by Up",
			`{"v":1,"event":"launch","preset":"p","preset_version":3}`,
			`{"v":1,"event":"launch","preset":"p","preset_version":3}`,
			`{"v":2,"event":"launch","build":"p","build_version":3}`,
		},
		{
			"v 3 is untouched both ways",
			`{"v":3,"build":"p","preset":"q"}`,
			`{"v":3,"build":"p","preset":"q"}`,
			`{"v":3,"build":"p","preset":"q"}`,
		},
		{
			"v written 2.0 matches 2",
			`{"v":2.0,"build":"p"}`,
			`{"v":1,"preset":"p"}`,
			`{"v":2.0,"build":"p"}`,
		},
		{
			"v as a string is not a number",
			`{"v":"2","build":"p"}`,
			`{"v":"2","build":"p"}`,
			`{"v":"2","build":"p"}`,
		},
		{
			"no v key",
			`{"event":"end","build":"p"}`,
			`{"event":"end","build":"p"}`,
			`{"event":"end","build":"p"}`,
		},
		{
			"duplicate v, the last one wins and it does not match",
			`{"v":2,"build":"p","v":3}`,
			`{"v":2,"build":"p","v":3}`,
			`{"v":2,"build":"p","v":3}`,
		},
		{
			"duplicate v, the last one matches",
			`{"v":3,"build":"p","v":2}`,
			`{"v":3,"preset":"p","v":1}`,
			`{"v":3,"build":"p","v":2}`,
		},
		{
			"end line: only v changes",
			`{"v":2,"event":"end","id":"L1","source":"cli"}`,
			`{"v":1,"event":"end","id":"L1","source":"cli"}`,
			`{"v":2,"event":"end","id":"L1","source":"cli"}`,
		},
		{
			"escapes and angle brackets stay",
			`{"v":2,"build":"a<b>&é","title":"t [jitso:L1]"}`,
			`{"v":1,"preset":"a<b>&é","title":"t [jitso:L1]"}`,
			`{"v":2,"build":"a<b>&é","title":"t [jitso:L1]"}`,
		},
		{"not json", `{ this is not json`, `{ this is not json`, `{ this is not json`},
		{"plain text", `this is not json`, `this is not json`, `this is not json`},
		{"an array", `[{"v":2,"build":"p"}]`, `[{"v":2,"build":"p"}]`, `[{"v":2,"build":"p"}]`},
		{"two objects on a line", `{"v":2,"build":"p"}{"v":2,"build":"p"}`, `{"v":2,"build":"p"}{"v":2,"build":"p"}`, `{"v":2,"build":"p"}{"v":2,"build":"p"}`},
		{"trailing garbage", `{"v":2,"build":"p"} x`, `{"v":2,"build":"p"} x`, `{"v":2,"build":"p"} x`},
		{"truncated object", `{"v":2,"build":"p"`, `{"v":2,"build":"p"`, `{"v":2,"build":"p"`},
		{"blank", ``, ``, ``},
		{"spaces only", `   `, `   `, `   `},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.line
			if got := string(DownLine([]byte(tc.line))); got != tc.down {
				t.Errorf("DownLine(%s)\n got %s\nwant %s", in, got, tc.down)
			}
			if got := string(UpLine([]byte(tc.line))); got != tc.up {
				t.Errorf("UpLine(%s)\n got %s\nwant %s", in, got, tc.up)
			}
		})
	}
}

func TestDownUpTextRoundTrip(t *testing.T) {
	text := `{"v":2,"event":"launch","id":"L1","build":"p","build_version":1}` + "\n" +
		"\n" +
		`{ this is not json` + "\n" +
		`{"v":2,"event":"end","id":"L1"}` + "\n" +
		`{"v":3,"event":"launch","build":"zzz"}` + "\n" +
		`{"v":2,"event":"launch","id":"L2","build":"q"}` // no final newline
	down := DownText([]byte(text))
	want := `{"v":1,"event":"launch","id":"L1","preset":"p","preset_version":1}` + "\n" +
		"\n" +
		`{ this is not json` + "\n" +
		`{"v":1,"event":"end","id":"L1"}` + "\n" +
		`{"v":3,"event":"launch","build":"zzz"}` + "\n" +
		`{"v":1,"event":"launch","id":"L2","preset":"q"}`
	if string(down) != want {
		t.Fatalf("DownText\n got %s\nwant %s", down, want)
	}
	if got := string(UpText(down)); got != text {
		t.Errorf("round trip\n got %s\nwant %s", got, text)
	}
	if len(DownText(nil)) != 0 {
		t.Errorf("DownText(nil) = %q", DownText(nil))
	}
}

func registryOf(t *testing.T, sb *sandbox.Sandbox) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sb.Project, ".jitso", "launches.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func newSandbox(t *testing.T) *sandbox.Sandbox {
	t.Helper()
	root := t.TempDir()
	sb := &sandbox.Sandbox{Root: root, Project: filepath.Join(root, "project")}
	if err := os.MkdirAll(filepath.Join(sb.Project, ".jitso"), 0o755); err != nil {
		t.Fatal(err)
	}
	return sb
}

func writeRegistry(t *testing.T, sb *sandbox.Sandbox, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(sb.Project, ".jitso", "launches.jsonl"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendRegistry(t *testing.T, sb *sandbox.Sandbox, text string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(sb.Project, ".jitso", "launches.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func TestAroundStep(t *testing.T) {
	ts := &TS{}
	seeded := `{"v":2,"event":"launch","id":"L1","build":"p","build_version":1}` + "\n" +
		`{"v":1,"event":"end","id":"Lold","preset":"kept"}` + "\n" +
		"{ not json\n"

	t.Run("seeded lines go down for the step and the case's bytes come back", func(t *testing.T) {
		sb := newSandbox(t)
		writeRegistry(t, sb, seeded)
		after, err := ts.AroundStep(sb)
		if err != nil {
			t.Fatal(err)
		}
		during := registryOf(t, sb)
		if !strings.Contains(during, `"preset":"p"`) || !strings.Contains(during, `"v":1,"event":"launch"`) {
			t.Errorf("seeded launch line was not turned into v1 for the oracle:\n%s", during)
		}
		// The oracle appends a v1 line and an end line.
		appendRegistry(t, sb, `{"v":1,"event":"end","id":"L1","source":"cli"}`+"\n"+`{"v":1,"event":"launch","id":"L2","preset":"q","preset_version":2}`+"\n")
		if err := after(); err != nil {
			t.Fatal(err)
		}
		want := seeded + `{"v":2,"event":"end","id":"L1","source":"cli"}` + "\n" + `{"v":2,"event":"launch","id":"L2","build":"q","build_version":2}` + "\n"
		if got := registryOf(t, sb); got != want {
			t.Errorf("after the step\n got %s\nwant %s", got, want)
		}
	})

	t.Run("a step that appends nothing leaves the case's bytes", func(t *testing.T) {
		sb := newSandbox(t)
		writeRegistry(t, sb, seeded)
		after, err := ts.AroundStep(sb)
		if err != nil {
			t.Fatal(err)
		}
		if err := after(); err != nil {
			t.Fatal(err)
		}
		if got := registryOf(t, sb); got != seeded {
			t.Errorf("got %s want %s", got, seeded)
		}
	})

	t.Run("a file the step creates is converted", func(t *testing.T) {
		sb := newSandbox(t)
		after, err := ts.AroundStep(sb)
		if err != nil {
			t.Fatal(err)
		}
		writeRegistry(t, sb, `{"v":1,"event":"launch","id":"L1","preset":"p","preset_version":1}`+"\n")
		if err := after(); err != nil {
			t.Fatal(err)
		}
		want := `{"v":2,"event":"launch","id":"L1","build":"p","build_version":1}` + "\n"
		if got := registryOf(t, sb); got != want {
			t.Errorf("got %s want %s", got, want)
		}
	})

	t.Run("no file before or after is nothing", func(t *testing.T) {
		sb := newSandbox(t)
		after, err := ts.AroundStep(sb)
		if err != nil {
			t.Fatal(err)
		}
		if err := after(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(sb.Project, ".jitso", "launches.jsonl")); err == nil {
			t.Error("the file appeared")
		}
	})

	t.Run("a rewritten file is a runner error", func(t *testing.T) {
		sb := newSandbox(t)
		writeRegistry(t, sb, seeded)
		after, err := ts.AroundStep(sb)
		if err != nil {
			t.Fatal(err)
		}
		writeRegistry(t, sb, `{"v":1,"event":"launch","id":"Lx","preset":"p"}`+"\n")
		if err := after(); err == nil || !strings.Contains(err.Error(), "rewrote") {
			t.Errorf("err = %v, want a rewrote error", err)
		}
	})

	t.Run("a removed file is a runner error", func(t *testing.T) {
		sb := newSandbox(t)
		writeRegistry(t, sb, seeded)
		after, err := ts.AroundStep(sb)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(sb.Project, ".jitso", "launches.jsonl")); err != nil {
			t.Fatal(err)
		}
		if err := after(); err == nil || !strings.Contains(err.Error(), "removed") {
			t.Errorf("err = %v, want a removed error", err)
		}
	})

	t.Run("a seeded v1 line stays v1 for the oracle and after the step", func(t *testing.T) {
		sb := newSandbox(t)
		v1 := `{"v":1,"event":"launch","id":"L1","preset":"p"}` + "\n"
		writeRegistry(t, sb, v1)
		after, err := ts.AroundStep(sb)
		if err != nil {
			t.Fatal(err)
		}
		if got := registryOf(t, sb); got != v1 {
			t.Errorf("v1 line changed for the oracle: %s", got)
		}
		appendRegistry(t, sb, `{"v":1,"event":"end","id":"L1","source":"cli"}`+"\n")
		if err := after(); err != nil {
			t.Fatal(err)
		}
		want := v1 + `{"v":2,"event":"end","id":"L1","source":"cli"}` + "\n"
		if got := registryOf(t, sb); got != want {
			t.Errorf("got %s want %s", got, want)
		}
	})
}

func TestBinAroundStepIsANoOp(t *testing.T) {
	sb := newSandbox(t)
	writeRegistry(t, sb, `{"v":2,"build":"p"}`+"\n")
	after, err := (&Bin{path: "x"}).AroundStep(sb)
	if err != nil {
		t.Fatal(err)
	}
	if err := after(); err != nil {
		t.Fatal(err)
	}
	if got := registryOf(t, sb); got != `{"v":2,"build":"p"}`+"\n" {
		t.Errorf("bin changed the registry: %s", got)
	}
}
