package brief

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const good = "## Goal\n\nFind where the sync entry point of this project is defined and explain in a few sentences how it is wired to the rest of the code.\n\n" +
	"## Scope\n\nRead only. Look at `src/sync.ts` and `docs/guide.md` and nothing outside the repo. Do not change any file.\n\n" +
	"## Done when\n\nYou can name the file and the line that defines the entry point, and you have checked it by reading the file.\n\n" +
	"## Report format\n\nOne short paragraph with the file path, the line number and a one line summary of what it does.\n"

func existing(paths ...string) func(string) bool {
	return func(p string) bool {
		for _, q := range paths {
			if p == q {
				return true
			}
		}
		return false
	}
}

func TestCheck(t *testing.T) {
	all := existing("src/sync.ts", "docs/guide.md")
	tests := []struct {
		name     string
		text     string
		language string
		exists   func(string) bool
		want     []Failure
	}{
		{"good", good, LanguageEnglish, all, nil},
		{"ghost path", good + "\nAlso `src/ghost/x.ts` and `src/ghost/x.ts`.\n", LanguageEnglish, all,
			[]Failure{{CheckPaths, "cited path does not exist: src/ghost/x.ts"}}},
		{"declared output exempts the path everywhere", "Output: `new/out.md`\n" + good + "\nSee `new/out.md`.\n", LanguageEnglish, all, nil},
		{"Output to does not declare", "Output to: `new/out.md`\n" + good, LanguageEnglish, all,
			[]Failure{{CheckPaths, "cited path does not exist: new/out.md"}}},
		{"ignored tokens", good + "`https://x.test/a/b` `~/a/b` `$HOME/a` `src/**/*.ts` `<d>/f.ts` `{a}/b` `ghost.ts`\n", LanguageEnglish, all, nil},
		{"only file-shaped tokens are checked", good + "`docs/foundation` `/project-foundation` `src/app/` `@platform/kit/line-chart` `@scope/pkg/missing.ts` `release/v0.1.2`\n", LanguageEnglish, all, nil},
		{"file-shaped ghosts are rejected", good + "`docs/foundation/plan.md` `.github/workflows/ghost.yml` `src/.env.ghost`\n", LanguageEnglish, all,
			[]Failure{{CheckPaths, "cited path does not exist: docs/foundation/plan.md, .github/workflows/ghost.yml, src/.env.ghost"}}},
		{"missing sections in order", strings.Replace(strings.Replace(good, "## Scope", "Scope", 1), "## Report format", "## Reports", 1), LanguageEnglish, all,
			[]Failure{{CheckSections, "missing section: Scope, Report format"}}},
		{"heading forms", strings.NewReplacer("## Goal", "######   GOAL  :  ", "## Scope", "#scope:").Replace(good), LanguageEnglish, all, nil},
		{"long s is not s", strings.Replace(good, "## Scope", "## ſcope", 1), LanguageEnglish, all,
			[]Failure{{CheckSections, "missing section: Scope"}}},
		{"too short", "## Goal\n## Scope\n## Done when\n## Report format\nthe a an of to and\n", LanguageEnglish, all,
			[]Failure{{CheckLength, "6 words, expected 40 to 800"}}},
		{"spanish rejected in en", spanishBrief, LanguageEnglish, all, []Failure{{CheckLanguage, "the brief must be written in English"}}},
		{"spanish accepted in es", spanishBrief, LanguageSpanish, all, nil},
		{"any skips language only", spanishBrief + "`x/ghost.ts`\n", LanguageAny, all, []Failure{{CheckPaths, "cited path does not exist: x/ghost.ts"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Check(tt.text, tt.language, tt.exists); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %v\nwant %v", got, tt.want)
			}
		})
	}
}

const spanishBrief = "## Goal\n\nEncuentra el punto de entrada de la sincronización del proyecto y explica en pocas frases cómo se conecta con el resto del código.\n\n" +
	"## Scope\n\nSolo lectura. Mira el archivo y la guía, y nada fuera del repositorio. No cambies ningún archivo de la carpeta.\n\n" +
	"## Done when\n\nPuedes nombrar el archivo y la línea que define el punto de entrada, y lo comprobaste leyendo el archivo.\n\n" +
	"## Report format\n\nUn párrafo corto con la ruta, el número de línea y un resumen de una línea de lo que hace.\n"

