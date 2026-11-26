package fleet

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// InsideAny reports whether a worktree folder Orca names is one of the
// checkouts: the same file on disk, so a symlink, another separator or another
// letter case still matches, or the same cleaned path when a folder is gone.
// It is the one rule of which terminals belong to a project, for fleet and for
// graph.
func InsideAny(checkouts []string) func(string) bool {
	var infos []os.FileInfo
	clean := map[string]bool{}
	for _, c := range checkouts {
		clean[filepath.Clean(c)] = true
		if info, err := os.Stat(c); err == nil {
			infos = append(infos, info)
		}
	}
	return func(folder string) bool {
		if clean[filepath.Clean(folder)] {
			return true
		}
		info, err := os.Stat(folder)
		if err != nil {
			return false
		}
		for _, have := range infos {
			if os.SameFile(info, have) {
				return true
			}
		}
		return false
	}
}

// RepoName is the name of the repository that holds a worktree folder: the
// folder that holds the git common directory, so a linked worktree and a
// subfolder are named after their repository, or the common directory itself
// without ".git" when it is not a .git folder (a bare repository). When git
// cannot say, it is the name of the folder.
func RepoName(folder string) string {
	// git's stderr is discarded, so a failure prints nothing of its own.
	out, err := exec.Command("git", "-C", folder, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	common := strings.TrimRight(string(out), "\r\n")
	if err != nil || common == "" {
		return baseName(folder)
	}
	if baseName(common) == ".git" {
		return baseName(filepath.Dir(common))
	}
	return strings.TrimSuffix(baseName(common), ".git")
}

// baseName is the last element of a path, "" for none. Orca writes the path of
// the machine it runs on, so both separators count.
func baseName(path string) string {
	path = strings.TrimRight(path, `/\`)
	return path[strings.LastIndexAny(path, `/\`)+1:]
}
