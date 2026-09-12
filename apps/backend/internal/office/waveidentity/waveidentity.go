// Package waveidentity derives the completion-wave identity for a
// task_children_completed wake (docs/specs/office/system-design/
// parent-wake-wave-identity.md, "Wave identity"). Both exported functions
// are pure: given the same parent id and member id slice they always
// return the same value, do no I/O, and never fail. Callers are
// responsible for restricting memberIDs to wave members (not archived, not
// ephemeral, not automation-origin) and ordering them ascending by
// tasks.id before calling either function.
package waveidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// waveKeyPrefix identifies the run reason a wave key belongs to, matching
// runs.reason for every producer of this wake.
const waveKeyPrefix = "task_children_completed"

// WaveString is the canonical encoding of a completion wave: the parent id
// and its wave members' ids, ascending by tasks.id, joined by "|" and ",".
// The separator is "|", not a NUL byte, because PostgreSQL text columns
// cannot store U+0000; task ids are lowercase-hex UUIDs, so "|" cannot
// occur inside one. A parent with no wave members still produces a
// well-formed string ("<parentID>|"), but per the requirements'
// Terminology that value is a pre-gate intermediate, not a wave string —
// callers must not persist or key on it for an empty memberIDs slice.
func WaveString(parentID string, memberIDs []string) string {
	return parentID + "|" + strings.Join(memberIDs, ",")
}

// WaveKey is the fixed-length digest encoding of the same identity
// WaveString names, indexed by the run queue's uniqueness constraint.
// It is a pure function of WaveString's output: equal wave strings always
// yield equal keys. The parent id appears again as a readable prefix so a
// stored key stays diagnosable, and so two parents with an identical
// member set never collide.
func WaveKey(parentID string, memberIDs []string) string {
	sum := sha256.Sum256([]byte(WaveString(parentID, memberIDs)))
	return waveKeyPrefix + ":" + parentID + ":" + hex.EncodeToString(sum[:])
}
