package impl

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOracleArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want []string
	}{
		{"launch with --build", []string{"launch", "--build", "plan", "--dry-run"}, []string{"launch", "--preset", "plan", "--dry-run"}},
		{"launch without --build", []string{"launch", "--dry-run"}, []string{"launch", "--dry-run"}},
		{"launch with --preset unchanged", []string{"launch", "--preset", "plan"}, []string{"launch", "--preset", "plan"}},
		{"launch --build after another flag", []string{"launch", "--field", "k=v", "--build", "plan"}, []string{"launch", "--field", "k=v", "--preset", "plan"}},
		{"launch value equal to --build", []string{"launch", "--field", "--build", "--build", "plan"}, []string{"launch", "--field", "--preset", "--preset", "plan"}},
		{"launch word that only contains --build", []string{"launch", "--build=plan", "x--build"}, []string{"launch", "--build=plan", "x--build"}},
		{"build list", []string{"build", "list"}, []string{"presets", "list"}},
		{"build list extra", []string{"build", "list", "extra"}, []string{"presets", "list", "extra"}},
		{"presets list unchanged", []string{"presets", "list"}, []string{"presets", "list"}},
		{"build alone unchanged", []string{"build"}, []string{"build"}},
		{"build other unchanged", []string{"build", "validate"}, []string{"build", "validate"}},
		{"--build outside launch untouched", []string{"analyze", "--build", "x"}, []string{"analyze", "--build", "x"}},
		{"build list not first untouched", []string{"help", "build", "list"}, []string{"help", "build", "list"}},
		{"no args", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := append([]string(nil), tc.in...)
			got := oracleArgs(tc.in)
			if !reflect.DeepEqual(got, tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
				t.Errorf("oracleArgs(%q) = %q, want %q", in, got, tc.want)
			}
			if !reflect.DeepEqual(tc.in, in) && !(len(in) == 0 && len(tc.in) == 0) {
				t.Errorf("input was modified: %q, was %q", tc.in, in)
			}
		})
	}
}

func TestLinkPresets(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dirs     []string
		files    []string
		wantLink bool
	}{
		{"builds present, presets absent", []string{"jitso/builds"}, nil, true},
		{"presets exists as a folder", []string{"jitso/builds", "jitso/presets"}, nil, false},
		{"builds absent", []string{"jitso"}, nil, false},
		{"builds is a file", nil, []string{"jitso/builds"}, false},
		{"no jitso folder", nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := t.TempDir()
			for _, d := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(proj, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for _, f := range tc.files {
				if err := os.MkdirAll(filepath.Join(proj, filepath.Dir(f)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(proj, f), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := linkPresets(proj); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(proj, "jitso", "presets")
			info, err := os.Lstat(link)
			if !tc.wantLink {
				if err == nil && info.Mode()&os.ModeSymlink != 0 {
					t.Fatalf("unexpected symlink at %s", link)
				}
				return
			}
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("want a symlink at %s, got %v, %v", link, info, err)
			}
			if target, _ := os.Readlink(link); target != "builds" {
				t.Errorf("link target = %q, want the relative %q", target, "builds")
			}
			// a build written afterwards is seen through the link
			if err := os.WriteFile(filepath.Join(proj, "jitso", "builds", "late.yaml"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(link, "late.yaml")); err != nil {
				t.Errorf("a later build is not visible through the link: %v", err)
			}
		})
	}
}
