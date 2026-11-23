package hook

import (
	"reflect"
	"strings"
	"testing"
)

func words(segment string) []string { return strings.Split(segment, " ") }

func TestName(t *testing.T) {
	for in, want := range map[string]string{
		"claude": "claude", "./claude": "claude", "/usr/local/bin/claude": "claude", "../x/claude": "claude", "~/bin/claude": "claude",
		`\claude`: "claude", `c\laude`: "claude", "CLAUDE": "claude", "Claude": "claude", "claude-code": "claude-code", "$C": "$c",
	} {
		if got := Name(in); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProgram(t *testing.T) {
	tests := []struct {
		segment string
		prog    string
		args    string
		inner   []string // each inner command as one string
	}{
		{"claude -p hi", "claude", "-p hi", nil},
		{"FOO=1 BAR=2 claude x", "claude", "x", nil},
		{"FOO=1", "", "", nil},
		{"if true", "true", "", nil},
		{"then claude", "claude", "", nil},
		{"! claude", "claude", "", nil},
		{"fi", "fi", "", nil},
		{"env claude", "claude", "", nil},
		{"env -i FOO=1 claude -p", "claude", "-p", nil},
		{"env -u HOME -C /x claude", "claude", "", nil},
		{"env -S claude -p x", "env", "-S claude -p x", []string{"claude -p x"}},
		{"env --split-string claude", "env", "--split-string claude", []string{"claude"}},
		{"command claude", "claude", "", nil},
		{"command -p claude", "claude", "", nil},
		{"command -v claude", "", "", nil},
		{"command -V claude", "", "", nil},
		{"command -pv claude", "", "", nil},
		{"exec claude", "claude", "", nil},
		{"exec -c -l claude", "claude", "", nil},
		{"exec -a me claude", "claude", "", nil},
		{"nohup claude", "claude", "", nil},
		{"time claude", "claude", "", nil},
		{"time -p claude", "claude", "", nil},
		{"time -f %e claude", "claude", "", nil},
		{"time -o out claude", "claude", "", nil},
		{"sudo claude", "claude", "", nil},
		{"sudo -u me claude", "claude", "", nil},
		{"sudo -E -u me -g wheel FOO=1 claude", "claude", "", nil},
		{"sudo -- claude", "claude", "", nil},
		{"nice -n 5 opencode", "opencode", "", nil},
		{"nice -5 opencode", "opencode", "", nil},
		{"timeout 10 claude", "claude", "", nil},
		{"timeout -s KILL -k 5 10 claude", "claude", "", nil},
		{"timeout --foreground 10s claude", "claude", "", nil},
		{"stdbuf -oL claude", "claude", "", nil},
		{"stdbuf -i 0 -o L claude", "claude", "", nil},
		{"xargs claude", "claude", "", nil},
		{"xargs -I{} claude {}", "claude", "{}", nil},
		{"xargs -I {} -n 1 claude {}", "claude", "{}", nil},
		{"xargs -0 -P 4 claude", "claude", "", nil},
		{"caffeinate -t 10 -w 3 claude", "claude", "", nil},
		{"sudo env nohup claude", "claude", "", nil},
		{"eval claude", "eval", "claude", []string{"claude"}},
		{"sh -c claude -p x", "sh", "-c claude -p x", []string{"claude -p x"}},
		{"bash -lc claude", "bash", "-lc claude", []string{"claude"}},
		{"zsh -ic claude", "zsh", "-ic claude", []string{"claude"}},
		{"bash -o pipefail -c claude", "bash", "-o pipefail -c claude", []string{"claude"}},
		{"bash --norc -ec claude", "bash", "--norc -ec claude", []string{"claude"}},
		{"sh x.sh claude", "sh", "x.sh claude", nil},
		{"sh /p/run.sh", "sh", "/p/run.sh", nil},
		{"sh", "sh", "", nil},
		{"/p/run.sh", "run.sh", "", nil},
		{"jitso launch --build claude", "jitso", "launch --build claude", nil},
		{"echo claude", "echo", "claude", nil},
		{"which claude", "which", "claude", nil},
		{"type opencode", "type", "opencode", nil},
		{"nohup", "", "", nil},
		{"timeout", "", "", nil},
		{"timeout 10", "", "", nil},
		{"env", "", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.segment, func(t *testing.T) {
			prog, args, inner := Program(words(tt.segment))
			if prog != tt.prog || strings.Join(args, " ") != tt.args {
				t.Errorf("Program = %q %q, want %q %q", prog, strings.Join(args, " "), tt.prog, tt.args)
			}
			var got []string
			for _, in := range inner {
				got = append(got, strings.Join(in, " "))
			}
			if !reflect.DeepEqual(got, tt.inner) {
				t.Errorf("inner = %q, want %q", got, tt.inner)
			}
		})
	}
}

func TestHarnessProgramDepth(t *testing.T) {
	nest := func(levels int) string {
		s := "claude"
		for range levels {
			s = "sh -c " + s
		}
		return s
	}
	for levels, want := range map[int]bool{0: true, 1: true, 7: true, 8: true, 9: false, 12: false} {
		if _, got := HarnessProgram(words(nest(levels))); got != want {
			t.Errorf("%d levels of sh -c: found %v, want %v", levels, got, want)
		}
	}
}

// The evasion matrix of the design: every input and the harness the guard
// names ("" when the line passes). The open rows are
// pinned too, so a change that closes one shows up here.
var harnessMatrixTests = []struct {
	command string
	want    string
}{
	{"claude -p hi", "claude"},
	{"codex exec x", "codex"},
	{"opencode run x", "opencode"},
	{"pi -p hi", "pi"},
	{"env pi", "pi"},
	{"PI", "pi"},
	{"./claude", "claude"},
	{"/usr/local/bin/claude --version", "claude"},
	{"../x/claude", "claude"},
	{"~/bin/claude", "claude"},
	{"claude --version", "claude"},
	{"claude mcp list", "claude"},
	{"codex --help", "codex"},
	{"sh -c 'claude -p x'", "claude"},
	{`bash -lc "claude"`, "claude"},
	{"env claude", "claude"},
	{"env -i FOO=1 claude", "claude"},
	{"command claude", "claude"},
	{"command -v claude", ""},
	{"which claude", ""},
	{"type claude", ""},
	{"exec claude", "claude"},
	{"nohup claude &", "claude"},
	{"time claude", "claude"},
	{"sudo -u me claude", "claude"},
	{"nice -n 5 opencode", "opencode"},
	{"timeout 10 claude", "claude"},
	{"$(claude)", "claude"},
	{"echo `claude`", "claude"},
	{"(claude)", "claude"},
	{"echo x | xargs claude", "claude"},
	{"xargs -I{} claude {}", "claude"},
	{"'claude'", "claude"},
	{`c"l"aude`, "claude"},
	{`c\laude`, "claude"},
	{`\claude`, "claude"},
	{"FOO=1 claude", "claude"},
	{"CLAUDE", "claude"},
	{"if true; then claude; fi", "claude"},
	{"eval claude", "claude"},
	{"git status && codex", "codex"},
	{"ls; claude", "claude"},
	// open, by design (section 3.4)
	{`ln -s "$(command -v claude)" /tmp/cc && /tmp/cc`, ""},
	{"C=claude; $C", ""},
	{"$(echo claude)", ""},
	{"{claude,}", ""},
	{"cla?de", ""},
	{"cla*", ""},
	{"npx @anthropic-ai/claude-code", ""},
	{"bunx @anthropic-ai/claude-code", ""},
	{"node /x/cli.js", ""},
	{"find . -exec claude {} ;", ""},
	{"tmux new -d claude", ""},
	{"screen -dm claude", ""},
	{"script -c claude", ""},
	{"watch claude", ""},
	{"parallel claude", ""},
	{`python3 -c "import subprocess as s; s.run(['claude'])"`, ""},
	{"sh x.sh", ""},
	{"./x.sh", ""},
	{`jitso "explain X"`, ""},
	{"orca terminal send --terminal h --text 'claude\\r'", ""},
	// only the program counts
	{"echo claude", ""},
	{"ls ~/.claude", ""},
	{"cat CLAUDE.md", ""},
	{"grep -r claude docs", ""},
	{"git log --author claude", ""},
	{"sh /p/.jitso/launches/L20261116T000000-abcd/run.sh", ""},
	{"'/p/.jitso/launches/L20261116T000000-abcd/run.sh'", ""},
	{"claude-code-notes --help", ""},
	{"which pi", ""},
	{"pip install x", ""},
	{"jitso launch --build claude --brief b.md --mode direct", ""},
	{"git status", ""},
	// text in a commit message or a heredoc body is data (J41); the old splitter denied these
	{"git commit -m 'fix (claude) bug'", ""},
	{"git commit -m 'see `claude` docs'", ""},
	{"cat <<'EOF'\nclaude rules\nEOF", ""},
}

func TestHarnessMatrix(t *testing.T) {
	tests := harnessMatrixTests
	l := Launcher{}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			d := l.BashDenial(tt.command)
			if d.Verb != tt.want {
				t.Errorf("BashDenial(%q) = %q, want %q", tt.command, d.Verb, tt.want)
			}
		})
	}
}

