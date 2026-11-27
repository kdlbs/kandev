package sqlite

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func BenchmarkDeliveryRetentionReplacement(b *testing.B) {
	benchmarkRetentionOwnerChange(b, "replacement")
}

func BenchmarkDeliveryRetentionDeletion(b *testing.B) {
	benchmarkRetentionOwnerChange(b, "deletion")
}

func benchmarkRetentionOwnerChange(b *testing.B, mode string) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		b.Run(engine, func(b *testing.B) { runRetentionBenchmarkBackground(b, engine, true, mode) })
	}
}

func startRetentionBenchmarkOwnerChange(b *testing.B, repo *Repository, mode string) func() {
	if mode == "" {
		return func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var operations atomic.Int64
	go func() {
		defer close(done)
		for index := 0; ctx.Err() == nil; index++ {
			err := runRetentionBenchmarkOwnerChange(ctx, repo, mode, index)
			if err != nil {
				if ctx.Err() == nil {
					b.Errorf("concurrent %s: %v", mode, err)
				}
				return
			}
			operations.Add(1)
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return func() {
		cancel()
		<-done
		if operations.Load() == 0 {
			b.Errorf("no concurrent %s operation completed", mode)
		}
		b.ReportMetric(float64(operations.Load()), mode+"-operations")
	}
}

func runRetentionBenchmarkOwnerChange(ctx context.Context, repo *Repository, mode string, index int) error {
	session := &models.TaskSession{ID: fmt.Sprintf("retention-benchmark-churn-%d", index), TaskID: "retention-benchmark-task", QueueIncarnationID: fmt.Sprintf("retention-benchmark-churn-incarnation-%d", index), State: models.TaskSessionStateCreated}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		return err
	}
	if mode == "replacement" {
		running := &models.ExecutorRunning{ID: session.ID, SessionID: session.ID, TaskID: session.TaskID, ExecutorID: "retention-benchmark-churn-executor", AgentExecutionID: "first", Status: "running"}
		if err := repo.UpsertExecutorRunning(ctx, running); err != nil {
			return err
		}
		running.AgentExecutionID = "second"
		if err := repo.UpsertExecutorRunning(ctx, running); err != nil {
			return err
		}
		stored, err := repo.GetExecutorRunningBySessionID(ctx, session.ID)
		if err != nil {
			return err
		}
		if stored.AgentExecutionID != "second" {
			return fmt.Errorf("replacement did not persist the new owner: %s", stored.AgentExecutionID)
		}
	}
	return repo.DeleteTaskSession(ctx, session)
}
