package deliveryretention

import "github.com/kandev/kandev/internal/task/models"

type discoveryProgress struct {
	initialized bool
	high        models.AgentDeliveryRetentionDiscovery
}

func (p *discoveryProgress) advance(before, after models.AgentDeliveryRetentionDiscovery) bool {
	if !p.initialized {
		p.high = before
		p.initialized = true
	}
	advanced := false
	if after.CompactionValid && (!p.high.CompactionValid || after.CompactionAt.After(p.high.CompactionAt) ||
		(after.CompactionAt.Equal(p.high.CompactionAt) && frontierAfter(after.CompactionStreamID, after.CompactionSequence, p.high.CompactionStreamID, p.high.CompactionSequence))) {
		p.high.CompactionAt = after.CompactionAt
		p.high.CompactionValid = true
		p.high.CompactionStreamID = after.CompactionStreamID
		p.high.CompactionSequence = after.CompactionSequence
		advanced = true
	}
	if frontierAfter(after.ReceiptStreamID, after.ReceiptSequence, p.high.ReceiptStreamID, p.high.ReceiptSequence) {
		p.high.ReceiptStreamID = after.ReceiptStreamID
		p.high.ReceiptSequence = after.ReceiptSequence
		advanced = true
	}
	if after.SubmissionID > p.high.SubmissionID {
		p.high.SubmissionID = after.SubmissionID
		advanced = true
	}
	return advanced
}

func frontierAfter(stream string, sequence int64, previousStream string, previousSequence int64) bool {
	return stream > previousStream || (stream == previousStream && sequence > previousSequence)
}
