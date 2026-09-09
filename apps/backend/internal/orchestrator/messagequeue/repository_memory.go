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
	mu                 sync.Mutex
	entries            map[string][]*QueuedMessage // sessionID -> ordered list (head = index 0)
	nextPosition       map[string]int64            // sessionID -> monotonic counter
	pendingMoves       map[string]*PendingMove
	generation         map[string]int64
	sendNowGeneration  map[string]int64
	autoRun            map[string]bool
	autoRunIncarnation map[string]string
	autoMergeOverrides map[QueueSessionIdentity]AutoMergeOverride
	identities         map[string]QueueSessionIdentity
	statusGeneration   map[string]int64
	authority          func(context.Context, string, string) (QueueSessionIdentity, error)
}

// NewMemoryRepository returns an in-memory Repository. Suitable for tests.
func NewMemoryRepository() Repository {
	return &memoryRepository{
		entries:            make(map[string][]*QueuedMessage),
		nextPosition:       make(map[string]int64),
		pendingMoves:       make(map[string]*PendingMove),
		generation:         make(map[string]int64),
		sendNowGeneration:  make(map[string]int64),
		autoRun:            make(map[string]bool),
		autoRunIncarnation: make(map[string]string),
		autoMergeOverrides: make(map[QueueSessionIdentity]AutoMergeOverride),
		statusGeneration:   make(map[string]int64),
		identities:         make(map[string]QueueSessionIdentity),
	}
}

// NewMemoryRepositoryWithAuthority returns an in-memory queue whose immutable
// identities come from the supplied task-session authority.
func NewMemoryRepositoryWithAuthority(
	authority func(context.Context, string, string) (QueueSessionIdentity, error),
) Repository {
	repo := NewMemoryRepository().(*memoryRepository)
	repo.authority = authority
	return repo
}

func (r *memoryRepository) clearSessionStateLocked(sessionID string) {
	delete(r.entries, sessionID)
	delete(r.nextPosition, sessionID)
	delete(r.pendingMoves, sessionID)
	delete(r.sendNowGeneration, sessionID)
	delete(r.autoRun, sessionID)
	delete(r.autoRunIncarnation, sessionID)
	delete(r.statusGeneration, sessionID)
	delete(r.identities, sessionID)
	for identity := range r.autoMergeOverrides {
		if identity.SessionID == sessionID {
			delete(r.autoMergeOverrides, identity)
		}
	}
}

func (r *memoryRepository) bindIdentityLocked(identity QueueSessionIdentity) error {
	if identity.TaskID == "" || identity.SessionID == "" || identity.SessionIncarnationID == "" {
		return ErrSessionIdentityMismatch
	}
	if current, ok := r.identities[identity.SessionID]; ok {
		if current != identity {
			return ErrSessionIdentityMismatch
		}
		return nil
	}
	if identity.SessionIncarnationID != "memory:"+identity.SessionID {
		return ErrSessionIdentityMismatch
	}
	for _, entry := range r.entries[identity.SessionID] {
		if entry.TaskID != identity.TaskID {
			return ErrSessionIdentityMismatch
		}
	}
	r.identities[identity.SessionID] = identity
	return nil
}

