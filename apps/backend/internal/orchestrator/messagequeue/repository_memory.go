package messagequeue

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// memoryRepository is an in-memory Repository implementation used in tests and
// any deployment that explicitly opts into ephemeral queueing.
type memoryRepository struct {
	mu           sync.Mutex
	entries      map[string][]*QueuedMessage // sessionID -> ordered list (head = index 0)
	nextPosition map[string]int64            // sessionID -> monotonic counter
	pendingMoves map[string]*PendingMove
	generation   map[string]int64
}

// NewMemoryRepository returns an in-memory Repository. Suitable for tests.
func NewMemoryRepository() Repository {
	return &memoryRepository{
		entries:      make(map[string][]*QueuedMessage),
		nextPosition: make(map[string]int64),
		pendingMoves: make(map[string]*PendingMove),
		generation:   make(map[string]int64),
	}
}

// LifecycleGeneration returns the current archive/delete generation for a task.
func (r *memoryRepository) LifecycleGeneration(_ context.Context, taskID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.generation[taskID], nil
}

// PurgeTask removes all task rows and advances its generation.
func (r *memoryRepository) PurgeTask(_ context.Context, taskID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := 0
	for sessionID, list := range r.entries {
		kept := list[:0]
		for _, msg := range list {
			if msg.TaskID == taskID {
				removed++
				continue
			}
			kept = append(kept, msg)
		}
		if len(kept) == 0 {
			delete(r.entries, sessionID)
			delete(r.nextPosition, sessionID)
			continue
		}
		r.entries[sessionID] = kept
	}
	r.generation[taskID]++
	return removed, nil
}

// CountPendingByTaskIDs counts pending entries per task, excluding durable
// lifecycle rows reserved in flight (filtered via IsReservedInFlight).
func (r *memoryRepository) CountPendingByTaskIDs(_ context.Context, taskIDs []string) (map[string]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	counts := make(map[string]int, len(taskIDs))
	for _, taskID := range taskIDs {
		counts[taskID] = 0
	}
	if len(taskIDs) == 0 {
		return counts, nil
	}
	for _, list := range r.entries {
		for _, msg := range list {
			if msg.IsReservedInFlight() {
				continue
			}
			if _, wanted := counts[msg.TaskID]; wanted {
				counts[msg.TaskID]++
			}
		}
	}
	return counts, nil
}

// Insert appends a new entry at the tail of the session's FIFO queue.
func (r *memoryRepository) Insert(_ context.Context, msg *QueuedMessage, maxPerSession int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insertLocked(msg, maxPerSession)
}

// Restore reinserts a previously dequeued entry at its original FIFO position.
func (r *memoryRepository) Restore(_ context.Context, msg *QueuedMessage, maxPerSession int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.entries[msg.SessionID]
	if maxPerSession > 0 && len(list) >= maxPerSession {
		return ErrQueueFull
	}
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.QueuedAt.IsZero() {
		msg.QueuedAt = time.Now().UTC()
	}
	clone := *msg
	index := sort.Search(len(list), func(i int) bool { return list[i].Position > clone.Position })
	list = append(list, nil)
	copy(list[index+1:], list[index:])
	list[index] = &clone
	r.entries[msg.SessionID] = list
	if clone.Position > r.nextPosition[msg.SessionID] {
		r.nextPosition[msg.SessionID] = clone.Position
	}
	return nil
}

// insertLocked performs the actual insert. Caller must already hold r.mu.
func (r *memoryRepository) insertLocked(msg *QueuedMessage, maxPerSession int) error {
	list := r.entries[msg.SessionID]
	if maxPerSession > 0 && len(list) >= maxPerSession {
		return ErrQueueFull
	}
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.QueuedAt.IsZero() {
		msg.QueuedAt = time.Now().UTC()
	}
	r.nextPosition[msg.SessionID]++
	msg.Position = r.nextPosition[msg.SessionID]
	clone := *msg
	r.entries[msg.SessionID] = append(list, &clone)
	return nil
}

