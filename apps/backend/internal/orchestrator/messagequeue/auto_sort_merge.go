package messagequeue

import (
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// AdmissionPolicy binds both policy decisions to one settings read.
// A nil sort policy means its override could not be read.
type AdmissionPolicy struct {
	AutoMerge       AutoMergePolicy
	AutoSortMerge   *AutoSortMergePolicy
	editLeaseBlocks func(string, string) bool
	log             *logger.Logger
}

func (p *AdmissionPolicy) sortedEnabled() bool {
	return p != nil && p.AutoMerge.Enabled && p.AutoSortMerge != nil && p.AutoSortMerge.Enabled
}

func isFoldBarrier(row *QueuedMessage) bool {
	return row.IsReservedInFlight() || row.IsDurableDelivery() ||
		row.QueuedBy == QueuedByWorkflow || row.QueuedBy == QueuedByServer || row.QueuedBy == QueuedByMoveTask
}

// Both repository row representations are scanned without copying the backlog.
func sortedFoldTarget[T QueuedMessage | *QueuedMessage](rows []T, source *QueuedMessage) (*QueuedMessage, int) {
	if source.QueuedBy != QueuedByAgent || isFoldBarrier(source) {
		return nil, 0
	}
	sender := metadataString(source.Metadata, MetadataSenderTaskID)
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
		if row.QueuedBy == QueuedByAgent && metadataString(row.Metadata, MetadataSenderTaskID) == sender {
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
	if policy != nil && policy.editLeaseBlocks != nil && policy.editLeaseBlocks(target.SessionID, target.ID) {
		return nil, nil
	}
	if blocked != nil {
		leased, err := blocked(target)
		if err != nil {
			return nil, err
		}
		if leased {
			return nil, nil
		}
	}
	values, compatible := buildAutoMergedEntry(target, source)
	if !compatible {
		return nil, nil
	}
	return values, nil
}
