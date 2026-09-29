package messagequeue

import (
	"context"
	"errors"
	"time"
)

//nolint:funlen // The in-memory adapter mirrors the durable admission contract for conformance tests.
func (r *memoryRepository) AdmitManagedInput(
	_ context.Context,
	identity QueueSessionIdentity,
	request ManagedInputRequest,
	maxPerSession int,
) (ManagedInputReceipt, bool, error) {
	if err := validateManagedInputRequest(identity, request); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	fingerprint, err := managedInputFingerprint(identity, request)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	message := managedInputQueueMessage(identity, request)
	queueFingerprint, err := queueAdmissionFingerprint(identity, message)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateIdentityLocked(identity); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	scope := managedInputScopeFor(identity)
	occurrenceKey := managedInputOccurrenceKey{Scope: scope, OccurrenceKey: request.OccurrenceKey}
	if inputKey, exists := r.managedOccurrences[occurrenceKey]; exists {
		receipt := r.managedInputs[inputKey]
		if receipt.fingerprint != fingerprint {
			return ManagedInputReceipt{}, false, ErrQueueIDConflict
		}
		return receipt, true, nil
	}
	if r.admissionReceiptExistsLocked(identity, request.OccurrenceKey) ||
		r.managedInputIDExistsLocked(request.ID) || r.queueIDExistsLocked(request.ID) {
		return ManagedInputReceipt{}, false, ErrQueueIDConflict
	}
	var replacement *ManagedInputReceipt
	if managedInputCanCoalesce(request) {
		replacement, _ = r.pendingPeriodicInputLocked(scope, request.CoalesceKey)
	}
	if replacement != nil {
		replacementKey := managedInputIDKey{Scope: scope, InputID: replacement.ID}
		replacementIndex := replacementIndexForID(r.entries[identity.SessionID], replacement.ID)
		if replacementIndex < 0 {
			return ManagedInputReceipt{}, false, ErrManagedInputNotPending
		}
		entries := r.entries[identity.SessionID]
		replacedEntry := entries[replacementIndex]
		remaining := make([]*QueuedMessage, 0, len(entries)-1)
		remaining = append(remaining, entries[:replacementIndex]...)
		remaining = append(remaining, entries[replacementIndex+1:]...)
		r.entries[identity.SessionID] = remaining
		if err := r.insertLocked(message, maxPerSession); err != nil {
			// The removed row freed one capacity slot, so insertion cannot fail
			// for queue size. Restore it if another repository invariant rejects
			// the replacement.
			restored := make([]*QueuedMessage, 0, len(entries))
			restored = append(restored, remaining[:replacementIndex]...)
			restored = append(restored, replacedEntry)
			restored = append(restored, remaining[replacementIndex:]...)
			r.entries[identity.SessionID] = restored
			return ManagedInputReceipt{}, false, err
		}
		old := r.managedInputs[replacementKey]
		old.State = ManagedInputStateSuperseded
		old.SupersededBy, old.UpdatedAt = request.ID, message.QueuedAt
		r.managedInputs[replacementKey] = old
	} else if err := r.insertLocked(message, maxPerSession); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	now := message.QueuedAt
	receipt := managedInputReceiptFromRequest(identity, request, r.nextManagedInputSequenceLocked(scope), now)
	receipt.fingerprint = fingerprint
	inputKey := managedInputIDKey{Scope: scope, InputID: request.ID}
	r.managedInputs[inputKey] = receipt
	r.managedOccurrences[occurrenceKey] = inputKey
	r.admissionReceipts[queueAdmissionKey{
		TaskID: identity.TaskID, SessionID: identity.SessionID,
		SessionIncarnationID: identity.SessionIncarnationID,
		ClientQueueID:        request.OccurrenceKey,
	}] = queueAdmissionReceipt{Fingerprint: queueFingerprint, Message: queueAdmissionResponseSnapshot(message)}
	return receipt, false, nil
}

func (r *memoryRepository) nextManagedInputSequenceLocked(scope managedInputScope) int64 {
	var sequence int64
	for key, receipt := range r.managedInputs {
		if key.Scope == scope && receipt.Sequence > sequence {
			sequence = receipt.Sequence
		}
	}
	return sequence + 1
}