// AppendOrInsertTail must hold the lock for the entire check-then-insert path
// so two concurrent same-sender callers can't both observe "no matching tail"
// and race to insert separate entries (which would violate the
// append-or-insert semantics the SQLite repo achieves with a transaction).
func (r *memoryRepository) AppendOrInsertTail(_ context.Context, sessionID, taskID, content, model, queuedBy string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, maxPerSession int) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	list := r.entries[sessionID]
	if len(list) > 0 {
		tail := list[len(list)-1]
		if tail.QueuedBy == queuedBy {
			tail.Content = tail.Content + "\n\n---\n\n" + content
			out := *tail
			return &out, true, nil
		}
	}

	msg := &QueuedMessage{
		SessionID:   sessionID,
		TaskID:      taskID,
		Content:     content,
		Model:       model,
		PlanMode:    planMode,
		Attachments: attachments,
		Metadata:    metadata,
		QueuedBy:    queuedBy,
	}
	if err := r.insertLocked(msg, maxPerSession); err != nil {
		return nil, false, err
	}
	return msg, false, nil
}

// InsertOrReplaceByCoalesceKey replaces an entry with the same session/queued_by/coalesce key, or inserts when allowInsert is set.
func (r *memoryRepository) InsertOrReplaceByCoalesceKey(_ context.Context, msg *QueuedMessage, coalesceKey string, maxPerSession int, allowInsert bool) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.entries[msg.SessionID] {
		if existing.QueuedBy != msg.QueuedBy {
			continue
		}
		if metadataString(existing.Metadata, MetadataCoalesceKey) != coalesceKey {
			continue
		}
		if msg.QueuedAt.IsZero() {
			msg.QueuedAt = time.Now().UTC()
		}
		existing.TaskID = msg.TaskID
		existing.Content = msg.Content
		existing.Model = msg.Model
		existing.PlanMode = msg.PlanMode
		existing.Attachments = msg.Attachments
		existing.Metadata = msg.Metadata
		existing.QueuedAt = msg.QueuedAt
		out := *existing
		return &out, true, nil
	}
	if !allowInsert {
		return nil, false, ErrEntryNotFound
	}
	if err := r.insertLocked(msg, maxPerSession); err != nil {
		return nil, false, err
	}
	return msg, false, nil
}

// The memory queue has no task store. Tests which need an archive race wrap
// this method with their deterministic active-task guard.
func (r *memoryRepository) InsertOrReplaceLifecycleByCoalesceKey(ctx context.Context, msg *QueuedMessage, coalesceKey string, maxPerSession int, allowInsert bool) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	expected, ok := lifecycleGenerationFromMetadata(msg.Metadata)
	if !ok || expected != r.generation[msg.TaskID] {
		return nil, false, ErrLifecycleCancelled
	}
	for _, existing := range r.entries[msg.SessionID] {
		if existing.QueuedBy != msg.QueuedBy || metadataString(existing.Metadata, MetadataCoalesceKey) != coalesceKey {
			continue
		}
		if msg.QueuedAt.IsZero() {
			msg.QueuedAt = time.Now().UTC()
		}
		existing.TaskID = msg.TaskID
		existing.Content = msg.Content
		existing.Model = msg.Model
		existing.PlanMode = msg.PlanMode
		existing.Attachments = msg.Attachments
		existing.Metadata = msg.Metadata
		existing.QueuedAt = msg.QueuedAt
		out := *existing
		return &out, true, nil
	}
	if !allowInsert {
		return nil, false, ErrEntryNotFound
	}
	if err := r.insertLocked(msg, maxPerSession); err != nil {
		return nil, false, err
	}
	return msg, false, nil
}