func (r *memoryRepository) ResolveSessionIdentity(ctx context.Context, taskID, sessionID string) (QueueSessionIdentity, error) {
	if taskID == "" || sessionID == "" {
		return QueueSessionIdentity{}, ErrSessionIdentityMismatch
	}
	if r.authority != nil {
		identity, err := r.authority(ctx, taskID, sessionID)
		if err != nil {
			return QueueSessionIdentity{}, err
		}
		if identity.TaskID != taskID || identity.SessionID != sessionID || identity.SessionIncarnationID == "" {
			return QueueSessionIdentity{}, ErrSessionIdentityMismatch
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if existing, ok := r.identities[sessionID]; ok && existing != identity {
			r.clearSessionStateLocked(sessionID)
		}
		r.identities[sessionID] = identity
		return identity, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if identity, ok := r.identities[sessionID]; ok {
		if identity.TaskID != taskID {
			return QueueSessionIdentity{}, ErrSessionIdentityMismatch
		}
		return identity, nil
	}
	identity := QueueSessionIdentity{
		TaskID: taskID, SessionID: sessionID, SessionIncarnationID: "memory:" + sessionID,
	}
	r.identities[sessionID] = identity
	return identity, nil
}

func (r *memoryRepository) Snapshot(
	_ context.Context,
	identity QueueSessionIdentity,
) (RepositorySnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return RepositorySnapshot{}, err
	}
	entries := make([]QueuedMessage, len(r.entries[identity.SessionID]))
	for index, entry := range r.entries[identity.SessionID] {
		entries[index] = *entry
	}
	autoRun := r.autoRunForIdentityLocked(identity)
	var override *AutoMergeOverride
	if stored, ok := r.autoMergeOverrides[identity]; ok {
		copy := stored
		override = &copy
	}
	var pendingMove *PendingMove
	if stored := r.pendingMoves[identity.SessionID]; stored != nil {
		copy := *stored
		pendingMove = &copy
	}
	r.statusGeneration[identity.SessionID]++
	return RepositorySnapshot{
		Entries:           entries,
		PendingMove:       pendingMove,
		AutoRun:           autoRun,
		AutoMergeOverride: override,
		StatusGeneration:  r.statusGeneration[identity.SessionID],
	}, nil
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
	for sessionID, move := range r.pendingMoves {
		if move != nil && move.TaskID == taskID {
			delete(r.pendingMoves, sessionID)
		}
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

func (r *memoryRepository) InsertForSession(_ context.Context, identity QueueSessionIdentity, msg *QueuedMessage, maxPerSession int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg == nil || msg.SessionID != identity.SessionID || msg.TaskID != identity.TaskID {
		return ErrSessionIdentityMismatch
	}
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	return r.insertLocked(msg, maxPerSession)
}

func (r *memoryRepository) InsertForSessionWithClaim(_ context.Context, identity QueueSessionIdentity, msg *QueuedMessage, _ QueueAttachmentClaim, maxPerSession int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg == nil || msg.SessionID != identity.SessionID || msg.TaskID != identity.TaskID {
		return ErrSessionIdentityMismatch
	}
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	return r.insertLocked(msg, maxPerSession)
}

func (r *memoryRepository) InsertForSessionWithPolicy(
	_ context.Context,
	identity QueueSessionIdentity,
	msg *QueuedMessage,
	_ *QueueAttachmentClaim,
	maxPerSession int,
	policy AutoMergePolicy,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg == nil || msg.SessionID != identity.SessionID || msg.TaskID != identity.TaskID {
		return ErrSessionIdentityMismatch
	}
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	if err := r.validateAutoMergePolicyLocked(identity, policy); err != nil {
		return err
	}
	return r.insertLocked(msg, maxPerSession)
}

// Restore reinserts a previously dequeued entry at its original FIFO position.
func (r *memoryRepository) Restore(_ context.Context, msg *QueuedMessage, maxPerSession int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.restoreLocked(msg, maxPerSession)
}

func (r *memoryRepository) RestoreForSession(
	_ context.Context,
	identity QueueSessionIdentity,
	msg *QueuedMessage,
	maxPerSession int,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg == nil || msg.SessionID != identity.SessionID || msg.TaskID != identity.TaskID {
		return ErrSessionIdentityMismatch
	}
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	return r.restoreLocked(msg, maxPerSession)
}

func (r *memoryRepository) restoreLocked(msg *QueuedMessage, maxPerSession int) error {
	list := r.entries[msg.SessionID]
	if maxPerSession > 0 && ordinaryQueueEntryCount(list) >= maxPerSession {
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

func (r *memoryRepository) GetAutoMergeOverride(
	_ context.Context,
	identity QueueSessionIdentity,
) (*AutoMergeOverride, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateIdentityLocked(identity); err != nil {
		return nil, err
	}
	override, ok := r.autoMergeOverrides[identity]
	if !ok {
		return nil, nil
	}
	return &override, nil
}

func (r *memoryRepository) SetAutoMergeOverride(
	_ context.Context,
	identity QueueSessionIdentity,
	enabled bool,
) (AutoMergeOverride, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return AutoMergeOverride{}, err
	}
	override := r.autoMergeOverrides[identity]
	override.Enabled = enabled
	override.Revision++
	r.autoMergeOverrides[identity] = override
	return override, nil
}

func (r *memoryRepository) validateIdentityLocked(identity QueueSessionIdentity) error {
	if identity.TaskID == "" || identity.SessionID == "" || identity.SessionIncarnationID == "" {
		return ErrSessionIdentityMismatch
	}
	current, ok := r.identities[identity.SessionID]
	if !ok || current != identity {
		return ErrSessionIdentityMismatch
	}
	return nil
}

func (r *memoryRepository) validateAutoMergePolicyLocked(identity QueueSessionIdentity, policy AutoMergePolicy) error {
	override, exists := r.autoMergeOverrides[identity]
	switch policy.Source {
	case AutoMergeSourceGlobal:
		if exists {
			return ErrAutoMergePolicyChanged
		}
	case AutoMergeSourceSession:
		if !exists || override.Enabled != policy.Enabled || override.Revision != policy.Revision {
			return ErrAutoMergePolicyChanged
		}
	default:
		return ErrAutoMergePolicyChanged
	}
	return nil
}

// insertLocked performs the actual insert. Caller must already hold r.mu.
func (r *memoryRepository) insertLocked(msg *QueuedMessage, maxPerSession int) error {
	list := r.entries[msg.SessionID]
	if maxPerSession > 0 && ordinaryQueueEntryCount(list) >= maxPerSession {
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

func ordinaryQueueEntryCount(entries []*QueuedMessage) int {
	count := 0
	for _, entry := range entries {
		if entry != nil && !entry.IsWorkflowControl() {
			count++
		}
	}
	return count
}

// RequeuePreservingFIFO inserts the entry at a position strictly lower than
// the current session head, so a superseded entry beats any new entry that
// arrives after the supersede was issued. Without this hook, the requeue
// landed at MAX+1 and a busy session starved the original message
// indefinitely — every turn-end drained the head, the superseded entry
// fell to the tail, and the next incoming message outranked it again.
//
// Decision under r.mu (caller must already hold it):
//   - existing entry with same (session_id, queued_by, coalesce_key)
//     → replace in place; preserve the existing entry's position and ID
//   - empty queue                → position = 1, fresh ID
//   - non-empty queue, no match  → position before the current head
//
// When MIN-1 is not positive, existing positions are shifted up before the
// insert. This keeps positions positive for TransferSession and future queue
// mutations while preserving the current order.
func (r *memoryRepository) RequeuePreservingFIFO(_ context.Context, msg *QueuedMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requeuePreservingFIFOLocked(msg, "")
}

func (r *memoryRepository) RequeuePreservingFIFOForSession(
	_ context.Context,
	identity QueueSessionIdentity,
	msg *QueuedMessage,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg == nil || msg.SessionID != identity.SessionID || msg.TaskID != identity.TaskID {
		return ErrSessionIdentityMismatch
	}
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	return r.requeuePreservingFIFOLocked(msg, identity.SessionIncarnationID)
}

func (r *memoryRepository) requeuePreservingFIFOLocked(
	msg *QueuedMessage,
	expectedIncarnationID string,
) error {
	if msg.IsReservedLifecycleDelivery() {
		return r.releaseLifecycleReservationForRetryLocked(msg, expectedIncarnationID)
	}
	list := r.entries[msg.SessionID]
	coalesceKey := metadataString(msg.Metadata, MetadataCoalesceKey)

	// Coalesce-replace only when the caller supplied a key. Reserved rows
	// belong to an active delivery and cannot be replacement targets.
	if coalesceKey != "" {
		for _, existing := range list {
			if existing.IsReservedInFlight() || existing.QueuedBy != msg.QueuedBy {
				continue
			}
			if metadataString(existing.Metadata, MetadataCoalesceKey) != coalesceKey {
				continue
			}
			existing.TaskID = msg.TaskID
			existing.Content = msg.Content
			existing.Model = msg.Model
			existing.PlanMode = msg.PlanMode
			existing.Attachments = msg.Attachments
			existing.Metadata = msg.Metadata
			if msg.QueuedAt.IsZero() {
				existing.QueuedAt = time.Now().UTC()
			} else {
				existing.QueuedAt = msg.QueuedAt
			}
			msg.ID = existing.ID
			msg.Position = existing.Position
			return nil
		}
	}
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.QueuedAt.IsZero() {
		msg.QueuedAt = time.Now().UTC()
	}
	msg.Position = r.nextRequeuePositionLocked(msg.SessionID, list)
	clone := *msg
	newList := make([]*QueuedMessage, 0, len(list)+1)
	newList = append(newList, &clone)
	newList = append(newList, list...)
	r.entries[msg.SessionID] = newList
	return nil
}

func (r *memoryRepository) releaseLifecycleReservationForRetryLocked(
	msg *QueuedMessage,
	expectedIncarnationID string,
) error {
	for index, existing := range r.entries[msg.SessionID] {
		if existing.ID != msg.ID || !existing.IsReservedInFlight() {
			continue
		}
		if expectedIncarnationID != "" &&
			lifecycleReservationIncarnation(existing.Metadata) != expectedIncarnationID {
			return ErrEntryNotFound
		}
		coalesceKey := metadataString(existing.Metadata, MetadataCoalesceKey)
		if r.hasPendingCoalescedSuccessorLocked(
			msg.SessionID,
			index,
			existing.QueuedBy,
			coalesceKey,
		) {
			r.removeEntryLocked(msg.SessionID, index)
			return nil
		}
		existing.Metadata = clearReservedMetadata(existing.Metadata)
		return nil
	}
	return ErrEntryNotFound
}

func (r *memoryRepository) hasPendingCoalescedSuccessorLocked(
	sessionID string,
	excludedIndex int,
	queuedBy, coalesceKey string,
) bool {
	if coalesceKey == "" {
		return false
	}
	for index, successor := range r.entries[sessionID] {
		if index == excludedIndex || successor.IsReservedInFlight() || successor.QueuedBy != queuedBy {
			continue
		}
		if metadataString(successor.Metadata, MetadataCoalesceKey) == coalesceKey {
			return true
		}
	}
	return false
}

func (r *memoryRepository) nextRequeuePositionLocked(sessionID string, list []*QueuedMessage) int64 {
	if len(list) == 0 {
		r.nextPosition[sessionID] = 1
		return 1
	}
	minPos := list[0].Position
	for _, entry := range list[1:] {
		if entry.Position < minPos {
			minPos = entry.Position
		}
	}
	position := minPos - 1
	if position <= 0 {
		shift := 1 - position
		for _, entry := range list {
			entry.Position += shift
		}
		position = 1
		r.nextPosition[sessionID] += shift
	}
	if position > r.nextPosition[sessionID] {
		r.nextPosition[sessionID] = position
	}
	return position
}

// AppendOrInsertTail must hold the lock for the entire check-then-insert path
// so two concurrent same-sender callers can't both observe "no matching tail"
// and race to insert separate entries (which would violate the
// append-or-insert semantics the SQLite repo achieves with a transaction).
func (r *memoryRepository) AppendOrInsertTail(_ context.Context, sessionID, taskID, content, model, queuedBy string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, maxPerSession int) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendOrInsertTailLocked(sessionID, taskID, content, model, queuedBy, planMode, attachments, metadata, maxPerSession)
}

func (r *memoryRepository) AppendOrInsertTailForSession(_ context.Context, identity QueueSessionIdentity, content, model, queuedBy string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, maxPerSession int) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, false, err
	}
	return r.appendOrInsertTailLocked(identity.SessionID, identity.TaskID, content, model, queuedBy, planMode, attachments, metadata, maxPerSession)
}

func (r *memoryRepository) appendOrInsertTailLocked(sessionID, taskID, content, model, queuedBy string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, maxPerSession int) (*QueuedMessage, bool, error) {
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
	return r.insertOrReplaceByCoalesceKeyLocked(msg, coalesceKey, maxPerSession, allowInsert)
}

func (r *memoryRepository) InsertOrReplaceByCoalesceKeyForSession(
	_ context.Context,
	identity QueueSessionIdentity,
	msg *QueuedMessage,
	coalesceKey string,
	maxPerSession int,
	allowInsert bool,
) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg == nil || msg.SessionID != identity.SessionID || msg.TaskID != identity.TaskID {
		return nil, false, ErrSessionIdentityMismatch
	}
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, false, err
	}
	return r.insertOrReplaceByCoalesceKeyLocked(msg, coalesceKey, maxPerSession, allowInsert)
}

func (r *memoryRepository) insertOrReplaceByCoalesceKeyLocked(
	msg *QueuedMessage,
	coalesceKey string,
	maxPerSession int,
	allowInsert bool,
) (*QueuedMessage, bool, error) {
	reservedMatch := false
	for _, existing := range r.entries[msg.SessionID] {
		if existing.QueuedBy != msg.QueuedBy {
			continue
		}
		if metadataString(existing.Metadata, MetadataCoalesceKey) != coalesceKey {
			continue
		}
		if existing.IsReservedInFlight() {
			reservedMatch = true
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
	if reservedMatch {
		maxPerSession = 0
	}
	if err := r.insertLocked(msg, maxPerSession); err != nil {
		return nil, false, err
	}
	return msg, false, nil
}
func (r *memoryRepository) InsertOrReplaceLifecycleByCoalesceKey(
	_ context.Context,
	msg *QueuedMessage,
	coalesceKey string,
	maxPerSession int,
	allowInsert bool,
) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insertOrReplaceLifecycleLocked(msg, coalesceKey, maxPerSession, allowInsert)
}

func (r *memoryRepository) InsertOrReplaceLifecycleByCoalesceKeyForSession(
	_ context.Context,
	identity QueueSessionIdentity,
	msg *QueuedMessage,
	coalesceKey string,
	maxPerSession int,
	allowInsert bool,
) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg == nil || msg.SessionID != identity.SessionID || msg.TaskID != identity.TaskID {
		return nil, false, ErrSessionIdentityMismatch
	}
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, false, err
	}
	return r.insertOrReplaceLifecycleLocked(msg, coalesceKey, maxPerSession, allowInsert)
}

