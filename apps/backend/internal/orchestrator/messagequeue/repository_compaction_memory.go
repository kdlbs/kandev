package messagequeue

import "context"

func (r *memoryRepository) SetSessionPolicyAndCompact(_ context.Context, identity QueueSessionIdentity, change SessionPolicyChange, global AutoMergePolicy, globalErr error) (SessionPolicyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return SessionPolicyResult{}, err
	}
	merge := r.autoMergeOverrides[identity]
	var mergeOverride *AutoMergeOverride
	if _, ok := r.autoMergeOverrides[identity]; ok {
		mergeOverride = &merge
	} else if change.Enabled && globalErr != nil {
		return SessionPolicyResult{}, globalErr
	}
	before := resolveAutoMergeOverride(mergeOverride, global)
	merge.Enabled = change.Enabled
	merge.Revision++
	result := SessionPolicyResult{AutoMerge: resolveAutoMergeOverride(&merge, global)}
	if !before.Enabled && result.AutoMerge.Enabled {
		result.CompactedCount = r.compactSessionQueueLocked(identity.SessionID, change.Leased)
		result.compactionRan = true
	}
	r.autoMergeOverrides[identity] = merge
	return result, nil
}

func (r *memoryRepository) compactSessionQueueLocked(sessionID string, leased map[string]struct{}) int {
	rows := r.entries[sessionID]
	compacted := compactQueueRows(rows, leased, nil)
	if len(compacted.removed) == 0 {
		return 0
	}
	for i := range compacted.kept {
		entry := &compacted.kept[i]
		if entry.dirty {
			*entry.original = entry.merged
		}
		rows[i] = entry.original
	}
	clear(rows[len(compacted.kept):])
	r.entries[sessionID] = rows[:len(compacted.kept)]
	return len(compacted.removed)
}
