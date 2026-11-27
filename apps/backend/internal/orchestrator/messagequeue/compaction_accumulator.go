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

// startCompactionMerge opens a survivor's accumulator with its first source.
// Identity, metadata, and attachment count are checked before any payload is
// decoded, so a rejected pair costs no more than a single admission check.
func startCompactionMerge(target, source *QueuedMessage) (*autoMergeAccumulator, bool) {
	if !autoMergeAllowed(target, source) || !autoMetadataEquivalent(target.Metadata, source.Metadata) ||
		len(target.Attachments)+len(source.Attachments) > messageconstraints.MaxAttachmentCount {
		return nil, false
	}
	merge, ok := newAutoMergeAccumulator(target)
	if !ok || !merge.absorb(source) {
		return nil, false
	}
	return merge, true
}

// autoMergeAccumulator folds a chain of compatible sources into one compaction
// survivor. Every input is validated once and its additive lists are kept as
// ordered sets, so N folds cost time linear in their inputs. Single admissions
// use buildAutoMergedEntry; TestCompactionFoldMatchesSingleAutoMerge pins that
// both produce the same entry.
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
	admissionReason string
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
	if reason := knownAdmissionReason(message.Metadata); reason != "" {
		a.admissionReason = reason
	}
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
	metadata = retainAdmissionReason(metadata, a.admissionReason)
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
