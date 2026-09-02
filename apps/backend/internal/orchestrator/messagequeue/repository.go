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

	// RequeuePreservingFIFO re-enqueues an entry, preserving both FIFO order
	// across the supersede→requeue cycle AND coalesce-replace semantics on
	// the original retry. Used by Service.requeueMessage when a queued
	// dispatch was superseded by a newer dispatch before it could be
	// claimed — without this hook the requeue landed at MAX+1 (tail) and a
	// busy session could starve the original message indefinitely.
	//
	// Decision under the lock:
	//   - existing entry with same (session_id, queued_by, coalesce_key)
	//     → replace in place (preserves position; coalesce semantics
	//       expected by lifecycle / CI-feedback retries)
	//   - empty queue                → msg.Position = 1, msg.ID assigned
	//   - non-empty queue, no match  → insert before the current head
	//
	// If the head position is already 1, implementations shift the existing
	// positions up before inserting. Queue positions stay positive so session
	// transfer and later queue mutations keep their ordering invariants.
	RequeuePreservingFIFO(ctx context.Context, msg *QueuedMessage) error

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

	// CountPendingByTaskIDs returns the number of pending entries per task,
	// keyed by task_id, for every requested task ID (zero when a task has no
	// pending entries). Pending excludes durable lifecycle rows already
	// reserved in flight, matching GetStatus semantics. The reserved exclusion
	// is applied in Go via IsReservedInFlight, never by matching JSON in SQL.
	CountPendingByTaskIDs(ctx context.Context, taskIDs []string) (map[string]int, error)

	// TakeHead atomically returns and deletes the lowest-position entry for the
	// session. Returns nil, nil if the queue is empty.
	TakeHead(ctx context.Context, sessionID string) (*QueuedMessage, error)

	// ReserveHead returns the lowest-position entry. Ordinary entries are
	// atomically deleted, matching TakeHead. Durable lifecycle entries remain
	// stored until AcknowledgeByID is called after executor acceptance.
	ReserveHead(ctx context.Context, sessionID string) (*QueuedMessage, error)

	// GetAutoRun returns the durable per-session automatic-drain policy. Missing
	// state defaults to true so existing sessions retain their current behavior.
	GetAutoRun(ctx context.Context, sessionID string) (bool, error)

	// SetAutoRun persists the per-session automatic-drain policy independently
	// of queue contents.
	SetAutoRun(ctx context.Context, sessionID string, enabled bool) error

	// PauseAutoRunIfPending atomically persists OFF only when at least one
	// visible pending entry remains. Reserved lifecycle rows do not count.
	PauseAutoRunIfPending(ctx context.Context, sessionID string) (bool, error)

	// ReserveHeadIfAutoRun reads the policy and reserves the FIFO head in one
	// per-session transaction. The returned bool is false only when Auto-run is
	// OFF; nil with true means the enabled queue is empty.
	ReserveHeadIfAutoRun(ctx context.Context, sessionID string) (*QueuedMessage, bool, error)

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

	// ClaimSendNow atomically claims the exact ordered source snapshot for an
	// interrupt-and-replace dispatch. The repository compares every requested
	// row with the snapshot before mutation, then orders the returned sources by
	// their persisted FIFO positions. Ordinary rows are removed, and durable
	// lifecycle rows are reserved until AcknowledgeSendNowClaim or
	// RestoreSendNowClaim.
	ClaimSendNow(ctx context.Context, sessionID string, expected []QueuedMessage) (*SendNowClaim, error)

	// RestoreSendNowClaim puts every source back at its original position and
	// clears durable lifecycle reservations. It must restore the complete claim
	// or leave the queue unchanged.
	RestoreSendNowClaim(ctx context.Context, claim *SendNowClaim) error

	// AcknowledgeSendNowClaim removes every durable source after the replacement
	// prompt has been accepted. Ordinary sources were already removed by claim.
	AcknowledgeSendNowClaim(ctx context.Context, claim *SendNowClaim) error

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

	// AutoMergeIntoAbove attempts to fold the exact source entry into its
	// immediate predecessor. A compatible merge returns the surviving target
	// and true. A missing source, missing predecessor, or incompatible pair is a
	// successful skip and returns false without mutating either row.
	AutoMergeIntoAbove(ctx context.Context, sessionID, sourceID string) (*QueuedMessage, bool, error)

	// AutoMergeCandidateIntoAbove folds a not-yet-admitted candidate message
	// into the session's tail entry when compatible, without inserting it. Used
	// by admission at full capacity: the fold itself is the admission, so a
	// message that would merge anyway is accepted instead of rejected with
	// ErrQueueFull. A missing tail or incompatible pair is a successful skip
	// and returns false without mutating any row.
	AutoMergeCandidateIntoAbove(ctx context.Context, candidate *QueuedMessage) (*QueuedMessage, bool, error)

	// ReorderEntries atomically rewrites the FIFO positions of the session's
	// pending entries to match orderedIDs. The submitted list must be an exact
	// permutation of the currently visible pending (not reserved-in-flight)
	// entry ids ordered as the caller wants them; any drift — missing, extra,
	// or duplicate ids, or an id belonging to a reserved in-flight row —
	// returns ErrQueueChanged and leaves the queue untouched. Reserved
	// in-flight rows keep their place in the sequence; visible rows are
	// interleaved in the submitted order and all positions are compacted to
	// 1..N in one transaction.
	ReorderEntries(ctx context.Context, sessionID string, orderedIDs []string) error

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

	// ListPendingMoves returns every armed deferred move, keyed by session.
	// TakePendingMove can only reach a move whose session still emits
	// agent.ready, so the sweep needs a session-independent view to find rows
	// nothing will ever replay.
	ListPendingMoves(ctx context.Context) ([]PendingMoveRecord, error)

	// DeletePendingMoveIfMatch removes the deferred move only when the stored
	// row still matches the exact record previously returned by
	// ListPendingMoves. When handoffEntryID is non-empty, the correlated queue
	// entry is removed in the same transaction. Reports whether the move row was
	// removed; a missing or replaced row is a successful no-op, not an error.
	DeletePendingMoveIfMatch(ctx context.Context, expected PendingMoveRecord, handoffEntryID string) (bool, error)
}

// applyMetadataUpdates merges metadata key updates into current; a nil value removes the key.
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