// ListBySession returns all entries for a session ordered by position ascending.
func (r *memoryRepository) ListBySession(_ context.Context, sessionID string) ([]QueuedMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.entries[sessionID]
	out := make([]QueuedMessage, len(list))
	for i, m := range list {
		out[i] = *m
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, nil
}

// CountBySession returns the number of entries for a session.
func (r *memoryRepository) CountBySession(_ context.Context, sessionID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries[sessionID]), nil
}

// TakeHead atomically returns and deletes the lowest-position entry for the session.
func (r *memoryRepository) TakeHead(_ context.Context, sessionID string) (*QueuedMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.entries[sessionID]
	if len(list) == 0 {
		return nil, nil
	}
	head := list[0]
	r.entries[sessionID] = list[1:]
	if len(r.entries[sessionID]) == 0 {
		delete(r.entries, sessionID)
		delete(r.nextPosition, sessionID)
	}
	out := *head
	return &out, nil
}

// ReserveHead returns the lowest-position entry, deleting ordinary rows and reserving durable lifecycle rows.
func (r *memoryRepository) ReserveHead(_ context.Context, sessionID string) (*QueuedMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.entries[sessionID]
	if len(list) == 0 {
		return nil, nil
	}
	head := list[0]
	out := *head
	if head.IsDurableLifecycle() {
		// Mirror the SQLite reservation: the stored row is flagged in flight so
		// queue status stops listing it, while the returned copy keeps the
		// unmarked metadata a requeue would write back.
		out.Metadata = clearReservedMetadata(head.Metadata)
		out.reservedLifecycleDelivery = true
		head.Metadata = markReservedMetadata(out.Metadata)
		return &out, nil
	}
	r.entries[sessionID] = list[1:]
	if len(r.entries[sessionID]) == 0 {
		delete(r.entries, sessionID)
		delete(r.nextPosition, sessionID)
	}
	return &out, nil
}

// AcknowledgeByID removes a reserved durable entry after executor acceptance.
func (r *memoryRepository) AcknowledgeByID(_ context.Context, sessionID, entryID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.entries[sessionID]
	for i, msg := range list {
		if msg.ID != entryID {
			continue
		}
		r.entries[sessionID] = append(list[:i], list[i+1:]...)
		if len(r.entries[sessionID]) == 0 {
			delete(r.entries, sessionID)
			delete(r.nextPosition, sessionID)
		}
		return nil
	}
	return ErrEntryNotFound
}

// TakeByID atomically returns and deletes the entry identified by entryID,
// regardless of its FIFO position. Unlike DeleteByID, it has no
// QueuedByAgent guard — see the Repository interface doc comment.
func (r *memoryRepository) TakeByID(_ context.Context, sessionID, entryID string) (*QueuedMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list, ok := r.entries[sessionID]
	if !ok {
		return nil, nil
	}
	for i, m := range list {
		if m.ID != entryID {
			continue
		}
		r.entries[sessionID] = append(list[:i], list[i+1:]...)
		if len(r.entries[sessionID]) == 0 {
			delete(r.entries, sessionID)
			delete(r.nextPosition, sessionID)
		}
		out := *m
		return &out, nil
	}
	return nil, nil
}