func (r *memoryRepository) insertOrReplaceLifecycleLocked(
	msg *QueuedMessage,
	coalesceKey string,
	maxPerSession int,
	allowInsert bool,
) (*QueuedMessage, bool, error) {
	expected, ok := lifecycleGenerationFromMetadata(msg.Metadata)
	if !ok || expected != r.generation[msg.TaskID] {
		return nil, false, ErrLifecycleCancelled
	}
	reservedMatch := false
	for _, existing := range r.entries[msg.SessionID] {
		if existing.QueuedBy != msg.QueuedBy || metadataString(existing.Metadata, MetadataCoalesceKey) != coalesceKey {
			continue
		}
		if existing.IsReservedInFlight() {
			reservedMatch = true
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
	if reservedMatch {
		maxPerSession = 0
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

// ListDurableLifecycleEntries returns durable lifecycle rows in stable FIFO
// order across sessions. The startup sweep uses this view to find queue rows
// left by a crash between queue admission and attempt persistence.
func (r *memoryRepository) ListDurableLifecycleEntries(_ context.Context) ([]QueuedMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []QueuedMessage
	for _, list := range r.entries {
		for _, msg := range list {
			if msg.IsDurableLifecycle() {
				out = append(out, *msg)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SessionID == out[j].SessionID {
			return out[i].Position < out[j].Position
		}
		return out[i].SessionID < out[j].SessionID
	})
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
	return r.reserveHeadLocked(sessionID, nil, false), nil
}

func (r *memoryRepository) reserveHeadLocked(
	sessionID string,
	identity *QueueSessionIdentity,
	retainOrdinary bool,
) *QueuedMessage {
	list := r.entries[sessionID]
	if len(list) == 0 {
		return nil
	}
	head := list[0]
	out := *head
	if head.IsDurableLifecycle() || retainOrdinary {
		reservationIncarnation := lifecycleReservationIncarnation(head.Metadata)
		if identity != nil && reservationIncarnation != "" &&
			reservationIncarnation != identity.SessionIncarnationID {
			r.removeEntryLocked(sessionID, 0)
			return r.reserveHeadLocked(sessionID, identity, retainOrdinary)
		}
		out.Metadata = clearReservedMetadata(head.Metadata)
		out.reservedLifecycleDelivery = head.IsDurableLifecycle()
		if identity != nil {
			out.reservationIdentity = *identity
			head.Metadata = markReservedMetadataForIncarnation(out.Metadata, identity.SessionIncarnationID)
		} else {
			head.Metadata = markReservedMetadata(out.Metadata)
		}
		return &out
	}
	r.removeEntryLocked(sessionID, 0)
	return &out
}

func (r *memoryRepository) removeEntryLocked(sessionID string, index int) {
	list := r.entries[sessionID]
	r.entries[sessionID] = append(list[:index], list[index+1:]...)
	if len(r.entries[sessionID]) == 0 {
		delete(r.entries, sessionID)
		delete(r.nextPosition, sessionID)
	}
}

// GetAutoRun returns true when no explicit policy exists.
func (r *memoryRepository) GetAutoRun(_ context.Context, sessionID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.autoRunLocked(sessionID), nil
}

func (r *memoryRepository) autoRunLocked(sessionID string) bool {
	enabled, exists := r.autoRun[sessionID]
	return !exists || enabled
}
func (r *memoryRepository) autoRunForIdentityLocked(identity QueueSessionIdentity) bool {
	enabled, exists := r.autoRun[identity.SessionID]
	if !exists {
		return true
	}
	incarnationID := r.autoRunIncarnation[identity.SessionID]
	if incarnationID == "" {
		r.autoRunIncarnation[identity.SessionID] = identity.SessionIncarnationID
		return enabled
	}
	if incarnationID != identity.SessionIncarnationID {
		return true
	}
	return enabled
}

func identityPointer(identity QueueSessionIdentity) *QueueSessionIdentity {
	if identity.SessionIncarnationID == "" {
		return nil
	}
	return &identity
}

func (r *memoryRepository) setAutoRunLocked(
	identity *QueueSessionIdentity,
	sessionID string,
	enabled bool,
) {
	r.autoRun[sessionID] = enabled
	if identity == nil {
		r.autoRunIncarnation[sessionID] = ""
		return
	}
	r.autoRunIncarnation[sessionID] = identity.SessionIncarnationID
}

// SetAutoRun persists the per-session automatic-drain policy.
func (r *memoryRepository) SetAutoRun(_ context.Context, sessionID string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setAutoRunLocked(nil, sessionID, enabled)
	return nil
}

func (r *memoryRepository) SetAutoRunForSession(_ context.Context, identity QueueSessionIdentity, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	r.setAutoRunLocked(&identity, identity.SessionID, enabled)
	return nil
}

// PauseAutoRunIfPending persists OFF only while visible work remains.
func (r *memoryRepository) PauseAutoRunIfPending(_ context.Context, sessionID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pauseAutoRunIfPendingLocked(nil, sessionID), nil
}

func (r *memoryRepository) PauseAutoRunIfPendingForSession(
	_ context.Context,
	identity QueueSessionIdentity,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return false, err
	}
	return r.pauseAutoRunIfPendingLocked(&identity, identity.SessionID), nil
}

func (r *memoryRepository) pauseAutoRunIfPendingLocked(
	identity *QueueSessionIdentity,
	sessionID string,
) bool {
	for _, msg := range r.entries[sessionID] {
		if msg.IsReservedInFlight() {
			continue
		}
		r.setAutoRunLocked(identity, sessionID, false)
		return true
	}
	return false
}

// ReserveHeadIfAutoRun atomically checks policy and reserves the FIFO head.
func (r *memoryRepository) ReserveHeadIfAutoRun(_ context.Context, sessionID string) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.autoRunLocked(sessionID) {
		return nil, false, nil
	}
	return r.reserveHeadLocked(sessionID, nil, false), true, nil
}

func (r *memoryRepository) ReserveHeadIfAutoRunForSession(_ context.Context, identity QueueSessionIdentity) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, true, err
	}
	if !r.autoRunForIdentityLocked(identity) {
		return nil, false, nil
	}
	return r.reserveHeadLocked(identity.SessionID, &identity, false), true, nil
}

func (r *memoryRepository) ReserveHeadForDeliveryIfAutoRunForSession(
	_ context.Context,
	identity QueueSessionIdentity,
) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, true, err
	}
	if !r.autoRunForIdentityLocked(identity) {
		return nil, false, nil
	}
	return r.reserveHeadLocked(identity.SessionID, &identity, true), true, nil
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

