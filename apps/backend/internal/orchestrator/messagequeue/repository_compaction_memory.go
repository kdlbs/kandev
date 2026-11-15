package messagequeue

import "context"

func (r *memoryRepository) SetSessionPolicyAndCompact(_ context.Context, identity QueueSessionIdentity, change SessionPolicyChange, global GlobalQueuePolicy) (SessionPolicyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return SessionPolicyResult{}, err
	}
	merge, sortPolicy := r.autoMergeOverrides[identity], r.autoSortMergeOverrides[identity]
	var mergeOverride *AutoMergeOverride
	if _, ok := r.autoMergeOverrides[identity]; ok {
		mergeOverride = &merge
	}
	var sortOverride *AutoSortMergeOverride
	if _, ok := r.autoSortMergeOverrides[identity]; ok {
		sortOverride = &sortPolicy
	}
	before := SessionPolicyResult{AutoMerge: resolveAutoMergeOverride(mergeOverride, global.AutoMerge), AutoSortMerge: resolveAutoSortMergeOverride(sortOverride, global.AutoSortMerge)}
	if change.AutoSortMerge {
		sortPolicy.Enabled = change.Enabled
		sortPolicy.Revision++
		sortOverride = &sortPolicy
	} else {
		merge.Enabled = change.Enabled
		merge.Revision++
		mergeOverride = &merge
	}
	result := SessionPolicyResult{AutoMerge: resolveAutoMergeOverride(mergeOverride, global.AutoMerge), AutoSortMerge: resolveAutoSortMergeOverride(sortOverride, global.AutoSortMerge)}
	if (!before.AutoMerge.Enabled || !before.AutoSortMerge.Enabled) && result.AutoMerge.Enabled && result.AutoSortMerge.Enabled {
		result.CompactedCount = r.compactSessionQueueLocked(identity.SessionID, change.Leased)
		result.compactionRan = true
	}
	if change.AutoSortMerge {
		r.autoSortMergeOverrides[identity] = sortPolicy
	} else {
		r.autoMergeOverrides[identity] = merge
	}
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