func TestHarnessReason(t *testing.T) {
	d := Launcher{}.BashDenial("claude -p hi")
	got := d.Reason(nil)
	want := "JITSO owns worker launch in this project: a harness starts only through jitso launch --build <id> --brief <file> --mode <mode>, never as a program of its own. If the word is text (a commit message, a heredoc line), put the text in a file and pass the file. (denied: claude)"
	if got != want {
		t.Errorf("reason %q, want %q", got, want)
	}
	if got := d.Reason([]byte(`{"scripts":{"jitso":"x"}}`)); !strings.Contains(got, "only through bun run jitso launch --build <id>") {
		t.Errorf("a project with a jitso script names bun run jitso: %q", got)
	}
}

// BuildSave, adoption and the launch verbs are judged before a harness program.
var harnessOrderTests = map[string]string{
	"jitso build save claude && claude":     VerbBuildSave,
	"jitso setup hook; claude":              VerbSetup,
	"orca agent launch claude":              "agent launch",
	"orca terminal create --command claude": "terminal create --command <harness>",
	"ls && claude && jitso setup hook":      "claude",
}

func TestHarnessDenialOrder(t *testing.T) {
	l := Launcher{}
	for command, want := range harnessOrderTests {
		if got := l.BashDenial(command).Verb; got != want {
			t.Errorf("BashDenial(%q) = %q, want %q", command, got, want)
		}
	}
}