func (r *memoryRepository) GetManagedInput(
	_ context.Context,
	identity QueueSessionIdentity,
	inputID string,
) (ManagedInputReceipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateIdentityLocked(identity); err != nil {
		return ManagedInputReceipt{}, err
	}
	receipt, exists := r.managedInputs[managedInputIDKey{Scope: managedInputScopeFor(identity), InputID: inputID}]
	if !exists {
		return ManagedInputReceipt{}, ErrEntryNotFound
	}
	return receipt, nil
}

func (r *memoryRepository) GetManagedInputByExecution(
	_ context.Context,
	identity QueueSessionIdentity,
	turnID, executionID string,
) (ManagedInputReceipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateIdentityLocked(identity); err != nil {
		return ManagedInputReceipt{}, err
	}
	for key, receipt := range r.managedInputs {
		if key.Scope == managedInputScopeFor(identity) &&
			receipt.TurnID == turnID && receipt.ExecutionID == executionID {
			return receipt, nil
		}
	}
	return ManagedInputReceipt{}, ErrEntryNotFound
}

func (r *memoryRepository) ListManagedInputs(
	_ context.Context,
	identity QueueSessionIdentity,
) ([]ManagedInputReceipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateIdentityLocked(identity); err != nil {
		return nil, err
	}
	scope := managedInputScopeFor(identity)
	receipts := make([]ManagedInputReceipt, 0)
	for key, receipt := range r.managedInputs {
		if key.Scope == scope {
			receipts = append(receipts, receipt)
		}
	}
	managedInputSort(receipts)
	return receipts, nil
}

func (r *memoryRepository) CancelManagedInput(
	_ context.Context,
	identity QueueSessionIdentity,
	inputID string,
) (ManagedInputReceipt, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateIdentityLocked(identity); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	key := managedInputIDKey{Scope: managedInputScopeFor(identity), InputID: inputID}
	receipt, exists := r.managedInputs[key]
	if !exists {
		return ManagedInputReceipt{}, false, ErrEntryNotFound
	}
	if receipt.State != ManagedInputStateAccepted {
		return receipt, false, nil
	}
	receipt.State = ManagedInputStateCancelled
	receipt.UpdatedAt = time.Now().UTC()
	r.managedInputs[key] = receipt
	r.removeManagedQueueRowLocked(identity.SessionID, inputID)
	return receipt, true, nil
}

