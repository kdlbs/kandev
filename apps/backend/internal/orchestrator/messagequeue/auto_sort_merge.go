package messagequeue

import (
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// AdmissionPolicy binds automatic folding to one effective Auto-merge snapshot.
type AdmissionPolicy struct {
	AutoMerge AutoMergePolicy
	// leased is the session's in-process edit-lease set, captured under the
	// session admission gate before any repository lock is taken.
	leased map[string]struct{}
	log    *logger.Logger
}

func (p *AdmissionPolicy) sortedEnabled() bool {
	return p != nil && p.AutoMerge.Enabled
}

func isFoldBarrier(row *QueuedMessage) bool {
	return row.IsReservedInFlight() || row.IsDurableDelivery() ||
		row.QueuedBy == QueuedByWorkflow || row.QueuedBy == QueuedByServer || row.QueuedBy == QueuedByMoveTask
}

// sortedFoldSender returns the sender task of a source that may move to its
// latest same-sender entry, or "" when the source keeps FIFO adjacency only.
func sortedFoldSender(source *QueuedMessage) string {
	if source.QueuedBy != QueuedByAgent || isFoldBarrier(source) {
		return ""
	}
	return metadataString(source.Metadata, MetadataSenderTaskID)
}

func isSameFoldSender(row *QueuedMessage, sender string) bool {
	return row.QueuedBy == QueuedByAgent && metadataString(row.Metadata, MetadataSenderTaskID) == sender
}

func sortedFoldTarget[T QueuedMessage | *QueuedMessage](rows []T, source *QueuedMessage) (*QueuedMessage, int) {
	sender := sortedFoldSender(source)
	if sender == "" {
		return nil, 0
	}
	for i := len(rows) - 1; i >= 0; i-- {
		var row *QueuedMessage
		switch value := any(&rows[i]).(type) {
		case *QueuedMessage:
			row = value
		case **QueuedMessage:
			row = *value
		}
		if isFoldBarrier(row) {
			return nil, 0
		}
		if isSameFoldSender(row, sender) {
			return row, len(rows) - 1 - i
		}
	}
	return nil, 0
}

func autoFoldTarget[T QueuedMessage | *QueuedMessage](rows []T, source *QueuedMessage, policy *AdmissionPolicy, blocked func(*QueuedMessage) (bool, error)) (*QueuedMessage, *autoMergedValues, error) {
	var sorted *QueuedMessage
	crossed := 0
	if policy.sortedEnabled() {
		sorted, crossed = sortedFoldTarget(rows, source)
	}
	if sorted != nil {
		values, err := autoFoldValues(sorted, source, policy, blocked)
		if values != nil && crossed > 0 && policy.log != nil {
			policy.log.Debug("sorted automatic merge target selected", zap.String("session_id", source.SessionID), zap.String("source_id", source.ID), zap.String("target_id", sorted.ID), zap.Int("crossed", crossed))
		}
		if err != nil || values != nil {
			return sorted, values, err
		}
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}
	var adjacent *QueuedMessage
	switch value := any(&rows[len(rows)-1]).(type) {
	case *QueuedMessage:
		adjacent = value
	case **QueuedMessage:
		adjacent = *value
	}
	if adjacent == sorted {
		return nil, nil, nil
	}
	values, err := autoFoldValues(adjacent, source, policy, blocked)
	if values == nil {
		return nil, nil, err
	}
	return adjacent, values, err
}

func autoFoldValues(target, source *QueuedMessage, policy *AdmissionPolicy, blocked func(*QueuedMessage) (bool, error)) (*autoMergedValues, error) {
	values, compatible := buildAutoMergedEntry(target, source)
	if !compatible {
		return nil, nil
	}
	if policy != nil {
		if _, leased := policy.leased[target.ID]; leased {
			return nil, nil
		}
	}
	if blocked != nil {
		leased, err := blocked(target)
		if err != nil || leased {
			return nil, err
		}
	}
	return values, nil
}
