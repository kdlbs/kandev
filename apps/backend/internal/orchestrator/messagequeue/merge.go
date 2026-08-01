package messagequeue

import (
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
	return queuedBy != "" && !IsReservedQueuedBy(queuedBy) && target.QueuedBy == queuedBy
}

// mergeEntryMetadata returns a copy of the target metadata with
// MetadataEntityReferences replaced by the union of both entries' references,
// normalized and deduplicated by canonical ref. The key is dropped when the
// union is empty.
func mergeEntryMetadata(target, source map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(target)+1)
	for key, value := range target {
		merged[key] = value
	}
	union := unionEntityReferences(target, source)
	if len(union) == 0 {
		delete(merged, MetadataEntityReferences)
		return merged
	}
	merged[MetadataEntityReferences] = union
	return merged
}

// unionEntityReferences returns the deduplicated union of two entries' entity
// reference lists, normalized from either their Go-struct or JSON-roundtripped
// storage form.
func unionEntityReferences(targetMetadata, sourceMetadata map[string]interface{}) []apiv1.EntityReference {
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
	return union
}
