package worktree

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
)

const cleanupBranchRefFormat = "--format=%(refname)%00%(objectname)%00%(objecttype)"

func (m *Manager) captureCleanupBranchOID(
	ctx context.Context, repositoryPath, branchRef string,
) (string, bool, error) {
	output, err := m.runBoundedGitInspect(
		ctx, repositoryPath, "for-each-ref", cleanupBranchRefFormat, branchRef,
	)
	if err != nil {
		return "", false, fmt.Errorf("inspect local branch %q: %w", branchRef, err)
	}
	return parseCleanupBranchRefOutput(output, branchRef)
}

func parseCleanupBranchRefOutput(output, wantedRef string) (string, bool, error) {
	if output == "" {
		return "", false, nil
	}
	lines := strings.Split(output, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return "", false, nil
	}

	var oid string
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		fields := strings.Split(line, "\x00")
		if len(fields) != 3 || !strings.HasPrefix(fields[0], "refs/") || fields[1] == "" || fields[2] == "" {
			return "", false, fmt.Errorf("malformed local branch inspection output")
		}
		// Validate every returned line for fail-closed safety; skip non-matching refs.
		if fields[2] != "commit" {
			return "", false, fmt.Errorf("local branch %q resolved to object type %q", fields[0], fields[2])
		}
		if !validCleanupCommitOID(fields[1]) {
			return "", false, fmt.Errorf("local branch %q returned an invalid commit %q", fields[0], fields[1])
		}
		if fields[0] != wantedRef {
			continue
		}
		if oid != "" {
			return "", false, fmt.Errorf("local branch %q was returned more than once", wantedRef)
		}
		oid = fields[1]
	}
	return oid, oid != "", nil
}

func parseCleanupCommitOID(output string) (string, error) {
	output = strings.TrimSuffix(output, "\n")
	output = strings.TrimSuffix(output, "\r")
	if !validCleanupCommitOID(output) {
		return "", fmt.Errorf("returned invalid commit identity")
	}
	return output, nil
}

func validCleanupCommitOID(oid string) bool {
	if len(oid) != 40 && len(oid) != 64 {
		return false
	}
	_, err := hex.DecodeString(oid)
	return err == nil
}