func (r *memoryRepository) AcknowledgeByIDForSession(
	_ context.Context,
	identity QueueSessionIdentity,
	entryID string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	for index, msg := range r.entries[identity.SessionID] {
		if msg.ID != entryID {
			continue
		}
		if !msg.IsReservedInFlight() ||
			lifecycleReservationIncarnation(msg.Metadata) != identity.SessionIncarnationID {
			return ErrEntryNotFound
		}
		r.removeEntryLocked(identity.SessionID, index)
		return nil
	}
	return ErrEntryNotFound
}

func (r *memoryRepository) ReleaseDeliveryReservationForSession(
	_ context.Context,
	identity QueueSessionIdentity,
	entryID string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	for index, msg := range r.entries[identity.SessionID] {
		if msg.ID != entryID {
			continue
		}
		if !msg.IsReservedInFlight() ||
			lifecycleReservationIncarnation(msg.Metadata) != identity.SessionIncarnationID {
			return ErrEntryNotFound
		}
		coalesceKey := metadataString(msg.Metadata, MetadataCoalesceKey)
		if r.hasPendingCoalescedSuccessorLocked(
			identity.SessionID,
			index,
			msg.QueuedBy,
			coalesceKey,
		) {
			r.removeEntryLocked(identity.SessionID, index)
			return nil
		}
		msg.Metadata = clearReservedMetadata(msg.Metadata)
		return nil
	}
	return ErrEntryNotFound
}
func (r *memoryRepository) DiscardLifecycleReservation(
	_ context.Context,
	identity QueueSessionIdentity,
	entryID string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, msg := range r.entries[identity.SessionID] {
		if msg.ID != entryID {
			continue
		}
		if !msg.IsReservedInFlight() ||
			lifecycleReservationIncarnation(msg.Metadata) != identity.SessionIncarnationID {
			return ErrEntryNotFound
		}
		r.removeEntryLocked(identity.SessionID, index)
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
	return r.takeByIDLocked(sessionID, entryID), nil
}
func (r *memoryRepository) TakeByIDForSession(
	_ context.Context,
	identity QueueSessionIdentity,
	entryID string,
) (*QueuedMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, err
	}
	return r.takeByIDLocked(identity.SessionID, entryID), nil
}

