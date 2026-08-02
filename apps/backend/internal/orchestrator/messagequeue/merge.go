package messagequeue

import (
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/entityrefs"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

// joinMergeContent concatenates the source content below the target content,
// separated by a blank line. When the target content is empty the source is
// used verbatim so there is no leading blank line.
func joinMergeContent(targetContent, sourceContent string) string {
	if targetContent == "" {
		return sourceContent
	}
	return targetContent + "\n\n" + sourceContent
}

// ErrMergeReferenceOverflow reports a merge whose combined entity-reference
// lists would exceed entityrefs' per-message cap. The merge is rejected
// atomically — neither row is touched — rather than silently dropping
// references that were already persisted.
var ErrMergeReferenceOverflow = errors.New("merge would exceed the per-message entity reference limit")

// mergeAllowed reports whether the source entry may be folded into the target
// entry under the caller identity queuedBy. User entries merge only into user
// entries owned by the caller; agent entries merge only into agent entries
// produced by the same sender task; workflow/server/system sources and
// reserved in-flight targets are never mergeable.
func mergeAllowed(source, target *QueuedMessage, queuedBy string) bool {
	if target == nil || target.IsReservedInFlight() {
		return false
	}
	if source.QueuedBy == QueuedByAgent {
		if target.QueuedBy != QueuedByAgent {
			return false
		}
		sourceSender := metadataString(source.Metadata, MetadataSenderTaskID)
		return sourceSender != "" && sourceSender == metadataString(target.Metadata, MetadataSenderTaskID)
	}
	if IsReservedQueuedBy(source.QueuedBy) {
		return false
	}
	return queuedBy != "" && !IsReservedQueuedBy(queuedBy) &&
		source.QueuedBy == queuedBy && target.QueuedBy == queuedBy
}

// mergeEntryMetadata returns a copy of the target metadata with
// MetadataEntityReferences replaced by the union of both entries' references,
// normalized and deduplicated by canonical ref. The key is dropped when the
// union is empty. Returns ErrMergeReferenceOverflow when the union would
// exceed the per-message reference cap.
func mergeEntryMetadata(target, source map[string]interface{}) (map[string]interface{}, error) {
	merged := make(map[string]interface{}, len(target)+1)
	for key, value := range target {
		merged[key] = value
	}
	union, err := unionEntityReferences(target, source)
	if err != nil {
		return nil, err
	}
	if len(union) == 0 {
		delete(merged, MetadataEntityReferences)
		return merged, nil
	}
	merged[MetadataEntityReferences] = union
	return merged, nil
}

// unionEntityReferences returns the deduplicated union of two entries' entity
// reference lists, normalized from either their Go-struct or JSON-roundtripped
// storage form. When the union would exceed the per-message limit entityrefs
// enforces (MaxReferencesPerMessage), it returns ErrMergeReferenceOverflow so
// the caller can reject the merge atomically instead of silently dropping
// references that were already persisted.
func unionEntityReferences(targetMetadata, sourceMetadata map[string]interface{}) ([]apiv1.EntityReference, error) {
	seen := make(map[string]struct{})
	var union []apiv1.EntityReference
	for _, references := range [][]apiv1.EntityReference{
		entityrefs.NormalizePersisted(targetMetadata[MetadataEntityReferences]),
		entityrefs.NormalizePersisted(sourceMetadata[MetadataEntityReferences]),
	} {
		for _, ref := range references {
			if _, duplicate := seen[ref.Ref]; duplicate {
				continue
			}
			seen[ref.Ref] = struct{}{}
			union = append(union, ref)
		}
	}
	if len(union) > entityrefs.MaxReferencesPerMessage {
		return nil, fmt.Errorf("%w: at most %d references are allowed", ErrMergeReferenceOverflow, entityrefs.MaxReferencesPerMessage)
	}
	return union, nil
}
