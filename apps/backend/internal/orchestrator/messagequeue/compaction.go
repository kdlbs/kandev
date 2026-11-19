package messagequeue

// SessionPolicyChange changes exactly one session preference.
type SessionPolicyChange struct {
	Enabled bool
	Leased  map[string]struct{}
}

type compactionRow struct {
	original      *QueuedMessage
	originalIndex int
	merged        QueuedMessage
	dirty         bool
	// merge accumulates every fold into this row; the merged entry is
	// materialized once after the pass so each input is copied and validated
	// once rather than on every later fold.
	merge *autoMergeAccumulator
}

type queueCompaction struct {
	kept    []compactionRow
	removed []int
	fences  map[string]string
}

// fences maps a row ID to its task-management claim; rows fold only when
// their claims are identical, so a survivor never carries content admitted
// under a different claim generation.
func compactQueueRows[T QueuedMessage | *QueuedMessage](rows []T, leased map[string]struct{}, fences map[string]string) queueCompaction {
	result := queueCompaction{kept: make([]compactionRow, 0, len(rows)), fences: fences}
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
			// A fresh map keeps reset cost proportional to this segment; clear
			// would keep walking the largest earlier segment's buckets.
			latest = make(map[string]int)
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
	for i := range result.kept {
		if row := &result.kept[i]; row.dirty {
			values := row.merge.values()
			row.merged = *row.original
			row.merged.Content = values.content
			row.merged.Attachments = values.attachments
			row.merged.Metadata = values.metadata
			row.merged.QueuedAt = values.queuedAt
		}
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
	target := row.original
	if _, blocked := leased[target.ID]; blocked || isFoldBarrier(target) || c.fences[target.ID] != c.fences[source.ID] {
		return false
	}
	if row.merge == nil {
		merge, ok := startCompactionMerge(target, source)
		if !ok {
			return false
		}
		row.merge = merge
	} else if !row.merge.add(source) {
		return false
	}
	row.dirty = true
	return true
}
