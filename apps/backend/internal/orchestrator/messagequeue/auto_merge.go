package messagequeue

import (
	"bytes"
	"encoding/json"
	"strings"
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
	if !autoMergeCompatible(target, source) {
		return nil, false
	}
	merge, ok := newAutoMergeAccumulator(target)
	if !ok || !merge.add(source) {
		return nil, false
	}
	return merge.values(), true
}

// autoMergeAccumulator folds compatible sources into one target. Every input
// is validated once and its additive lists are kept as ordered sets, so a
// chain of N folds costs time linear in its inputs rather than re-copying and
// re-validating the growing survivor on each fold.
type autoMergeAccumulator struct {
	target          *QueuedMessage
	comparable      []byte
	bodies          []string
	attachments     []MessageAttachment
	attachmentBytes int64
	references      []apiv1.EntityReference
	referenceSeen   map[string]struct{}
	contexts        []apiv1.ContextFileMeta
	contextSeen     map[string]struct{}
	admissionIDs    []string
	admissionSeen   map[string]struct{}
	queuedAt        time.Time
}

func newAutoMergeAccumulator(target *QueuedMessage) (*autoMergeAccumulator, bool) {
	if target == nil {
		return nil, false
	}
	comparable, ok := comparableAutoMetadata(target.Metadata)
	if !ok {
		return nil, false
	}
	merge := &autoMergeAccumulator{
		target: target, comparable: comparable, queuedAt: target.QueuedAt,
		referenceSeen: map[string]struct{}{}, contextSeen: map[string]struct{}{}, admissionSeen: map[string]struct{}{},
	}
	return merge, merge.absorb(target)
}

// autoMergeCompatible checks identity and non-additive metadata only, so an
// incompatible pair is rejected before any attachment payload is decoded.
func autoMergeCompatible(target, source *QueuedMessage) bool {
	if !autoMergeAllowed(target, source) {
		return false
	}
	targetComparable, ok := comparableAutoMetadata(target.Metadata)
	if !ok {
		return false
	}
	sourceComparable, ok := comparableAutoMetadata(source.Metadata)
	return ok && bytes.Equal(targetComparable, sourceComparable)
}

// add folds source into the accumulator when it is compatible and every
// combined limit still holds. A rejected source leaves the state unchanged.
func (a *autoMergeAccumulator) add(source *QueuedMessage) bool {
	if !autoMergeAllowed(a.target, source) {
		return false
	}
	comparable, ok := comparableAutoMetadata(source.Metadata)
	if !ok || !bytes.Equal(a.comparable, comparable) {
		return false
	}
	return a.absorb(source)
}

func (a *autoMergeAccumulator) absorb(message *QueuedMessage) bool {
	attachmentBytes, ok := a.incomingAttachmentBytes(message.Attachments)
	if !ok {
		return false
	}
	references, ok := normalizeAutoEntityReferences(message.Metadata[MetadataEntityReferences])
	if !ok {
		return false
	}
	references = newOrderedItems(references, a.referenceSeen, func(reference apiv1.EntityReference) string { return reference.Ref })
	if len(a.references)+len(references) > entityrefs.MaxReferencesPerMessage {
		return false
	}
	contexts, ok := normalizeAutoContextFiles(message.Metadata[MetadataContextFiles])
	if !ok {
		return false
	}
	contexts = newOrderedItems(contexts, a.contextSeen, contextFileKey)
	if len(a.contexts)+len(contexts) > messageconstraints.MaxContextFilesPerMessage {
		return false
	}
	admissionIDs, ok := normalizeQueueAdmissionIDs(message.Metadata[MetadataQueueAdmissionIDs])
	if !ok {
		return false
	}
	admissionIDs = newOrderedItems(admissionIDs, a.admissionSeen, func(id string) string { return id })

	a.attachments = append(a.attachments, message.Attachments...)
	a.attachmentBytes += attachmentBytes
	a.references = appendOrderedItems(a.references, references, a.referenceSeen, func(reference apiv1.EntityReference) string { return reference.Ref })
	a.contexts = appendOrderedItems(a.contexts, contexts, a.contextSeen, contextFileKey)
	a.admissionIDs = appendOrderedItems(a.admissionIDs, admissionIDs, a.admissionSeen, func(id string) string { return id })
	if message.Content != "" {
		a.bodies = append(a.bodies, message.Content)
	}
	a.queuedAt = latestQueuedAt(a.queuedAt, message.QueuedAt)
	return true
}