func (r *memoryRepository) takeByIDLocked(sessionID, entryID string) *QueuedMessage {
	list, ok := r.entries[sessionID]
	if !ok {
		return nil
	}
	for i, message := range list {
		if message.ID != entryID {
			continue
		}
		r.entries[sessionID] = append(list[:i], list[i+1:]...)
		if len(r.entries[sessionID]) == 0 {
			delete(r.entries, sessionID)
			delete(r.nextPosition, sessionID)
		}
		out := *message
		return &out
	}
	return nil
}

// ClaimSendNow atomically claims the exact ordered source snapshot for a send-now dispatch.
func (r *memoryRepository) ClaimSendNow(_ context.Context, sessionID string, expected []QueuedMessage) (*SendNowClaim, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.claimSendNowLocked(r.identities[sessionID], sessionID, expected)
}

func (r *memoryRepository) ClaimSendNowForSession(_ context.Context, identity QueueSessionIdentity, expected []QueuedMessage) (*SendNowClaim, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, err
	}
	return r.claimSendNowLocked(identity, identity.SessionID, expected)
}

func (r *memoryRepository) claimSendNowLocked(
	identity QueueSessionIdentity,
	sessionID string,
	expected []QueuedMessage,
) (*SendNowClaim, error) {
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
	bindSendNowLifecycleReservations(sources, identity)
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
			entry.Metadata = markReservedMetadataForIncarnation(
				entry.Metadata,
				identity.SessionIncarnationID,
			)
			remaining = append(remaining, entry)
		}
	}
	if len(remaining) == 0 {
		delete(r.entries, sessionID)
		delete(r.nextPosition, sessionID)
	} else {
		r.entries[sessionID] = remaining
	}
	r.setAutoRunLocked(identityPointer(identity), sessionID, true)
	r.sendNowGeneration[sessionID]++
	return &SendNowClaim{
		Identity:            identity,
		OperationGeneration: r.sendNowGeneration[sessionID],
		Sources:             sources,
		Dispatch:            *envelope,
		SourceGenerations:   generations,
	}, nil
}

