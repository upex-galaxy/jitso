package graph

import "strings"

// folderName is the last element of a worktree path, "" for none. Orca writes
// the path of the machine it runs on, so both separators count.
func folderName(path string) string {
	path = strings.TrimRight(path, `/\`)
	return path[strings.LastIndexAny(path, `/\`)+1:]
}