// ClaimSendNow atomically claims the exact ordered source snapshot for a send-now dispatch.
func (r *memoryRepository) ClaimSendNow(_ context.Context, sessionID string, expected []QueuedMessage) (*SendNowClaim, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(expected) == 0 {
		return nil, ErrSendNowEmpty
	}

	requested, err := requestedSendNowIDs(expected)
	if err != nil {
		return nil, err
	}

	list := r.entries[sessionID]
	selected := make([]*QueuedMessage, 0, len(expected))
	for _, entry := range list {
		if _, ok := requested[entry.ID]; !ok {
			continue
		}
		if entry.IsReservedInFlight() {
			return nil, ErrSendNowReservationConflict
		}
		selected = append(selected, entry)
	}
	if len(selected) != len(expected) {
		return nil, ErrSendNowClaimChanged
	}
	if err := validateSendNowSnapshot(selected, expected); err != nil {
		return nil, ErrSendNowClaimChanged
	}

	sources := cloneSendNowSources(selected)
	envelope, err := BuildSendNowEnvelope(sources)
	if err != nil {
		return nil, err
	}
	generations := make(map[string]int64)
	for _, source := range sources {
		if source.TaskID != "" {
			generations[source.TaskID] = r.generation[source.TaskID]
		}
	}

	remaining := make([]*QueuedMessage, 0, len(list))
	for _, entry := range list {
		if _, ok := requested[entry.ID]; !ok {
			remaining = append(remaining, entry)
			continue
		}
		if entry.IsDurableLifecycle() {
			entry.Metadata = markReservedMetadata(entry.Metadata)
			remaining = append(remaining, entry)
		}
	}
	if len(remaining) == 0 {
		delete(r.entries, sessionID)
		delete(r.nextPosition, sessionID)
	} else {
		r.entries[sessionID] = remaining
	}
	return &SendNowClaim{Sources: sources, Dispatch: *envelope, SourceGenerations: generations}, nil
}