func TestCountWords(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"plain words", "one two  three\tfour\nfive", 5},
		{"a cited path is one word", "`src/sync.ts`", 1},
		{"a path with underscores and dashes", "foo/bar-baz_qux", 1},
		{"a contraction", "don't", 1},
		{"an abbreviation", "e.g.", 1},
		{"accented letters", "na\u00efve se\u00f1al", 2},
		{"numbers are not words", "3.14 (7) 2026-11-15", 0},
		{"a number with a letter is a word", "v2 3x", 2},
		{"a mid-line hash is not a heading", "see #9 and #Goal", 3},
		{"a heading line adds nothing", "# Done when\nreal words here", 3},
		{"a heading without a space", "#Goal\ntwo words", 2},
		{"every heading depth up to six", "# a\n## b\n### c\n#### d\n##### e\n###### f\nx", 1},
		{"seven hashes are not a heading", "####### Goal", 1},
		{"an empty heading line", "#\none", 1},
		{"a heading with CRLF", "# Goal\r\nfind it\r\n", 2},
		{"an indented hash is not a heading, and a lone # is no word", "  # Goal", 1},
		{"a no-break space separates", "one\u00a0two", 2},
		{"a zero width space does not", "one\u200btwo", 1},
		{"a punctuation-only token", "- * -- ... | one", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countWords(tt.text); got != tt.want {
				t.Errorf("countWords(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

// TestCheckCountsWordsNotTokens pins defect N-27: the length
// check reads words, while the language check keeps its letter runs.
func TestCheckCountsWordsNotTokens(t *testing.T) {
	all := existing("src/sync.ts", "docs/guide.md")
	if got := countWords(good); got < MinWords || got > MaxWords {
		t.Fatalf("the good brief has %d words", got)
	}
	// Four headings and 39 plain words, two of them cited paths.
	body := "## Goal\nFind the sync entry point in `src/sync.ts` and then read `docs/guide.md` for context.\n" +
		"## Scope\nRead only. Do not change any file.\n" +
		"## Done when\nThe file is named and the answer is one short line for the reader.\n" +
		"## Report format\nOne line with the path.\n"
	want := []Failure{{CheckLength, "39 words, expected 40 to 800"}}
	if got := Check(body, LanguageEnglish, all); !reflect.DeepEqual(got, want) {
		t.Errorf("39 words: got %v", got)
	}
	if got := Check(body+"more\n", LanguageEnglish, all); got != nil {
		t.Errorf("40 words: got %v", got)
	}
}

func TestReason(t *testing.T) {
	got := Reason([]Failure{{CheckSections, "missing section: Goal"}, {CheckPaths, "cited path does not exist: a/b"}})
	if want := "brief failed: sections (missing section: Goal); paths (cited path does not exist: a/b). " + pathsHint; got != want {
		t.Errorf("got %q", got)
	}
	if got := Reason([]Failure{{CheckSections, "missing section: Goal"}}); strings.Contains(got, "Output:") {
		t.Errorf("hint without a paths failure: %q", got)
	}
}

func TestResolvePath(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	briefs := filepath.Join(root, "p", ".jitso", "briefs")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{briefs, outside, filepath.Join(briefs, "sub"), filepath.Join(root, "p", ".jitso", "briefs-evil")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path string) {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(briefs, "b.md"))
	write(filepath.Join(outside, "b.md"))
	write(filepath.Join(root, "p", ".jitso", "secret.md"))
	write(filepath.Join(root, "p", ".jitso", "briefs-evil", "b.md"))
	link := func(target, name string) {
		if err := os.Symlink(target, name); err != nil {
			t.Fatal(err)
		}
	}
	link(filepath.Join(briefs, "b.md"), filepath.Join(briefs, "alias.md"))
	link(filepath.Join(outside, "b.md"), filepath.Join(briefs, "escape.md"))
	link(outside, filepath.Join(briefs, "away"))
	// briefs/deep -> outside/deep, so briefs/deep/../b.md is outside/b.md for realpath.
	if err := os.MkdirAll(filepath.Join(outside, "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	link(filepath.Join(outside, "deep"), filepath.Join(briefs, "deep"))

	allowed := []string{filepath.Join(root, "missing", "briefs"), briefs}
	tests := []struct {
		name, file, cwd, want, reason string
	}{
		{"inside", filepath.Join(briefs, "b.md"), "/", filepath.Join(briefs, "b.md"), ""},
		{"relative from the cwd", "briefs/b.md", filepath.Join(root, "p", ".jitso"), filepath.Join(briefs, "b.md"), ""},
		{"dotdot staying inside", filepath.Join(briefs, "sub") + "/../b.md", "/", filepath.Join(briefs, "b.md"), ""},
		{"symlink inside", filepath.Join(briefs, "alias.md"), "/", filepath.Join(briefs, "b.md"), ""},
		{"dotdot out", briefs + "/../secret.md", "/", "", reasonOutside},
		{"symlink to a file outside", filepath.Join(briefs, "escape.md"), "/", "", reasonOutside},
		{"symlinked folder outside", filepath.Join(briefs, "away", "b.md"), "/", "", reasonOutside},
		{"dotdot after a symlinked folder climbs from its target", filepath.Join(briefs, "deep") + "/../b.md", "/", "", reasonOutside},
		{"relative dotdot after a symlinked folder", "deep/../b.md", briefs, "", reasonOutside},
		{"sibling prefix", filepath.Join(root, "p", ".jitso", "briefs-evil", "b.md"), "/", "", reasonOutside},
		{"directory", briefs, "/", "", reasonNotRegular},
		{"missing", filepath.Join(briefs, "nope.md"), "/", "", reasonMissing},
		{"empty", "", "/", "", reasonEmpty},
		{"NUL", "a\x00b", "/", "", reasonEmpty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePath(tt.file, tt.cwd, allowed)
			if tt.reason == "" {
				if err != nil || got != tt.want {
					t.Fatalf("got %q, %v; want %q", got, err, tt.want)
				}
				return
			}
			if err == nil || err.Error() != "JITSO REJECTED: "+tt.reason {
				t.Fatalf("got %q, %v; want rejection %q", got, err, tt.reason)
			}
		})
	}
}

func TestExists(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	exists := Exists(root)
	for path, want := range map[string]bool{"a.txt": true, filepath.Join(root, "a.txt"): true, "b.txt": false, "dangling": false} {
		if exists(path) != want {
			t.Errorf("exists(%q) != %v", path, want)
		}
	}
}
