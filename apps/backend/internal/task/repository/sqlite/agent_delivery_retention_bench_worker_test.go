package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/deliveryretention"
	"github.com/kandev/kandev/internal/task/models"
)

func startRetentionBenchmarkCleanup(b *testing.B, repo *Repository, active bool) func() {
	event := seedRetentionBenchmarkBacklog(b, repo)
	if !active {
		return func() {
			b.ReportMetric(float64(retentionBenchmarkEligibleBacklog(b, repo, event.StreamID)), "eligible-backlog-left")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(0)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			_, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{MaxCandidates: deliveryretention.BackgroundBatchRows})
			if err != nil && ctx.Err() == nil && !errors.Is(err, context.DeadlineExceeded) {
				b.Errorf("concurrent cleanup: %v", err)
				return
			}
			timer.Reset(100 * time.Millisecond)
		}
	}()
	return func() {
		cancel()
		<-done
		cursor, err := repo.GetAgentDeliveryCursor(context.Background(), event.StreamID)
		if err != nil {
			b.Error(err)
			return
		}
		remaining := retentionBenchmarkEligibleBacklog(b, repo, event.StreamID)
		if cursor.RetiredSequence == 0 || remaining == 0 {
			b.Errorf("cleanup-active benchmark must retire rows and retain eligible backlog throughout measurement: %+v %v", cursor, err)
		}
		b.ReportMetric(float64(cursor.RetiredSequence), "cleanup-retired")
		b.ReportMetric(float64(remaining), "eligible-backlog-left")
	}
}

func setRetentionBenchmarkEventOwner(event *models.AgentDeliveryEvent) {
	event.TaskID = "retention-benchmark-task"
	event.ExecutionID = "retention-benchmark-execution"
	event.OwnerGeneration = 1
	event.ExpectedStreamID = event.StreamID
	event.SubmissionID = "prompt:retention-benchmark"
}