// RestoreSendNowClaim puts every claimed source back at its original position.
func (r *memoryRepository) RestoreSendNowClaim(_ context.Context, claim *SendNowClaim) error {
	if claim == nil || len(claim.Sources) == 0 {
		return ErrSendNowEmpty
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	sessionID := claim.Sources[0].SessionID
	list := r.entries[sessionID]
	existing := make(map[string]*QueuedMessage, len(list))
	for _, entry := range list {
		existing[entry.ID] = entry
	}
	if err := validateMemorySendNowRestore(claim, sessionID, existing, r.generation); err != nil {
		return err
	}

	for _, source := range claim.Sources {
		if sendNowSourceGenerationChanged(claim, source, r.generation[source.TaskID]) {
			continue
		}
		if existing[source.ID] != nil {
			if source.IsDurableLifecycle() {
				existing[source.ID].Metadata = restoreSendNowMetadata(existing[source.ID].Metadata, source.Metadata)
			}
			continue
		}
		clone := cloneQueuedMessage(&source)
		clone.Metadata = clearReservedMetadata(clone.Metadata)
		list = append(list, clone)
		if clone.Position > r.nextPosition[sessionID] {
			r.nextPosition[sessionID] = clone.Position
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Position < list[j].Position })
	r.entries[sessionID] = list
	return nil
}

// validateMemorySendNowRestore verifies the stored entries still match the claim before restoring.
func validateMemorySendNowRestore(
	claim *SendNowClaim,
	sessionID string,
	existing map[string]*QueuedMessage,
	generations map[string]int64,
) error {
	for _, source := range claim.Sources {
		if source.SessionID != sessionID {
			return ErrSendNowClaimChanged
		}
		if sendNowSourceGenerationChanged(claim, source, generations[source.TaskID]) {
			continue
		}
		entry := existing[source.ID]
		if source.IsDurableLifecycle() {
			if entry == nil || (!entry.IsReservedInFlight() && !sameQueuedMessageContent(entry, &source)) {
				return ErrSendNowClaimChanged
			}
			continue
		}
		if entry != nil && entry.IsReservedInFlight() {
			return ErrSendNowClaimChanged
		}
	}
	return nil
}

// AcknowledgeSendNowClaim removes every durable source after the replacement prompt is accepted.
func (r *memoryRepository) AcknowledgeSendNowClaim(_ context.Context, claim *SendNowClaim) error {
	if claim == nil || len(claim.Sources) == 0 {
		return ErrSendNowEmpty
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sessionID := claim.Sources[0].SessionID
	requested := make(map[string]struct{})
	for _, source := range claim.Sources {
		if source.SessionID != sessionID {
			return ErrSendNowClaimChanged
		}
		if !source.IsDurableLifecycle() {
			continue
		}
		requested[source.ID] = struct{}{}
	}
	for _, entry := range r.entries[sessionID] {
		if _, ok := requested[entry.ID]; ok && !entry.IsReservedInFlight() {
			return ErrSendNowClaimChanged
		}
	}
	if len(requested) == 0 {
		return nil
	}
	list := r.entries[sessionID]
	remaining := list[:0]
	for _, entry := range list {
		if _, ok := requested[entry.ID]; !ok {
			remaining = append(remaining, entry)
		}
	}
	if len(remaining) == 0 {
		delete(r.entries, sessionID)
		delete(r.nextPosition, sessionID)
	} else {
		r.entries[sessionID] = remaining
	}
	return nil
}

// cloneSendNowSources deep-copies the claimed sources so later mutation cannot alter the claim.
func cloneSendNowSources(entries []*QueuedMessage) []QueuedMessage {
	sources := make([]QueuedMessage, 0, len(entries))
	for _, entry := range entries {
		clone := cloneQueuedMessage(entry)
		clone.Metadata = clearReservedMetadata(clone.Metadata)
		if entry.IsDurableLifecycle() {
			clone.reservedLifecycleDelivery = true
		}
		sources = append(sources, *clone)
	}
	return sources
}

// cloneQueuedMessage deep-copies a queued message including metadata and attachments.
func cloneQueuedMessage(entry *QueuedMessage) *QueuedMessage {
	if entry == nil {
		return nil
	}
	clone := *entry
	clone.Attachments = append([]MessageAttachment(nil), entry.Attachments...)
	clone.Metadata = copyMessageMetadata(entry.Metadata, 0)
	return &clone
}

// sameQueuedMessageContent reports whether two entries carry identical content, attachments, and metadata.
func sameQueuedMessageContent(left, right *QueuedMessage) bool {
	if left == nil || right == nil {
		return false
	}
	leftCopy := cloneQueuedMessage(left)
	rightCopy := cloneQueuedMessage(right)
	leftCopy.Metadata = clearReservedMetadata(leftCopy.Metadata)
	rightCopy.Metadata = clearReservedMetadata(rightCopy.Metadata)
	leftCopy.reservedLifecycleDelivery = false
	rightCopy.reservedLifecycleDelivery = false
	return reflect.DeepEqual(leftCopy, rightCopy)
}

// UpdateContent replaces the content of an entry owned by queuedBy.
func (r *memoryRepository) UpdateContent(ctx context.Context, sessionID, entryID, content string, attachments []MessageAttachment, queuedBy string) error {
	return r.UpdateContentAndMetadata(ctx, sessionID, entryID, content, attachments, nil, queuedBy)
}

// UpdateContentAndMetadata replaces content and applies metadata updates to an entry owned by queuedBy.
func (r *memoryRepository) UpdateContentAndMetadata(_ context.Context, sessionID, entryID, content string, attachments []MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if queuedBy == "" || IsReservedQueuedBy(queuedBy) {
		return ErrEntryNotFound
	}
	list, ok := r.entries[sessionID]
	if !ok {
		return ErrEntryNotFound
	}
	for _, m := range list {
		if m.ID != entryID {
			continue
		}
		if queuedBy != "" && m.QueuedBy != queuedBy {
			return ErrEntryNotFound
		}
		if IsReservedQueuedBy(m.QueuedBy) {
			return ErrEntryNotFound
		}
		m.Content = content
		m.Attachments = attachments
		m.Metadata = applyMetadataUpdates(m.Metadata, metadataUpdates)
		return nil
	}
	return ErrEntryNotFound
}

// MergeIntoAbove folds the source entry into the entry directly above it within
// the same session, mirroring the sqlite repository's semantics. The target is
// the entry with the greatest position strictly below the source's — the slice
// may not be position-sorted after ReplaceSession. See
// Repository.MergeIntoAbove for the merge rules and error mapping.
func (r *memoryRepository) MergeIntoAbove(_ context.Context, sessionID, sourceID, queuedBy string) (*QueuedMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list, ok := r.entries[sessionID]
	if !ok {
		return nil, ErrEntryNotFound
	}
	sourceIndex := -1
	for i, m := range list {
		if m.ID == sourceID {
			sourceIndex = i
			break
		}
	}
	if sourceIndex < 0 {
		return nil, ErrEntryNotFound
	}
	source := list[sourceIndex]

	var target *QueuedMessage
	for _, m := range list {
		if m.Position >= source.Position {
			continue
		}
		if target == nil || m.Position > target.Position {
			target = m
		}
	}
	if target == nil {
		return nil, ErrNoMergeTarget
	}
	if !mergeAllowed(source, target, queuedBy) {
		return nil, ErrNoMergeTarget
	}

	// Compute every merged value before mutating the target: mergeEntryMetadata
	// can reject an over-cap reference union, and the target must stay untouched
	// on that path so the failed merge is atomic (mirrors the sqlite
	// repository's build-then-apply ordering).
	content := joinMergeContent(target.Content, source.Content)
	attachments := append(append([]MessageAttachment{}, target.Attachments...), source.Attachments...)
	metadata, err := mergeEntryMetadata(target.Metadata, source.Metadata)
	if err != nil {
		return nil, err
	}
	target.Content = content
	target.Attachments = attachments
	target.Metadata = metadata
	r.entries[sessionID] = append(list[:sourceIndex], list[sourceIndex+1:]...)
	if len(r.entries[sessionID]) == 0 {
		delete(r.entries, sessionID)
		delete(r.nextPosition, sessionID)
	}

	merged := *target
	return &merged, nil
}

// ReorderEntries rewrites the session's visible pending order to match
// orderedIDs, mirroring the sqlite repository's semantics: reserved in-flight
// rows keep their place in the sequence, visible rows appear in the submitted
// order, and positions are compacted to 1..N. The stored slice is reassigned
// to the new sequence so TakeHead/ReserveHead (which consume list[0]) drain in
// the reordered order, not the pre-reorder slice order. Any drift returns
// ErrQueueChanged and leaves every entry untouched.
func (r *memoryRepository) ReorderEntries(_ context.Context, sessionID string, orderedIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := r.entries[sessionID]

	// The stored slice is not guaranteed position-sorted after ReplaceSession;
	// sort a copy so reserved rows interleave at their persisted places.
	sorted := make([]*QueuedMessage, len(stored))
	copy(sorted, stored)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })

	visible, _ := splitVisibleAndReserved(sorted)
	ordered, err := validateReorderSet(visible, orderedIDs)
	if err != nil {
		return err
	}

	sequence := make([]*QueuedMessage, 0, len(sorted))
	visibleCursor := 0
	for _, msg := range sorted {
		if msg.IsReservedInFlight() {
			sequence = append(sequence, msg)
		} else {
			sequence = append(sequence, ordered[visibleCursor])
			visibleCursor++
		}
	}
	for i, msg := range sequence {
		msg.Position = int64(i + 1)
	}
	if len(sequence) > 0 {
		// The entries slice is consumed by index (TakeHead/ReserveHead take
		// list[0]); it must be stored in the new position order or drains
		// would still return the pre-reorder head.
		r.entries[sessionID] = sequence
		if n := len(sequence); int64(n) > r.nextPosition[sessionID] {
			r.nextPosition[sessionID] = int64(n)
		}
	}
	return nil
}

