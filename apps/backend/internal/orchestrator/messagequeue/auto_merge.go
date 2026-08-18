package messagequeue

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/messageconstraints"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type autoMergedValues struct {
	content     string
	attachments []MessageAttachment
	metadata    map[string]interface{}
	queuedAt    time.Time
}

func buildAutoMergedEntry(target, source *QueuedMessage) (*autoMergedValues, bool) {
	if !autoMergeAllowed(target, source) || !autoMetadataEquivalent(target.Metadata, source.Metadata) {
		return nil, false
	}
	attachments, ok := combineAutoMergeAttachments(target.Attachments, source.Attachments)
	if !ok {
		return nil, false
	}
	metadata, ok := mergeAutoMetadata(target.Metadata, source.Metadata)
	if !ok {
		return nil, false
	}
	return &autoMergedValues{
		content:     joinAutoMergeContent(target.Content, source.Content),
		attachments: attachments,
		metadata:    metadata,
		queuedAt:    latestQueuedAt(target.QueuedAt, source.QueuedAt),
	}, true
}

func latestQueuedAt(first, second time.Time) time.Time {
	if second.After(first) {
		return second
	}
	return first
}

func autoMergeAllowed(target, source *QueuedMessage) bool {
	if target == nil || source == nil || target.IsDurableLifecycle() || source.IsDurableLifecycle() {
		return false
	}
	if target.TaskID != source.TaskID || target.Model != source.Model || target.PlanMode != source.PlanMode {
		return false
	}
	if source.QueuedBy == QueuedByAgent {
		sourceSenderTaskID := metadataString(source.Metadata, MetadataSenderTaskID)
		return target.QueuedBy == QueuedByAgent &&
			sourceSenderTaskID != "" &&
			sourceSenderTaskID == metadataString(target.Metadata, MetadataSenderTaskID)
	}
	return source.QueuedBy != "" && source.QueuedBy == target.QueuedBy && !IsReservedQueuedBy(source.QueuedBy)
}

func joinAutoMergeContent(target, source string) string {
	if target == "" {
		return source
	}
	if source == "" {
		return target
	}
	return target + "\n\n" + source
}

func autoMetadataEquivalent(target, source map[string]interface{}) bool {
	targetJSON, targetOK := comparableAutoMetadata(target)
	sourceJSON, sourceOK := comparableAutoMetadata(source)
	return targetOK && sourceOK && bytes.Equal(targetJSON, sourceJSON)
}

func comparableAutoMetadata(metadata map[string]interface{}) ([]byte, bool) {
	comparable := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		if key == MetadataEntityReferences || key == MetadataContextFiles {
			continue
		}
		comparable[key] = value
	}
	encoded, err := json.Marshal(comparable)
	return encoded, err == nil
}

func combineAutoMergeAttachments(target, source []MessageAttachment) ([]MessageAttachment, bool) {
	if len(target)+len(source) > messageconstraints.MaxAttachmentCount {
		return nil, false
	}
	combined := append(append([]MessageAttachment(nil), target...), source...)
	var total int64
	for _, attachment := range combined {
		payloadBytes, err := attachmentPayloadBytes(attachment)
		if err != nil || payloadBytes > messageconstraints.MaxAttachmentBytes-total {
			return nil, false
		}
		total += payloadBytes
	}
	return combined, true
}

func mergeAutoMetadata(target, source map[string]interface{}) (map[string]interface{}, bool) {
	merged := copyMessageMetadata(target, 0)
	references, ok := unionAutoEntityReferences(target, source)
	if !ok {
		return nil, false
	}
	contexts, ok := unionAutoContextFiles(target, source)
	if !ok {
		return nil, false
	}
	merged = setAutoMetadataList(merged, MetadataEntityReferences, references)
	merged = setAutoMetadataList(merged, MetadataContextFiles, contexts)
	return merged, true
}

func setAutoMetadataList(metadata map[string]interface{}, key string, value interface{}) map[string]interface{} {
	encoded, _ := json.Marshal(value)
	if string(encoded) == "null" || string(encoded) == "[]" {
		delete(metadata, key)
	} else {
		if metadata == nil {
			metadata = make(map[string]interface{}, 1)
		}
		metadata[key] = value
	}
	return metadata
}

func unionAutoEntityReferences(target, source map[string]interface{}) ([]apiv1.EntityReference, bool) {
	var union []apiv1.EntityReference
	seen := make(map[string]struct{})
	for _, metadata := range []map[string]interface{}{target, source} {
		references, ok := normalizeAutoEntityReferences(metadata[MetadataEntityReferences])
		if !ok {
			return nil, false
		}
		for _, reference := range references {
			if _, duplicate := seen[reference.Ref]; duplicate {
				continue
			}
			seen[reference.Ref] = struct{}{}
			union = append(union, reference)
		}
	}
	return union, len(union) <= entityrefs.MaxReferencesPerMessage
}

func normalizeAutoEntityReferences(raw interface{}) ([]apiv1.EntityReference, bool) {
	if raw == nil {
		return nil, true
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var references []apiv1.EntityReference
	if err := json.Unmarshal(encoded, &references); err != nil {
		return nil, false
	}
	normalized, err := entityrefs.NormalizeForSubmission(references)
	return normalized, err == nil
}

func unionAutoContextFiles(target, source map[string]interface{}) ([]apiv1.ContextFileMeta, bool) {
	var union []apiv1.ContextFileMeta
	seen := make(map[string]struct{})
	for _, metadata := range []map[string]interface{}{target, source} {
		contexts, ok := normalizeAutoContextFiles(metadata[MetadataContextFiles])
		if !ok {
			return nil, false
		}
		for _, contextFile := range contexts {
			encoded, _ := json.Marshal(contextFile)
			key := string(encoded)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			union = append(union, contextFile)
		}
	}
	return union, len(union) <= messageconstraints.MaxContextFilesPerMessage
}

func normalizeAutoContextFiles(raw interface{}) ([]apiv1.ContextFileMeta, bool) {
	if raw == nil {
		return nil, true
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var contexts []apiv1.ContextFileMeta
	if err := json.Unmarshal(encoded, &contexts); err != nil {
		return nil, false
	}
	return contexts, true
}
