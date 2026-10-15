package lifecycle

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type remoteInitialPromptRunner struct {
	*remotePassthroughRunner
	manager *Manager
	index   int
	count   int
}

func (r *remoteInitialPromptRunner) WriteStdin(processID, data string) error {
	id := uuid.Must(uuid.NewV7()).String()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.manager.updateRemotePassthroughRecord(ctx, r.execution, func(record *remotePassthroughRecord) {
		record.InitialPromptState = remotePromptUncertain
		record.InitialSubmissionIDs = append(record.InitialSubmissionIDs, id)
	}); err != nil {
		return err
	}
	receipt, err := r.writeStdinReceipt(processID, data, id)
	if err != nil {
		return err
	}
	r.index++
	if r.index == r.count {
		return r.manager.updateRemotePassthroughRecord(ctx, r.execution, func(record *remotePassthroughRecord) {
			record.InitialPromptState = remotePromptWritten
			record.InitialSubmissionSequence = receipt.Sequence
		})
	}
	return nil
}
