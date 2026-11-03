package messagequeue

import "context"

func (r *memoryRepository) RecoveryRead(ctx context.Context, identity QueueSessionIdentity) (RepositorySnapshot, error) {
	snapshots, err := r.RecoveryReadBatch(ctx, []QueueSessionIdentity{identity})
	if err != nil {
		return RepositorySnapshot{}, err
	}
	return snapshots[0], nil
}

func (r *memoryRepository) RecoveryReadBatch(ctx context.Context, identities []QueueSessionIdentity) ([]RepositorySnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(identities) > 256 {
		return nil, ErrRecoveryLimit
	}
	for _, identity := range identities {
		if r.authority != nil {
			current, err := r.authority(ctx, identity.TaskID, identity.SessionID)
			if err != nil {
				return nil, err
			}
			if current != identity {
				return nil, ErrSessionIdentityMismatch
			}
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]RepositorySnapshot, 0, len(identities))
	total := 0
	for _, identity := range identities {
		snapshot, err := r.recoveryReadLocked(identity, 32*1024*1024-total)
		if err != nil {
			return nil, err
		}
		size, err := recoveryEntriesBytes(snapshot.Entries)
		if err != nil {
			return nil, err
		}
		total += size
		if total > 32*1024*1024 {
			return nil, ErrRecoveryLimit
		}
		result = append(result, snapshot)
	}
	return result, nil
}

func (r *memoryRepository) recoveryReadLocked(identity QueueSessionIdentity, remaining int) (RepositorySnapshot, error) {
	_, bound := r.identities[identity.SessionID]
	if !bound && r.authority != nil && len(r.entries[identity.SessionID]) == 0 {
		return RepositorySnapshot{Entries: []QueuedMessage{}, AutoRun: r.recoveryAutoRunLocked(identity)}, nil
	}
	if err := r.validateIdentityLocked(identity); err != nil {
		return RepositorySnapshot{}, err
	}
	entries := r.entries[identity.SessionID]
	if len(entries) > MaxRecoveryEntries {
		return RepositorySnapshot{}, ErrRecoveryLimit
	}
	size := 0
	for _, entry := range entries {
		if entry.TaskID != identity.TaskID || entry.SessionID != identity.SessionID {
			return RepositorySnapshot{}, ErrSessionIdentityMismatch
		}
		n, err := recoverySourceBytes(entry)
		if err != nil {
			return RepositorySnapshot{}, err
		}
		size += n
		if size > MaxRecoverySourceBytes || size > remaining {
			return RepositorySnapshot{}, ErrRecoveryLimit
		}
	}
	return RepositorySnapshot{Entries: cloneQueuedMessages(entries), AutoRun: r.recoveryAutoRunLocked(identity)}, nil
}

func (r *memoryRepository) recoveryAutoRunLocked(identity QueueSessionIdentity) bool {
	enabled, exists := r.autoRun[identity.SessionID]
	incarnation := r.autoRunIncarnation[identity.SessionID]
	if !exists || incarnation != "" && incarnation != identity.SessionIncarnationID {
		enabled = true
	}
	return enabled
}
