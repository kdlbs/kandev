package messagequeue

import "context"

// Repository abstracts persistent storage of queued messages and pending moves.
// Operations on queued messages are atomic per session.
type Repository interface {
	// Insert appends a new entry at the tail of the session's FIFO queue.
	// Returns ErrQueueFull if maxPerSession > 0 and the count would exceed it.
	Insert(ctx context.Context, msg *QueuedMessage, maxPerSession int) error

	// Restore reinserts a previously dequeued entry at its original FIFO
	// position. It is used when a delivery fails after TakeHead so later
	// messages cannot overtake the failed one.
	Restore(ctx context.Context, msg *QueuedMessage, maxPerSession int) error

	// AppendOrInsertTail concatenates content onto the tail entry when the tail
	// exists AND its QueuedBy matches the supplied queuedBy. Otherwise inserts a
	// new entry. Returns the resulting entry and whether the call appended (true)
	// or inserted (false). Honors maxPerSession when inserting.
	AppendOrInsertTail(ctx context.Context, sessionID, taskID, content, model, queuedBy string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, maxPerSession int) (*QueuedMessage, bool, error)

	// InsertOrReplaceByCoalesceKey replaces an existing queued entry with the
	// same session, queued_by, and metadata coalesce key. If no matching entry
	// exists, it inserts a new tail entry when allowInsert is true. Returns
	// ErrEntryNotFound when allowInsert is false and no matching entry exists.
	InsertOrReplaceByCoalesceKey(ctx context.Context, msg *QueuedMessage, coalesceKey string, maxPerSession int, allowInsert bool) (*QueuedMessage, bool, error)

	// InsertOrReplaceLifecycleByCoalesceKey is the lifecycle-specific variant
	// that accepts the entry only while msg.TaskID exists and is not archived.
	// SQLite implementations make that check and queue mutation one transaction.
	InsertOrReplaceLifecycleByCoalesceKey(ctx context.Context, msg *QueuedMessage, coalesceKey string, maxPerSession int, allowInsert bool) (*QueuedMessage, bool, error)

	// LifecycleGeneration returns the current archive/delete generation for a
	// task. Lifecycle insertion verifies this generation atomically.
	LifecycleGeneration(ctx context.Context, taskID string) (int64, error)

	// PurgeTask is backend-only cleanup. It removes all task rows, including
	// reserved server-owned lifecycle rows, and advances its generation.
	PurgeTask(ctx context.Context, taskID string) (int, error)

	// ListBySession returns all entries for a session ordered by position ascending.
	ListBySession(ctx context.Context, sessionID string) ([]QueuedMessage, error)

	// CountBySession returns the number of entries for a session.
	CountBySession(ctx context.Context, sessionID string) (int, error)

	// TakeHead atomically returns and deletes the lowest-position entry for the
	// session. Returns nil, nil if the queue is empty.
	TakeHead(ctx context.Context, sessionID string) (*QueuedMessage, error)

	// ReserveHead returns the lowest-position entry. Ordinary entries are
	// atomically deleted, matching TakeHead. Durable lifecycle entries remain
	// stored until AcknowledgeByID is called after executor acceptance.
	ReserveHead(ctx context.Context, sessionID string) (*QueuedMessage, error)

	// AcknowledgeByID is an internal dispatch operation that removes a reserved
	// entry regardless of its server-owned queued_by identity.
	AcknowledgeByID(ctx context.Context, sessionID, entryID string) error

	// TakeByID atomically returns and deletes the entry identified by entryID
	// for sessionID, regardless of its FIFO position. Unlike DeleteByID, it has
	// no reserved queued_by guard: the caller is internal orchestrator code
	// (InterruptForPeerMessage) dispatching the specific entry that triggered
	// its own interrupt, not a user-driven "remove queued message" request.
	// Returns nil, nil if no entry matches (already taken or never existed).
	TakeByID(ctx context.Context, sessionID, entryID string) (*QueuedMessage, error)

	// UpdateContent replaces the content/attachments of an entry. The session
	// scope (`AND session_id = ?`) is mandatory so a caller can't update an
	// entry by guessing its UUID across sessions. If queuedBy is non-empty the
	// update only succeeds when the entry's queued_by also matches. Returns
	// ErrEntryNotFound when no row matches all guards.
	UpdateContent(ctx context.Context, sessionID, entryID, content string, attachments []MessageAttachment, queuedBy string) error

	// UpdateContentAndMetadata atomically replaces content/attachments and
	// applies metadata key updates. A nil update value removes that key while
	// unrelated metadata remains unchanged.
	UpdateContentAndMetadata(ctx context.Context, sessionID, entryID, content string, attachments []MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error

	// MergeIntoAbove folds the entry identified by sourceID into the entry
	// immediately above it (greatest position strictly below the source) within
	// the same session. Content, attachments, and entity references are merged
	// and the source row is removed, all in one transaction.
	//
	// The merge is allowed only between entries of the same sender kind: a
	// user-owned source merges into a user-owned target owned by queuedBy; an
	// agent-owned source merges into an agent-owned target whose
	// sender_task_id metadata matches the source's. Anything else — head source,
	// mismatched kinds, agent sender-task mismatch, caller not owning both rows,
	// or a reserved in-flight lifecycle target — returns ErrNoMergeTarget. A
	// missing source returns ErrEntryNotFound.
	MergeIntoAbove(ctx context.Context, sessionID, sourceID, queuedBy string) (*QueuedMessage, error)

	// DeleteByID removes a single entry. The session scope (`AND session_id = ?`)
	// is mandatory so a caller can't delete an entry by guessing its UUID across
	// sessions — the queue_full MCP payload deliberately discloses sibling-task
	// entry IDs, so without this guard a hostile agent could prune another
	// task's queue. A durable lifecycle entry already reserved in flight is not
	// pending and returns ErrEntryNotFound.
	DeleteByID(ctx context.Context, sessionID, entryID string) error

	// DeleteAllBySession removes every pending entry for a session, regardless
	// of provenance. Durable lifecycle entries already reserved in flight remain.
	// Returns the exact count removed.
	DeleteAllBySession(ctx context.Context, sessionID string) (int, error)

	// TransferSession moves all entries (and any pending move) from oldSessionID
	// to newSessionID. Used on workflow session switches.
	TransferSession(ctx context.Context, oldSessionID, newSessionID string) error

	// ReplaceSession replaces a session's queued entries and pending move with
	// the supplied snapshot, preserving queued-message identity fields.
	ReplaceSession(ctx context.Context, sessionID string, entries []QueuedMessage, pendingMove *PendingMove) error

	// SetPendingMove upserts the deferred workflow move for a session.
	SetPendingMove(ctx context.Context, sessionID string, move *PendingMove) error

	// GetPendingMove returns the deferred move for a session without removing it.
	// Returns nil, nil if absent.
	GetPendingMove(ctx context.Context, sessionID string) (*PendingMove, error)

	// TakePendingMove returns and removes the deferred move for a session.
	// Returns nil, nil if absent.
	TakePendingMove(ctx context.Context, sessionID string) (*PendingMove, error)
}

func applyMetadataUpdates(current, updates map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(current)+len(updates))
	for key, value := range current {
		merged[key] = value
	}
	for key, value := range updates {
		if value == nil {
			delete(merged, key)
			continue
		}
		merged[key] = value
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}
