package messagequeue

// SessionPolicyChange changes exactly one session preference.
type SessionPolicyChange struct {
	AutoSortMerge bool
	Enabled       bool
	Leased        map[string]struct{}
}

type compactionRow struct {
	original      *QueuedMessage
	originalIndex int
	merged        QueuedMessage
	dirty         bool
}

func (r *compactionRow) message() *QueuedMessage {
	if r.dirty {
		return &r.merged
	}
	return r.original
}

type queueCompaction struct {
	kept    []compactionRow
	removed []int
}

func compactQueueRows[T QueuedMessage | *QueuedMessage](rows []T, leased map[string]struct{}) queueCompaction {
	result := queueCompaction{kept: make([]compactionRow, 0, len(rows))}
	latest := make(map[string]int)
	for i := range rows {
		var source *QueuedMessage
		switch value := any(&rows[i]).(type) {
		case *QueuedMessage:
			source = value
		case **QueuedMessage:
			source = *value
		}
		if isFoldBarrier(source) {
			clear(latest)
			result.kept = append(result.kept, compactionRow{original: source, originalIndex: i})
			continue
		}
		sender := ""
		if source.QueuedBy == QueuedByAgent {
			sender = metadataString(source.Metadata, MetadataSenderTaskID)
		}
		_, sourceLeased := leased[source.ID]
		if !sourceLeased && result.fold(source, sender, latest, leased) {
			result.removed = append(result.removed, i)
			continue
		}
		if sender != "" {
			latest[sender] = len(result.kept)
		}
		result.kept = append(result.kept, compactionRow{original: source, originalIndex: i})
	}
	return result
}

func (c *queueCompaction) fold(source *QueuedMessage, sender string, latest map[string]int, leased map[string]struct{}) bool {
	sorted, exists := latest[sender]
	if sender != "" && exists && c.foldInto(sorted, source, leased) {
		return true
	}
	previous := len(c.kept) - 1
	if previous < 0 || (sender != "" && exists && previous == sorted) {
		return false
	}
	return c.foldInto(previous, source, leased)
}

func (c *queueCompaction) foldInto(index int, source *QueuedMessage, leased map[string]struct{}) bool {
	row := &c.kept[index]
	target := row.message()
	if _, blocked := leased[target.ID]; blocked || isFoldBarrier(target) {
		return false
	}
	values, compatible := buildAutoMergedEntry(target, source)
	if !compatible {
		return false
	}
	if !row.dirty {
		row.merged = *target
		row.dirty = true
	}
	row.merged.Content = values.content
	row.merged.Attachments = values.attachments
	row.merged.Metadata = values.metadata
	row.merged.QueuedAt = values.queuedAt
	return true
}