func (a *autoMergeAccumulator) incomingAttachmentBytes(attachments []MessageAttachment) (int64, bool) {
	if len(a.attachments)+len(attachments) > messageconstraints.MaxAttachmentCount {
		return 0, false
	}
	var incoming int64
	for _, attachment := range attachments {
		payloadBytes, err := attachmentPayloadBytes(attachment)
		if err != nil || payloadBytes > messageconstraints.MaxAttachmentBytes-a.attachmentBytes-incoming {
			return 0, false
		}
		incoming += payloadBytes
	}
	return incoming, true
}

// values materializes the merged entry once; the target's other metadata is
// copied and the additive lists are replaced by their ordered unions.
func (a *autoMergeAccumulator) values() *autoMergedValues {
	metadata := copyMessageMetadata(a.target.Metadata, 0)
	metadata = setAutoMetadataList(metadata, MetadataEntityReferences, a.references)
	metadata = setAutoMetadataList(metadata, MetadataContextFiles, a.contexts)
	metadata = setAutoMetadataList(metadata, MetadataQueueAdmissionIDs, a.admissionIDs)
	return &autoMergedValues{
		content:     strings.Join(a.bodies, "\n\n"),
		attachments: append([]MessageAttachment(nil), a.attachments...),
		metadata:    metadata,
		queuedAt:    a.queuedAt,
	}
}

// newOrderedItems returns the items of incoming not yet in seen, deduplicated
// among themselves, in their original order. seen is not modified.
func newOrderedItems[T any](incoming []T, seen map[string]struct{}, key func(T) string) []T {
	var fresh []T
	local := map[string]struct{}{}
	for _, item := range incoming {
		itemKey := key(item)
		if _, duplicate := seen[itemKey]; duplicate {
			continue
		}
		if _, duplicate := local[itemKey]; duplicate {
			continue
		}
		local[itemKey] = struct{}{}
		fresh = append(fresh, item)
	}
	return fresh
}

func appendOrderedItems[T any](items, fresh []T, seen map[string]struct{}, key func(T) string) []T {
	for _, item := range fresh {
		seen[key(item)] = struct{}{}
	}
	return append(items, fresh...)
}

func contextFileKey(contextFile apiv1.ContextFileMeta) string {
	encoded, _ := json.Marshal(contextFile)
	return string(encoded)
}

func latestQueuedAt(first, second time.Time) time.Time {
	if second.After(first) {
		return second
	}
	return first
}

func autoMergeAllowed(target, source *QueuedMessage) bool {
	if target == nil || source == nil || isManagedInputQueueEntry(target) || isManagedInputQueueEntry(source) ||
		target.IsDurableDelivery() || source.IsDurableDelivery() {
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

func isManagedInputQueueEntry(message *QueuedMessage) bool {
	if message == nil || message.Metadata == nil {
		return false
	}
	managed, _ := message.Metadata[MetadataManagedInput].(bool)
	return managed
}

func comparableAutoMetadata(metadata map[string]interface{}) ([]byte, bool) {
	comparable := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		if key == MetadataEntityReferences || key == MetadataContextFiles || key == MetadataQueueAdmissionIDs {
			continue
		}
		comparable[key] = value
	}
	encoded, err := json.Marshal(comparable)
	return encoded, err == nil
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
