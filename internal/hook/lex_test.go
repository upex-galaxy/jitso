package hook

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// lexed is the commands of line as "word word | word", or "!" when it does
// not lex.
func lexed(line string) string {
	cmds, ok := lex(line)
	if !ok {
		return "!"
	}
	var out []string
	for _, c := range cmds {
		out = append(out, strings.Join(c.texts(), " "))
	}
	return strings.Join(out, " | ")
}

func TestLex(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"words", "ls  -la\tx", "ls -la x"},
		{"separators", "a; b && c || d | e & f\ng", "a | b | c | d | e | f | g"},
		{"single quotes keep everything", `echo 'a (b); $(c) "d"'`, `echo a (b); $(c) "d"`},
		{"double quotes", `echo "a 'b' \"c\" \$d \\e \f"`, `echo a 'b' "c" $d \e \f`},
		{"quotes join a word", `cl'au'"de" -p`, "claude -p"},
		{"backslash", `a\ b \c`, "a b c"},
		{"line continuation", "echo a\\\nb", "echo ab"},
		{"empty words are dropped", `jitso '' build ""`, "jitso build"},
		{"substitution is a command and stays in the word", `echo "x $(claude -p) y" z`, "echo x $(claude -p) y z | claude -p"},
		{"nested substitution", "echo $(a $(b) c)", "echo $(a $(b) c) | a $(b) c | b"},
		{"subshell parentheses inside a substitution", "echo $( (a) )", "echo $( (a) ) | a"},
		{"backtick", "echo `claude -p`", "echo `claude -p` | claude -p"},
		{"backtick in double quotes", "echo \"a `b c`\"", "echo a `b c` | b c"},
		{"escaped backtick in a backtick", "echo `a \\`b\\``", "echo `a \\`b\\`` | a `b` | b"},
		{"process substitution", "diff <(a x) >(b)", "diff <(a x) >(b) | a x | b"},
		{"subshell", "(a; b) && c", "a | b | c"},
		{"redirection keeps &", "a 2>&1 >/dev/null", "a 2>&1 >/dev/null"},
		{"quoted heredoc body is data", "cat <<'EOF'\nclaude $(x)\nEOF\nls", "cat << EOF | ls"},
		{"double-quoted delimiter", "cat <<\"E F\"\nclaude\nE F", "cat << E F"},
		{"backslash delimiter is quoted", "cat <<\\EOF\n$(x)\nEOF", "cat << EOF"},
		{"unquoted body has its substitutions", "cat <<EOF\na $(b c) `d`\n\\$(e)\nEOF", "cat << EOF | b c | d"},
		{"dash heredoc strips tabs", "cat <<-EOF\n\tclaude\n\tEOF\nls", "cat << EOF | ls"},
		{"two heredocs on a line", "cat <<A <<B\na\nA\nb\nB\nls", "cat << A << B | ls"},
		{"heredoc inside a substitution", "git commit -m \"$(cat <<'EOF'\nfix (claude)\nEOF\n)\"", "git commit -m $(cat <<'EOF'\nfix (claude)\nEOF\n) | cat << EOF"},
		{"here-string is not a heredoc", `bash <<< "claude -p"` + "\nls", "bash <<< claude -p | ls"},
		{"unbalanced single quote", "echo 'a", "!"},
		{"unbalanced double quote", `echo "a`, "!"},
		{"unbalanced backtick", "echo `a", "!"},
		{"unbalanced substitution", "echo $(a", "!"},
		{"a ) with no (", "case x in a) b;; esac", "!"},
		{"heredoc without its delimiter line", "cat <<EOF\nclaude", "!"},
		{"heredoc at the end of the line", "cat <<EOF", "!"},
		{"heredoc without a delimiter word", "cat << ;", "!"},
		{"no-break space separates words", "claude -p", "claude -p"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lexed(tt.in); got != tt.want {
				t.Errorf("lex(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDataPositions(t *testing.T) {
	tests := []struct {
		command string
		data    []int
		ok      bool
	}{
		{"git commit -m x", []int{3}, true},
		{"git commit -am x", []int{3}, true},
		{"git commit --message=x --amend", []int{2}, true},
		{"git tag -a v1 -m x", []int{5}, true},
		{"git commit -F - << EOF", nil, true},
		{"git commit --file=- << EOF", nil, true},
		{"git commit -m x << EOF", nil, false},
		{"git commit -c HEAD", nil, false},
		{"git commit -m x --exec y", nil, false},
		{"git -c a=b commit -m x", nil, false},
		{"git rebase --exec x", nil, false},
		{"git push", nil, false},
		{"orca orchestration send --to x --body y --subject z", []int{6, 8}, true},
		{"orca orchestration ask --question q", []int{4}, true},
		{"orca orchestration reply --id i --body=b", []int{5}, true},
		{"orca orchestration worker-start --run r", nil, false},
		{"orca orchestration send --body x << EOF", nil, false},
		{"gh pr create --title t --body b", []int{4, 6}, true},
		{"gh issue comment 1 -b x", []int{5}, true},
		{"gh release create v1 -n x", []int{5}, true},
		{"gh pr merge 1", nil, false},
		{"echo a b", []int{1, 2}, true},
		{"printf %s x", []int{1, 2}, true},
		{"echo << EOF", nil, false},
		{"echo <<< x", nil, false},
		{"cat << EOF", nil, true},
		{"cat > r.md << EOF", nil, true},
		{"cat >>r.md << EOF", nil, true},
		{"cat r.md", nil, false},
		{"cat << EOF r.md", nil, false},
		{"cat", nil, false},
		{"FOO=1 git commit -m x", nil, false},
		{"python3 -c x", nil, false},
		{"sh -c x", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			data, ok := dataPositions(words(tt.command))
			if ok != tt.ok || !reflect.DeepEqual(data, tt.data) {
				t.Errorf("dataPositions = %v %v, want %v %v", data, ok, tt.data, tt.ok)
			}
		})
	}
}

// What the lexer changes, beyond the old tables: each row is judged by the
// guard, and old says what the old splitter said, so a row that moves is
// visible here. Only quoted text and heredoc bodies of data commands move.
func TestLexedJudgment(t *testing.T) {
	l := Launcher{IsOwn: isOwnAt("/opt/jitso")}
	const save, setup, ws, harness = VerbBuildSave, VerbSetup, "orchestration worker-start", "claude"
	tests := []struct {
		command, want, old string
	}{
		// text in a data command
		{`orca orchestration send --to dispatch:x --body "ask the owner to run jitso build save cp-x"`, "", save},
		{`orca orchestration reply --id i --body 'run jitso setup hook'`, "", setup},
		{`git commit -m "$(cat <<'EOF'` + "\nfix (claude): jitso build save x\nEOF\n)\"", "", harness},
		{`gh pr create --title "x" --body "$(cat <<'EOF'` + "\nrun jitso setup hook\nEOF\n)\"", "", setup},
		{"git commit -F - <<'EOF'\norca orchestration worker-start is denied\nEOF", "", ws},
		{"cat > r.md <<EOF\n| w1 | claude --resume abc |\nEOF", "", harness},
		{"cat <<-EOF\n\tclaude\n\tEOF", "", harness},
		{"cat <<EOF\n\\$(claude -p)\nEOF", "", harness},
		{`echo "x" "jitso build save x"`, "", save},
		{`git commit -m "a" && git tag -m "jitso build save x" v1`, "", save},
		// text outside a data position is still read
		{`git commit -m "x" jitso build save y`, save, save},
		{`echo jitso build save x`, save, save},
		{`gh pr create --title x jitso setup hook`, setup, setup},
		// a substitution is a command, so it must be a data command too
		{`git commit -m "x $(claude -p hi)"`, harness, harness},
		{"cat <<EOF\n`claude -p`\nEOF", harness, harness},
		{`git commit -m "$(printf 'jitso build save x' | sh)"`, save, save},
		// every other program, anywhere on the line, keeps the old splitter
		{`python3 -c "jitso build save x"`, save, save},
		{`python3 -c "jitso setup hook"`, setup, setup},
		{`python3 -c "orca orchestration worker-start --run r"`, ws, ws},
		{`node -e "jitso build save x"`, save, save},
		{`echo "jitso build save x" | python3`, save, save},
		{`echo "jitso build save x" > a.sh && sh a.sh`, save, save},
		{`docker exec c sh -c "jitso build save x"`, save, save},
		{`busybox sh -c "jitso build save x"`, save, save},
		{`git -c alias.x='! jitso build save x' x`, save, save},
		{`git rebase --exec "jitso build save x"`, save, save},
		{`cd x && git commit -m "jitso build save x"`, save, save},
		{`FOO=1 git commit -m "jitso build save x"`, save, save},
		{`sh -c 'git commit -m "fix (claude)"'`, harness, harness},
		{`bash -c "git commit -m 'jitso build save notes'"`, save, save},
		{"read -r X <<'EOF'\njitso build save x\nEOF\n$X", save, save},
		{`echo 'jitso build save x' | xargs -I{} sh -c '{}'`, save, save},
		{`echo "jitso build save x" | cat`, save, save},
		{`cat <<< "jitso build save x"`, save, save},
		{`echo "jitso build save x" | git commit -F - && sh -c x`, save, save},
		{`'claude -p hi'`, harness, harness},
		{`orca "orchestration worker-start" --run r`, ws, ws},
		{`orca terminal create --title "a b" --command "claude --model x"`, "terminal create --command <harness>", "terminal create --command <harness>"},
		{`echo "$(case x in a) jitso build save x;; esac)"`, save, save},
		{"cat <<EOF\nclaude\n", harness, harness},
		{"jitso setup hook --dry-run", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			if got := l.BashDenial(tt.command).Verb; got != tt.want {
				t.Errorf("BashDenial(%q) = %q, want %q", tt.command, got, tt.want)
			}
			if got := l.segmentDenial(tt.command).Verb; got != tt.old {
				t.Errorf("old splitter (%q) = %q, want %q", tt.command, got, tt.old)
			}
		})
	}
}