func (r *memoryRepository) validateSendNowClaimAuthority(
	ctx context.Context,
	claim *SendNowClaim,
) error {
	if claim.Identity.SessionIncarnationID == "" || r.authority == nil {
		return nil
	}
	current, err := r.authority(ctx, claim.Identity.TaskID, claim.Identity.SessionID)
	if err != nil || current != claim.Identity {
		return ErrSessionIdentityMismatch
	}
	return nil
}

func (r *memoryRepository) validateSendNowClaimLocked(claim *SendNowClaim) (string, error) {
	sessionID := claim.Sources[0].SessionID
	if claim.Identity.SessionIncarnationID != "" && r.identities[sessionID] != claim.Identity {
		return "", ErrSessionIdentityMismatch
	}
	if claim.OperationGeneration > 0 && r.sendNowGeneration[sessionID] != claim.OperationGeneration {
		return "", ErrSendNowClaimChanged
	}
	return sessionID, nil
}

// RestoreSendNowClaim puts every claimed source back at its original position.
func (r *memoryRepository) RestoreSendNowClaim(ctx context.Context, claim *SendNowClaim) error {
	if claim == nil || len(claim.Sources) == 0 {
		return ErrSendNowEmpty
	}
	if err := r.validateSendNowClaimAuthority(ctx, claim); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sessionID, err := r.validateSendNowClaimLocked(claim)
	if err != nil {
		return err
	}
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
func (r *memoryRepository) AcknowledgeSendNowClaim(ctx context.Context, claim *SendNowClaim) error {
	if claim == nil || len(claim.Sources) == 0 {
		return ErrSendNowEmpty
	}
	if err := r.validateSendNowClaimAuthority(ctx, claim); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sessionID, err := r.validateSendNowClaimLocked(claim)
	if err != nil {
		return err
	}
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

func cloneQueuedMessages(entries []*QueuedMessage) []QueuedMessage {
	clones := make([]QueuedMessage, 0, len(entries))
	for _, entry := range entries {
		clones = append(clones, *cloneQueuedMessage(entry))
	}
	return clones
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
	return r.updateContentAndMetadataLocked(sessionID, entryID, content, attachments, metadataUpdates, queuedBy)
}

func (r *memoryRepository) UpdateContentAndMetadataForSession(_ context.Context, identity QueueSessionIdentity, entryID, content string, attachments []MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	return r.updateContentAndMetadataLocked(identity.SessionID, entryID, content, attachments, metadataUpdates, queuedBy)
}

func (r *memoryRepository) UpdateContentAndMetadataForSessionWithClaim(_ context.Context, identity QueueSessionIdentity, entryID, content string, attachments []MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string, _ QueueAttachmentClaim) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	return r.updateContentAndMetadataLocked(identity.SessionID, entryID, content, attachments, metadataUpdates, queuedBy)
}

func (r *memoryRepository) updateContentAndMetadataLocked(sessionID, entryID, content string, attachments []MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error {
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
		if m.QueuedBy != queuedBy || m.IsReservedInFlight() {
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
	return r.mergeIntoAboveLocked(sessionID, sourceID, queuedBy)
}

func (r *memoryRepository) MergeIntoAboveForSession(_ context.Context, identity QueueSessionIdentity, sourceID, queuedBy string) (*QueuedMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, err
	}
	return r.mergeIntoAboveLocked(identity.SessionID, sourceID, queuedBy)
}

func (r *memoryRepository) mergeIntoAboveLocked(sessionID, sourceID, queuedBy string) (*QueuedMessage, error) {
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
	target.QueuedAt = latestQueuedAt(target.QueuedAt, source.QueuedAt)
	r.entries[sessionID] = append(list[:sourceIndex], list[sourceIndex+1:]...)
	if len(r.entries[sessionID]) == 0 {
		delete(r.nextPosition, sessionID)
	}
	merged := *target
	return &merged, nil
}

// AutoMergeIntoAbove folds one exact source into its immediate compatible
// predecessor. Incompatibility and missing candidates are successful skips.
func (r *memoryRepository) AutoMergeIntoAbove(_ context.Context, sessionID, sourceID string) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.autoMergeIntoAboveLocked(sessionID, sourceID)
}

func (r *memoryRepository) AutoMergeIntoAboveForSession(_ context.Context, identity QueueSessionIdentity, sourceID string) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, false, err
	}
	return r.autoMergeIntoAboveLocked(identity.SessionID, sourceID)
}