// DeleteByID removes a single pending entry scoped to its session.
func (r *memoryRepository) DeleteByID(_ context.Context, sessionID, entryID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list, ok := r.entries[sessionID]
	if !ok {
		return ErrEntryNotFound
	}
	for i, m := range list {
		if m.ID != entryID {
			continue
		}
		if m.IsReservedInFlight() {
			return ErrEntryNotFound
		}
		r.entries[sessionID] = append(list[:i], list[i+1:]...)
		if len(r.entries[sessionID]) == 0 {
			delete(r.entries, sessionID)
			delete(r.nextPosition, sessionID)
		}
		return nil
	}
	return ErrEntryNotFound
}

// DeleteAllBySession removes every pending entry for a session, keeping reserved in-flight rows.
func (r *memoryRepository) DeleteAllBySession(_ context.Context, sessionID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.entries[sessionID]
	kept := list[:0]
	removed := 0
	for _, msg := range list {
		if msg.IsReservedInFlight() {
			kept = append(kept, msg)
			continue
		}
		removed++
	}
	if len(kept) == 0 {
		delete(r.entries, sessionID)
		delete(r.nextPosition, sessionID)
	} else {
		r.entries[sessionID] = kept
	}
	return removed, nil
}

// TransferSession moves all entries (and any pending move) from one session to another.
func (r *memoryRepository) TransferSession(_ context.Context, oldSessionID, newSessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if list, ok := r.entries[oldSessionID]; ok {
		// Mirror the SQLite repo: shift transferred positions past the
		// destination's max so source entries always sort *after* anything
		// already queued there. Without this, a transfer into a non-empty
		// destination would mix orderings and break FIFO drain.
		var destMax int64
		for _, m := range r.entries[newSessionID] {
			if m.Position > destMax {
				destMax = m.Position
			}
		}
		for _, m := range list {
			m.SessionID = newSessionID
			m.Position += destMax
		}
		r.entries[newSessionID] = append(r.entries[newSessionID], list...)
		// Recompute nextPosition for the destination so future inserts keep
		// monotonic ordering.
		var maxPos int64
		for _, m := range r.entries[newSessionID] {
			if m.Position > maxPos {
				maxPos = m.Position
			}
		}
		r.nextPosition[newSessionID] = maxPos
		delete(r.entries, oldSessionID)
		delete(r.nextPosition, oldSessionID)
	}
	if move, ok := r.pendingMoves[oldSessionID]; ok {
		r.pendingMoves[newSessionID] = move
		delete(r.pendingMoves, oldSessionID)
	}
	return nil
}

