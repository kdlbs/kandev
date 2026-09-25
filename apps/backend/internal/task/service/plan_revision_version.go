package service

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// PlanRevisionVersion returns the opaque snapshot token used by agent restore
// calls. It changes when coalescing changes any source field included here.
func PlanRevisionVersion(revision *models.TaskPlanRevision) string {
	if revision == nil {
		return ""
	}
	var canonical []byte
	for _, value := range []string{
		revision.ID,
		revision.TaskID,
		revision.Title,
		revision.Content,
		revision.UpdatedAt.UTC().Format(time.RFC3339Nano),
	} {
		canonical = appendLengthPrefixed(canonical, value)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}

func appendLengthPrefixed(dst []byte, value string) []byte {
	var length [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(length[:], uint64(len(value)))
	dst = append(dst, length[:n]...)
	return append(dst, value...)
}
