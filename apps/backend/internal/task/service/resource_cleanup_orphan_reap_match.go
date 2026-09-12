package service

import (
	"path/filepath"
	"strings"
)

// orphanReapCandidate is one host process attributed to a reap root.
type orphanReapCandidate struct {
	hostProcess
	Root string
}

// attributeOrphanReapCandidates matches every snapshot process against the
// active reap roots and returns the matches grouped by root, attributing a
// candidate under more than one root to the longest (deepest) match.
// Matching uses working directory alone.
func attributeOrphanReapCandidates(snapshot []hostProcess, roots []string) map[string][]orphanReapCandidate {
	// Longest root first so the first match found is the deepest one.
	ordered := append([]string(nil), roots...)
	sortByDescendingLength(ordered)

	byRoot := make(map[string][]orphanReapCandidate)
	for _, proc := range snapshot {
		if proc.Cwd == "" {
			continue
		}
		for _, root := range ordered {
			if orphanReapPathWithinRoot(root, proc.Cwd) {
				byRoot[root] = append(byRoot[root], orphanReapCandidate{hostProcess: proc, Root: root})
				break
			}
		}
	}
	return byRoot
}

// orphanReapPathWithinRoot reports whether candidate is root itself or nested
// inside it on a whole path-component boundary, so a root of ".../task-a"
// never matches ".../task-abc".
func orphanReapPathWithinRoot(root, candidate string) bool {
	if root == "" || candidate == "" {
		return false
	}
	if candidate == root {
		return true
	}
	prefix := root
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(candidate, prefix)
}

func sortByDescendingLength(paths []string) {
	// Small N (reap roots per job), insertion sort keeps this dependency-free
	// and stable for equal-length paths.
	for i := 1; i < len(paths); i++ {
		for j := i; j > 0 && len(paths[j]) > len(paths[j-1]); j-- {
			paths[j], paths[j-1] = paths[j-1], paths[j]
		}
	}
}