func (r *memoryRepository) AutoMergeIntoAboveForSessionWithPolicy(
	_ context.Context,
	identity QueueSessionIdentity,
	sourceID string,
	policy AutoMergePolicy,
) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, false, err
	}
	if err := r.validateAutoMergePolicyLocked(identity, policy); err != nil {
		return nil, false, err
	}
	return r.autoMergeIntoAboveLocked(identity.SessionID, sourceID)
}

func (r *memoryRepository) autoMergeIntoAboveLocked(sessionID, sourceID string) (*QueuedMessage, bool, error) {
	list := r.entries[sessionID]
	sourceIndex := -1
	for index, message := range list {
		if message.ID == sourceID {
			sourceIndex = index
			break
		}
	}
	if sourceIndex < 0 {
		return nil, false, nil
	}
	source := list[sourceIndex]
	var target *QueuedMessage
	for _, message := range list {
		if message.Position < source.Position && (target == nil || message.Position > target.Position) {
			target = message
		}
	}
	if target == nil {
		return cloneQueuedMessage(source), false, nil
	}
	values, compatible := buildAutoMergedEntry(target, source)
	if !compatible {
		return cloneQueuedMessage(source), false, nil
	}
	target.Content = values.content
	target.Attachments = values.attachments
	target.Metadata = values.metadata
	target.QueuedAt = values.queuedAt
	r.entries[sessionID] = append(list[:sourceIndex], list[sourceIndex+1:]...)
	return cloneQueuedMessage(target), true, nil
}

// AutoMergeCandidateIntoAbove folds a not-yet-admitted candidate into the
// session's tail entry when compatible, without inserting it. Incompatibility
// and missing tails are successful skips.
func (r *memoryRepository) AutoMergeCandidateIntoAbove(_ context.Context, candidate *QueuedMessage) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.autoMergeCandidateIntoAboveLocked(candidate)
}

func (r *memoryRepository) AutoMergeCandidateIntoAboveForSession(_ context.Context, identity QueueSessionIdentity, candidate *QueuedMessage) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if candidate == nil || candidate.SessionID != identity.SessionID || candidate.TaskID != identity.TaskID {
		return nil, false, ErrSessionIdentityMismatch
	}
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, false, err
	}
	return r.autoMergeCandidateIntoAboveLocked(candidate)
}

func (r *memoryRepository) AutoMergeCandidateIntoAboveForSessionWithClaim(_ context.Context, identity QueueSessionIdentity, candidate *QueuedMessage, _ QueueAttachmentClaim) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if candidate == nil || candidate.SessionID != identity.SessionID || candidate.TaskID != identity.TaskID {
		return nil, false, ErrSessionIdentityMismatch
	}
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, false, err
	}
	return r.autoMergeCandidateIntoAboveLocked(candidate)
}

func (r *memoryRepository) AutoMergeCandidateIntoAboveForSessionWithPolicy(
	_ context.Context,
	identity QueueSessionIdentity,
	candidate *QueuedMessage,
	_ *QueueAttachmentClaim,
	policy AutoMergePolicy,
) (*QueuedMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if candidate == nil || candidate.SessionID != identity.SessionID || candidate.TaskID != identity.TaskID {
		return nil, false, ErrSessionIdentityMismatch
	}
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, false, err
	}
	if err := r.validateAutoMergePolicyLocked(identity, policy); err != nil {
		return nil, false, err
	}
	return r.autoMergeCandidateIntoAboveLocked(candidate)
}

func (r *memoryRepository) autoMergeCandidateIntoAboveLocked(candidate *QueuedMessage) (*QueuedMessage, bool, error) {
	list := r.entries[candidate.SessionID]
	var target *QueuedMessage
	for _, message := range list {
		if target == nil || message.Position > target.Position {
			target = message
		}
	}
	if target == nil {
		return nil, false, nil
	}
	values, compatible := buildAutoMergedEntry(target, candidate)
	if !compatible {
		return nil, false, nil
	}
	target.Content = values.content
	target.Attachments = values.attachments
	target.Metadata = values.metadata
	target.QueuedAt = values.queuedAt
	return cloneQueuedMessage(target), true, nil
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
	return r.reorderEntriesLocked(sessionID, orderedIDs)
}

func (r *memoryRepository) ReorderEntriesForSession(_ context.Context, identity QueueSessionIdentity, orderedIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	return r.reorderEntriesLocked(identity.SessionID, orderedIDs)
}

func (r *memoryRepository) reorderEntriesLocked(sessionID string, orderedIDs []string) error {
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
	return r.deleteByIDLocked(sessionID, entryID)
}

func (r *memoryRepository) DeleteByIDForSession(_ context.Context, identity QueueSessionIdentity, entryID string) (*QueueRemovalResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, err
	}
	return r.deleteByIDForSessionLocked(identity.SessionID, entryID)
}

func (r *memoryRepository) deleteByIDForSessionLocked(sessionID, entryID string) (*QueueRemovalResult, error) {
	list, ok := r.entries[sessionID]
	if !ok {
		return nil, ErrEntryNotFound
	}
	for i, message := range list {
		if message.ID != entryID {
			continue
		}
		if message.IsReservedInFlight() {
			return nil, ErrEntryNotFound
		}
		removed := cloneQueuedMessage(message)
		r.entries[sessionID] = append(list[:i], list[i+1:]...)
		retained := cloneQueuedMessages(r.entries[sessionID])
		if len(r.entries[sessionID]) == 0 {
			delete(r.entries, sessionID)
			delete(r.nextPosition, sessionID)
		}
		return &QueueRemovalResult{
			Removed:  []QueuedMessage{*removed},
			Retained: retained,
		}, nil
	}
	return nil, ErrEntryNotFound
}

