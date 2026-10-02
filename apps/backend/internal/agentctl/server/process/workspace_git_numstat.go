package process

import (
	"strconv"
	"strings"
)

// parseWorkspaceNumstatZ consumes one git diff --numstat -z entry. An empty
// path column introduces separate old and new NUL-terminated path records.
func parseWorkspaceNumstatZ(output string) (numstatEntry, string, bool) {
	record, rest, ok := strings.Cut(output, "\x00")
	if !ok {
		return numstatEntry{}, "", false
	}
	parts := strings.SplitN(record, "\t", 3)
	if len(parts) != 3 {
		return numstatEntry{}, "", false
	}
	path := parts[2]
	if path == "" {
		oldPath, afterOld, foundOld := strings.Cut(rest, "\x00")
		newPath, afterNew, foundNew := strings.Cut(afterOld, "\x00")
		if !foundOld || !foundNew || oldPath == "" || newPath == "" {
			return numstatEntry{}, "", false
		}
		path, rest = newPath, afterNew
	}
	additions, _ := strconv.Atoi(parts[0])
	deletions, _ := strconv.Atoi(parts[1])
	return numstatEntry{path: path, additions: additions, deletions: deletions}, rest, true
}
