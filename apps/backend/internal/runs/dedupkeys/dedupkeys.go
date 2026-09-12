// Package dedupkeys centralizes the shared dedup-key builders that more than
// one office producer must derive identically for the same occurrence
// (docs/specs/office/system-design/run-dedup-generation-01.md#convergent-producers,
// #the-shared-key-builders). Placed beside internal/runs/commentkeys as a
// sibling package so the office service, the office scheduler and the
// orchestrator can all reach it without an import cycle.
package dedupkeys

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// AssignmentKey builds the canonical task_assigned dedup key. Both
// task_assigned producers (office/service's queueTaskAssignedRun and
// office/scheduler's reactToAssigneeChange) call this rather than
// formatting the string themselves, which is what makes their convergence
// structural instead of coincidental (AC-OFFICE-RUN-DEDUP-002.1).
func AssignmentKey(taskID, agentProfileID string, generation int64) string {
	return fmt.Sprintf("task_assigned:%s:%s:%d", taskID, agentProfileID, generation)
}

// BlockerDigest computes the shared digest over a blocker-task-id set. Both
// blocker producers (office/service's resolveAndWakeIfUnblocked and
// office/scheduler's cascadeBlockersResolved) call this so a set observed in
// different read order still converges on the same digest. The encoding is
// binding: sort ascending by byte value (NOT ListTaskBlockers's created_at,
// which is not unique), join with a single comma, then SHA-256 the UTF-8
// bytes and render the full digest as lowercase hex.
func BlockerDigest(blockerTaskIDs []string) string {
	sorted := make([]string, len(blockerTaskIDs))
	copy(sorted, blockerTaskIDs)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, ",")))
	return fmt.Sprintf("%x", sum)
}
