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
}

// NewMemoryRepository returns an in-memory Repository. Suitable for tests.
func NewMemoryRepository() Repository {
	return &memoryRepository{
		entries:      make(map[string][]*QueuedMessage),
		nextPosition: make(map[string]int64),
		pendingMoves: make(map[string]*PendingMove),
	}
}

func (r *memoryRepository) Insert(_ context.Context, msg *QueuedMessage, maxPerSession int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insertLocked(msg, maxPerSession)
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

func (r *memoryRepository) UpdateContent(_ context.Context, sessionID, entryID, content string, attachments []MessageAttachment, queuedBy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
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
		m.Content = content
		m.Attachments = attachments
		return nil
	}
	return ErrEntryNotFound
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
		if m.QueuedBy == QueuedByAgent {
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
	n := len(r.entries[sessionID])
	delete(r.entries, sessionID)
	delete(r.nextPosition, sessionID)
	return n, nil
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
