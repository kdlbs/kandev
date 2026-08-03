package messagequeue

import (
	"context"
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

func (r *memoryRepository) LifecycleGeneration(_ context.Context, taskID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.generation[taskID], nil
}

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

func (r *memoryRepository) Insert(_ context.Context, msg *QueuedMessage, maxPerSession int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insertLocked(msg, maxPerSession)
}

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

func (r *memoryRepository) CountBySession(_ context.Context, sessionID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries[sessionID]), nil
}

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

func (r *memoryRepository) UpdateContent(ctx context.Context, sessionID, entryID, content string, attachments []MessageAttachment, queuedBy string) error {
	return r.UpdateContentAndMetadata(ctx, sessionID, entryID, content, attachments, nil, queuedBy)
}

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
		if IsReservedQueuedBy(m.QueuedBy) {
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

func (r *memoryRepository) DeleteAllBySession(_ context.Context, sessionID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.entries[sessionID]
	kept := list[:0]
	removed := 0
	for _, msg := range list {
		if IsReservedQueuedBy(msg.QueuedBy) {
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
