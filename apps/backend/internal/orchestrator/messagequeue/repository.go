package messagequeue

import "context"

// Repository abstracts persistent storage of queued messages and pending moves.
// Operations on queued messages are atomic per session.
type Repository interface {
	// Insert appends a new entry at the tail of the session's FIFO queue.
	// Returns ErrQueueFull if maxPerSession > 0 and the count would exceed it.
	Insert(ctx context.Context, msg *QueuedMessage, maxPerSession int) error

	// AppendOrInsertTail concatenates content onto the tail entry when the tail
	// exists AND its QueuedBy matches the supplied queuedBy. Otherwise inserts a
	// new entry. Returns the resulting entry and whether the call appended (true)
	// or inserted (false). Honors maxPerSession when inserting.
	AppendOrInsertTail(ctx context.Context, sessionID, taskID, content, model, queuedBy string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, maxPerSession int) (*QueuedMessage, bool, error)

	// ListBySession returns all entries for a session ordered by position ascending.
	ListBySession(ctx context.Context, sessionID string) ([]QueuedMessage, error)

	// CountBySession returns the number of entries for a session.
	CountBySession(ctx context.Context, sessionID string) (int, error)

	// TakeHead atomically returns and deletes the lowest-position entry for the
	// session. Returns nil, nil if the queue is empty.
	TakeHead(ctx context.Context, sessionID string) (*QueuedMessage, error)

	// UpdateContent replaces the content/attachments of an entry by id. Returns
	// ErrEntryNotFound if the entry no longer exists. If queuedBy is non-empty,
	// the update only succeeds when the entry's queued_by matches.
	UpdateContent(ctx context.Context, entryID, content string, attachments []MessageAttachment, queuedBy string) error

	// DeleteByID removes a single entry by id. Returns ErrEntryNotFound if absent.
	DeleteByID(ctx context.Context, entryID string) error

	// DeleteAllBySession removes every entry for a session. Returns the count of
	// rows removed.
	DeleteAllBySession(ctx context.Context, sessionID string) (int, error)

	// TransferSession moves all entries (and any pending move) from oldSessionID
	// to newSessionID. Used on workflow session switches.
	TransferSession(ctx context.Context, oldSessionID, newSessionID string) error

	// SetPendingMove upserts the deferred workflow move for a session.
	SetPendingMove(ctx context.Context, sessionID string, move *PendingMove) error

	// TakePendingMove returns and removes the deferred move for a session.
	// Returns nil, nil if absent.
	TakePendingMove(ctx context.Context, sessionID string) (*PendingMove, error)
}