// ReplaceSession replaces a session's queue with the supplied snapshot.
func (r *memoryRepository) ReplaceSession(_ context.Context, sessionID string, entries []QueuedMessage, pendingMove *PendingMove) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(entries) == 0 {
		delete(r.entries, sessionID)
		delete(r.nextPosition, sessionID)
	} else {
		replaced := make([]*QueuedMessage, 0, len(entries))
		var maxPos int64
		for _, entry := range entries {
			clone := entry
			clone.SessionID = sessionID
			if clone.Position > maxPos {
				maxPos = clone.Position
			}
			replaced = append(replaced, &clone)
		}
		r.entries[sessionID] = replaced
		r.nextPosition[sessionID] = maxPos
	}
	if pendingMove == nil {
		delete(r.pendingMoves, sessionID)
		return nil
	}
	clone := *pendingMove
	r.pendingMoves[sessionID] = &clone
	return nil
}

// SetPendingMove upserts the deferred workflow move for a session.
func (r *memoryRepository) SetPendingMove(_ context.Context, sessionID string, move *PendingMove) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if move.QueuedAt.IsZero() {
		move.QueuedAt = time.Now().UTC()
	}
	clone := *move
	r.pendingMoves[sessionID] = &clone
	return nil
}

// GetPendingMove returns the deferred workflow move for a session, or nil when absent.
func (r *memoryRepository) GetPendingMove(_ context.Context, sessionID string) (*PendingMove, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	move, ok := r.pendingMoves[sessionID]
	if !ok {
		return nil, nil
	}
	clone := *move
	return &clone, nil
}

// TakePendingMove returns and removes the deferred workflow move for a session.
func (r *memoryRepository) TakePendingMove(_ context.Context, sessionID string) (*PendingMove, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	move, ok := r.pendingMoves[sessionID]
	if !ok {
		return nil, nil
	}
	delete(r.pendingMoves, sessionID)
	return move, nil
}