// The lexer is one pass over the line: long and deeply nested lines finish
// fast, and nesting past maxNest goes to the old splitter.
func TestLexLongInput(t *testing.T) {
	l := Launcher{}
	for _, command := range []string{
		strings.Repeat("$(", 75000) + "claude",
		strings.Repeat("(", 150000) + "claude",
		strings.Repeat(`"a" `, 37500) + "&& claude",
		strings.Repeat("cat <<A ", 18000) + "\n" + strings.Repeat("A\n", 18000) + "claude",
		strings.Repeat("eval ", 30000) + "claude",
		strings.Repeat("sh -c ", 25000) + "claude",
		strings.Repeat("`", 150001),
		"cat <<EOF\n" + strings.Repeat("$(a) ", 30000) + "\nEOF\nclaude",
	} {
		start := time.Now()
		got := l.BashDenial(command).Verb
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("%.20q...: %v", command, d)
		}
		if strings.HasSuffix(command, "claude") && !strings.HasPrefix(command, "sh -c") && !strings.HasPrefix(command, "eval") && got != "claude" {
			t.Errorf("%.20q...: %q, want claude", command, got)
		}
	}
	if got := l.BashDenial(strings.Repeat("$(", maxNest+1) + "claude" + strings.Repeat(")", maxNest+1)).Verb; got != "claude" {
		t.Errorf("nesting past maxNest: %q, want claude from the old splitter", got)
	}
}