func (r *memoryRepository) MarkManagedInputRunning(
	_ context.Context,
	identity QueueSessionIdentity,
	inputID, turnID, executionID string,
) (ManagedInputReceipt, bool, error) {
	if turnID == "" || executionID == "" {
		return ManagedInputReceipt{}, false, errors.New("managed input execution identity is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateIdentityLocked(identity); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	key := managedInputIDKey{Scope: managedInputScopeFor(identity), InputID: inputID}
	receipt, exists := r.managedInputs[key]
	if !exists {
		return ManagedInputReceipt{}, false, ErrEntryNotFound
	}
	if receipt.State == ManagedInputStateRunning && receipt.TurnID == turnID && receipt.ExecutionID == executionID {
		return receipt, false, nil
	}
	if receipt.State != ManagedInputStateAccepted || r.managedExecutionExistsLocked(key.Scope, turnID, executionID) {
		return ManagedInputReceipt{}, false, ErrManagedInputTransition
	}
	if r.managedInputRunningLocked(key.Scope) {
		return ManagedInputReceipt{}, false, ErrManagedInputBusy
	}
	rowIndex := replacementIndexForID(r.entries[identity.SessionID], inputID)
	if rowIndex < 0 {
		return ManagedInputReceipt{}, false, ErrManagedInputNotPending
	}
	if !isMemoryQueueHead(r.entries[identity.SessionID], inputID) {
		return ManagedInputReceipt{}, false, ErrManagedInputNotHead
	}
	receipt.State = ManagedInputStateRunning
	receipt.TurnID = turnID
	receipt.ExecutionID = executionID
	receipt.UpdatedAt = time.Now().UTC()
	r.managedInputs[key] = receipt
	entries := r.entries[identity.SessionID]
	r.entries[identity.SessionID] = append(entries[:rowIndex], entries[rowIndex+1:]...)
	return receipt, true, nil
}

func (r *memoryRepository) SettleManagedInput(
	_ context.Context,
	identity QueueSessionIdentity,
	inputID, turnID, executionID string,
	state ManagedInputState,
	outcome string,
) (ManagedInputReceipt, bool, error) {
	if err := validateManagedInputSettlement(turnID, executionID, state, outcome); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateIdentityLocked(identity); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	key := managedInputIDKey{Scope: managedInputScopeFor(identity), InputID: inputID}
	receipt, exists := r.managedInputs[key]
	if !exists {
		return ManagedInputReceipt{}, false, ErrEntryNotFound
	}
	if receipt.State == state && receipt.TurnID == turnID && receipt.ExecutionID == executionID && receipt.Outcome == outcome {
		return receipt, false, nil
	}
	if receipt.State == ManagedInputStateAccepted && state == ManagedInputStateUncertain &&
		turnID == "" && executionID == "" {
		receipt.State = ManagedInputStateUncertain
		receipt.Outcome = outcome
		receipt.UpdatedAt = time.Now().UTC()
		r.managedInputs[key] = receipt
		r.removeManagedQueueRowLocked(identity.SessionID, inputID)
		return receipt, true, nil
	}
	if receipt.State != ManagedInputStateRunning || receipt.TurnID != turnID || receipt.ExecutionID != executionID {
		return ManagedInputReceipt{}, false, ErrManagedInputTransition
	}
	receipt.State = state
	receipt.Outcome = outcome
	receipt.UpdatedAt = time.Now().UTC()
	r.managedInputs[key] = receipt
	return receipt, true, nil
}

func (r *memoryRepository) admissionReceiptExistsLocked(identity QueueSessionIdentity, occurrenceKey string) bool {
	key := queueAdmissionKey{
		TaskID: identity.TaskID, SessionID: identity.SessionID,
		SessionIncarnationID: identity.SessionIncarnationID, ClientQueueID: occurrenceKey,
	}
	_, exists := r.admissionReceipts[key]
	return exists
}

func (r *memoryRepository) managedInputIDExistsLocked(inputID string) bool {
	for key := range r.managedInputs {
		if key.InputID == inputID {
			return true
		}
	}
	return false
}

func (r *memoryRepository) pendingPeriodicInputLocked(
	scope managedInputScope,
	coalesceKey string,
) (*ManagedInputReceipt, bool) {
	var latest *ManagedInputReceipt
	for key, receipt := range r.managedInputs {
		if key.Scope != scope || receipt.State != ManagedInputStateAccepted ||
			receipt.Origin != ManagedInputOriginPeriodic || receipt.CoalesceKey != coalesceKey {
			continue
		}
		rowIndex := replacementIndexForID(r.entries[scope.SessionID], receipt.ID)
		if rowIndex < 0 || r.entries[scope.SessionID][rowIndex].IsReservedInFlight() ||
			r.entries[scope.SessionID][rowIndex].IsDeliveryAttempted() {
			continue
		}
		if latest == nil || receipt.Sequence > latest.Sequence ||
			(receipt.Sequence == latest.Sequence && receipt.CreatedAt.After(latest.CreatedAt)) {
			copy := receipt
			latest = &copy
		}
	}
	return latest, latest != nil
}

func replacementIndexForID(entries []*QueuedMessage, inputID string) int {
	for index, entry := range entries {
		if entry.ID == inputID {
			return index
		}
	}
	return -1
}

func (r *memoryRepository) removeManagedQueueRowLocked(sessionID, inputID string) {
	entries := r.entries[sessionID]
	index := replacementIndexForID(entries, inputID)
	if index >= 0 {
		r.entries[sessionID] = append(entries[:index], entries[index+1:]...)
	}
}

func isMemoryQueueHead(entries []*QueuedMessage, inputID string) bool {
	if len(entries) == 0 {
		return false
	}
	head := entries[0]
	for _, entry := range entries[1:] {
		if entry.Position < head.Position {
			head = entry
		}
	}
	return head.ID == inputID
}

func (r *memoryRepository) managedExecutionExistsLocked(
	scope managedInputScope,
	turnID, executionID string,
) bool {
	for key, receipt := range r.managedInputs {
		if key.Scope == scope && receipt.TurnID == turnID && receipt.ExecutionID == executionID {
			return true
		}
	}
	return false
}

func (r *memoryRepository) managedInputRunningLocked(scope managedInputScope) bool {
	for key, receipt := range r.managedInputs {
		if key.Scope == scope && receipt.State == ManagedInputStateRunning {
			return true
		}
	}
	return false
}
