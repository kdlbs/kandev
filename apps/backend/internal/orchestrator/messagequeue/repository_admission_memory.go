package messagequeue

import (
	"context"
	"errors"
	"time"
)

func (r *memoryRepository) LookupQueueAdmission(
	_ context.Context,
	identity QueueSessionIdentity,
	clientQueueID string,
	candidate *QueuedMessage,
) (*QueuedMessage, bool, error) {
	if err := validateQueueAdmissionInput(identity, clientQueueID, candidate); err != nil {
		return nil, false, err
	}
	fingerprint, err := queueAdmissionFingerprint(identity, candidate)
	if err != nil {
		return nil, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, false, err
	}
	key := queueAdmissionKey{
		TaskID: identity.TaskID, SessionID: identity.SessionID,
		SessionIncarnationID: identity.SessionIncarnationID, ClientQueueID: clientQueueID,
	}
	return r.replayAdmissionLocked(key, fingerprint)
}

func (r *memoryRepository) AdmitQueueMessage(
	_ context.Context,
	identity QueueSessionIdentity,
	clientQueueID string,
	candidate *QueuedMessage,
	claim *QueueAttachmentClaim,
	maxPerSession int,
	policy *AutoMergePolicy,
) (*QueuedMessage, bool, error) {
	if err := validateQueueAdmissionInput(identity, clientQueueID, candidate); err != nil {
		return nil, false, err
	}
	fingerprint, err := queueAdmissionFingerprint(identity, candidate)
	if err != nil {
		return nil, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return nil, false, err
	}
	key := queueAdmissionKey{
		TaskID: identity.TaskID, SessionID: identity.SessionID,
		SessionIncarnationID: identity.SessionIncarnationID, ClientQueueID: clientQueueID,
	}
	if replay, found, err := r.replayAdmissionLocked(key, fingerprint); found {
		return replay, true, err
	}
	if r.queueIDExistsLocked(clientQueueID) {
		return nil, false, ErrQueueIDConflict
	}
	message := cloneQueuedMessage(candidate)
	message.ID = clientQueueID
	return r.admitNewAdmissionLocked(
		identity, key, fingerprint, message, claim, maxPerSession, policy,
	)
}

func (r *memoryRepository) replayAdmissionLocked(
	key queueAdmissionKey,
	fingerprint string,
) (*QueuedMessage, bool, error) {
	receipt, ok := r.admissionReceipts[key]
	if !ok {
		return nil, false, nil
	}
	if receipt.Fingerprint != fingerprint {
		return nil, false, ErrQueueIDConflict
	}
	return cloneQueuedMessage(receipt.Message), true, nil
}

func (r *memoryRepository) queueIDExistsLocked(clientQueueID string) bool {
	for _, entries := range r.entries {
		for _, entry := range entries {
			if entry.ID == clientQueueID {
				return true
			}
		}
	}
	return false
}

func (r *memoryRepository) admitNewAdmissionLocked(
	identity QueueSessionIdentity,
	key queueAdmissionKey,
	fingerprint string,
	message *QueuedMessage,
	claim *QueueAttachmentClaim,
	maxPerSession int,
	policy *AutoMergePolicy,
) (*QueuedMessage, bool, error) {
	if message.QueuedAt.IsZero() {
		message.QueuedAt = time.Now().UTC()
	}
	if len(r.entries[identity.SessionID]) >= maxPerSession && maxPerSession > 0 {
		if claim != nil && len(claim.IDs) > 0 {
			return nil, false, ErrQueueFull
		}
		return r.admitFullAdmissionLocked(key, fingerprint, message, policy)
	}
	if claim != nil && len(claim.IDs) > 0 {
		return nil, false, ErrQueueAdmissionUnavailable
	}
	if err := r.insertLocked(message, maxPerSession); err != nil {
		return nil, false, err
	}
	accepted, err := r.mergeInsertedAdmissionLocked(identity.SessionID, message, policy)
	if err != nil {
		return nil, false, err
	}
	return r.rememberAdmissionLocked(key, fingerprint, accepted)
}

func (r *memoryRepository) admitFullAdmissionLocked(
	key queueAdmissionKey,
	fingerprint string,
	message *QueuedMessage,
	policy *AutoMergePolicy,
) (*QueuedMessage, bool, error) {
	if policy == nil || !policy.Enabled {
		return nil, false, ErrQueueFull
	}
	merged, didMerge, err := r.autoMergeCandidateIntoAboveLocked(message)
	if err != nil {
		return nil, false, err
	}
	if !didMerge {
		return nil, false, ErrQueueFull
	}
	return r.rememberAdmissionLocked(key, fingerprint, merged)
}

func (r *memoryRepository) mergeInsertedAdmissionLocked(
	sessionID string,
	message *QueuedMessage,
	policy *AutoMergePolicy,
) (*QueuedMessage, error) {
	if policy == nil || !policy.Enabled {
		return message, nil
	}
	merged, didMerge, err := r.autoMergeIntoAboveLocked(sessionID, message.ID)
	if err != nil {
		return nil, err
	}
	if didMerge {
		return merged, nil
	}
	return message, nil
}

func (r *memoryRepository) rememberAdmissionLocked(
	key queueAdmissionKey,
	fingerprint string,
	message *QueuedMessage,
) (*QueuedMessage, bool, error) {
	if message == nil {
		return nil, false, errors.New("queue admission message is nil")
	}
	snapshot := queueAdmissionResponseSnapshot(message)
	r.admissionReceipts[key] = queueAdmissionReceipt{Fingerprint: fingerprint, Message: snapshot}
	return cloneQueuedMessage(message), false, nil
}
