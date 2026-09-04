package messagequeue

import "time"

// RoutineWakeSourceReceipt is content-free provenance for one wake folded
// into a canonical pending entry.
type RoutineWakeSourceReceipt struct {
	ID       string `json:"id"`
	QueuedAt string `json:"queued_at"`
}

// RoutineWakeReceipt is the body-free durable admission result carried on the
// canonical queue entry and later copied to the visible message metadata.
type RoutineWakeReceipt struct {
	CanonicalEntryID   string                     `json:"canonical_entry_id"`
	CanonicalQueuedAt  string                     `json:"canonical_queued_at"`
	AbsorbedSources    []RoutineWakeSourceReceipt `json:"absorbed_sources"`
	AbsorbedCount      int                        `json:"absorbed_count"`
	LeaderFencingToken string                     `json:"leader_fencing_token"`
	DirtyGeneration    string                     `json:"dirty_generation"`
	PostRunRequeue     bool                       `json:"post_run_requeue"`
}

func prepareRoutineWakeSource(msg *QueuedMessage) {
	if msg == nil || !msg.IsRoutineWake() {
		return
	}
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]interface{})
	}
	msg.Metadata[MetadataRoutineSourceEntryID] = msg.ID
	msg.Metadata[MetadataRoutineSourceQueuedAt] = msg.QueuedAt.UTC().Format(time.RFC3339Nano)
	if _, ok := msg.Metadata[MetadataRoutineAbsorbedCount]; !ok {
		msg.Metadata[MetadataRoutineAbsorbedCount] = 0
	}
}

func mergeRoutineWakeMetadata(canonical, incoming map[string]interface{}) map[string]interface{} {
	merged := copyMessageMetadata(canonical, 0)
	sources := routineWakeSources(canonical)
	source := RoutineWakeSourceReceipt{
		ID:       metadataString(incoming, MetadataRoutineSourceEntryID),
		QueuedAt: metadataString(incoming, MetadataRoutineSourceQueuedAt),
	}
	if source.ID != "" && !containsRoutineWakeSource(sources, source.ID) {
		sources = append(sources, source)
	}
	merged[MetadataRoutineAbsorbedSources] = sources
	merged[MetadataRoutineAbsorbedCount] = len(sources)
	for _, key := range []string{
		MetadataRoutineLeaderFencingToken,
		MetadataRoutineDirtyGeneration,
	} {
		if value := metadataString(incoming, key); value != "" {
			merged[key] = value
		}
	}
	if metadataBool(canonical, MetadataRoutinePostRunRequeue) ||
		metadataBool(incoming, MetadataRoutinePostRunRequeue) {
		merged[MetadataRoutinePostRunRequeue] = true
	}
	return merged
}

func containsRoutineWakeSource(sources []RoutineWakeSourceReceipt, id string) bool {
	for _, source := range sources {
		if source.ID == id {
			return true
		}
	}
	return false
}

func routineWakeSources(metadata map[string]interface{}) []RoutineWakeSourceReceipt {
	raw, ok := metadata[MetadataRoutineAbsorbedSources]
	if !ok {
		return nil
	}
	sources := make([]RoutineWakeSourceReceipt, 0)
	switch values := raw.(type) {
	case []RoutineWakeSourceReceipt:
		return append(sources, values...)
	case []interface{}:
		for _, value := range values {
			entry, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			sources = append(sources, RoutineWakeSourceReceipt{
				ID:       metadataString(entry, "id"),
				QueuedAt: metadataString(entry, "queued_at"),
			})
		}
	}
	return sources
}

// RoutineWakeReceiptFromMessage returns nil for ordinary rows. It deliberately
// omits content and raw metadata.
func RoutineWakeReceiptFromMessage(msg *QueuedMessage) *RoutineWakeReceipt {
	if msg == nil || !msg.IsRoutineWake() {
		return nil
	}
	sources := routineWakeSources(msg.Metadata)
	return &RoutineWakeReceipt{
		CanonicalEntryID:   msg.ID,
		CanonicalQueuedAt:  msg.QueuedAt.UTC().Format(time.RFC3339Nano),
		AbsorbedSources:    sources,
		AbsorbedCount:      len(sources),
		LeaderFencingToken: metadataString(msg.Metadata, MetadataRoutineLeaderFencingToken),
		DirtyGeneration:    metadataString(msg.Metadata, MetadataRoutineDirtyGeneration),
		PostRunRequeue:     metadataBool(msg.Metadata, MetadataRoutinePostRunRequeue),
	}
}