func (r *memoryRepository) deleteByIDLocked(sessionID, entryID string) error {
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
	return r.deleteAllBySessionLocked(sessionID)
}

func (r *memoryRepository) DeleteAllBySessionForIdentity(_ context.Context, identity QueueSessionIdentity) (*QueueRemovalResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, err
	}
	return r.deleteAllBySessionForIdentityLocked(identity.SessionID), nil
}

func (r *memoryRepository) deleteAllBySessionForIdentityLocked(sessionID string) *QueueRemovalResult {
	list := r.entries[sessionID]
	kept := list[:0]
	result := &QueueRemovalResult{}
	for _, message := range list {
		cloned := cloneQueuedMessage(message)
		if message.IsReservedInFlight() {
			kept = append(kept, message)
			result.Retained = append(result.Retained, *cloned)
			continue
		}
		result.Removed = append(result.Removed, *cloned)
	}
	if len(kept) == 0 {
		delete(r.entries, sessionID)
		delete(r.nextPosition, sessionID)
	} else {
		r.entries[sessionID] = kept
	}
	return result
}

func (r *memoryRepository) deleteAllBySessionLocked(sessionID string) (int, error) {
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

func (r *memoryRepository) PurgeDeletedSession(
	_ context.Context,
	identity QueueSessionIdentity,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, exists := r.identities[identity.SessionID]; exists && current != identity {
		return nil
	}
	r.clearSessionStateLocked(identity.SessionID)
	return nil
}

// TransferSession moves all entries (and any pending move) from one session to another.
func (r *memoryRepository) TransferSession(_ context.Context, oldSessionID, newSessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.transferSessionLocked(nil, nil, oldSessionID, newSessionID)
}

func (r *memoryRepository) TransferSessionIdentities(_ context.Context, source, destination QueueSessionIdentity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if source.TaskID != destination.TaskID {
		return ErrSessionIdentityMismatch
	}
	if err := r.bindIdentityLocked(source); err != nil {
		return err
	}
	if err := r.bindIdentityLocked(destination); err != nil {
		return err
	}
	return r.transferSessionLocked(&source, &destination, source.SessionID, destination.SessionID)
}

func (r *memoryRepository) transferSessionLocked(
	source, destination *QueueSessionIdentity,
	oldSessionID, newSessionID string,
) error {
	if oldSessionID == newSessionID {
		return nil
	}
	for _, entry := range r.entries[oldSessionID] {
		if entry.IsReservedInFlight() {
			return ErrQueueChanged
		}
	}
	sourceAutoRun := r.autoRunLocked(oldSessionID)
	destinationAutoRun := r.autoRunLocked(newSessionID)
	if source != nil && destination != nil {
		sourceAutoRun = r.autoRunForIdentityLocked(*source)
		destinationAutoRun = r.autoRunForIdentityLocked(*destination)
	}
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
		if destination, exists := r.identities[newSessionID]; exists {
			move.SessionIncarnationID = destination.SessionIncarnationID
			move.TaskID = destination.TaskID
		}
		r.pendingMoves[newSessionID] = move
		delete(r.pendingMoves, oldSessionID)
	}
	r.setAutoRunLocked(destination, newSessionID, sourceAutoRun && destinationAutoRun)
	delete(r.autoRun, oldSessionID)
	delete(r.autoRunIncarnation, oldSessionID)
	delete(r.identities, oldSessionID)
	return nil
}

// ReplaceSession replaces a session's queue with the supplied snapshot.
func (r *memoryRepository) ReplaceSession(_ context.Context, sessionID string, entries []QueuedMessage, pendingMove *PendingMove) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.replaceSessionLocked(sessionID, entries, pendingMove)
}

func (r *memoryRepository) ReplaceSessionForIdentity(_ context.Context, identity QueueSessionIdentity, entries []QueuedMessage, pendingMove *PendingMove) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := validateReplacementSnapshot(&identity, entries, pendingMove); err != nil {
		return err
	}
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	return r.replaceSessionLocked(identity.SessionID, entries, pendingMove)
}

func (r *memoryRepository) replaceSessionLocked(sessionID string, entries []QueuedMessage, pendingMove *PendingMove) error {
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
	if move == nil || move.TaskID == "" || sessionID == "" {
		return ErrSessionIdentityMismatch
	}
	if move.SessionIncarnationID == "" {
		if identity, ok := r.identities[sessionID]; ok {
			if identity.TaskID != move.TaskID {
				return ErrSessionIdentityMismatch
			}
			move.SessionIncarnationID = identity.SessionIncarnationID
		} else {
			move.SessionIncarnationID = "memory:" + sessionID
			r.identities[sessionID] = QueueSessionIdentity{
				TaskID: move.TaskID, SessionID: sessionID, SessionIncarnationID: move.SessionIncarnationID,
			}
		}
	}
	if _, ok := r.identities[sessionID]; !ok {
		r.identities[sessionID] = QueueSessionIdentity{
			TaskID: move.TaskID, SessionID: sessionID, SessionIncarnationID: move.SessionIncarnationID,
		}
	}
	if err := r.bindIdentityLocked(QueueSessionIdentity{
		TaskID: move.TaskID, SessionID: sessionID, SessionIncarnationID: move.SessionIncarnationID,
	}); err != nil {
		return err
	}
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
