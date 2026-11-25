package graph

import (
	"os"
	"path/filepath"
	"strings"
)

// folderName is the last element of a worktree path, "" for none. Orca writes
// the path of the machine it runs on, so both separators count.
func folderName(path string) string {
	path = strings.TrimRight(path, `/\`)
	return path[strings.LastIndexAny(path, `/\`)+1:]
}

// insideAny reports whether a worktree folder Orca names is one of the
// checkouts: the same file on disk, so a symlink, another separator or another
// letter case still matches, or the same cleaned path when a folder is gone.
func insideAny(checkouts []string) func(string) bool {
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
