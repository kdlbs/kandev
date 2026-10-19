package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func BenchmarkDeliveryRetention(b *testing.B) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, active := range []bool{false, true} {
			b.Run(fmt.Sprintf("%s/cleanup_%t", engine, active), func(b *testing.B) {
				runRetentionBenchmark(b, engine, active)
			})
		}
	}
}

func runRetentionBenchmark(b *testing.B, engine string, active bool) {
	runRetentionBenchmarkBackground(b, engine, active, "")
}

func runRetentionBenchmarkBackground(b *testing.B, engine string, active bool, ownerChange string) {
	repo := newRetentionBenchmarkRepo(b, engine)
	session := createRetentionBenchmarkSession(b, repo)
	prepareRetentionBenchmarkOwner(b, repo)
	stop := startRetentionBenchmarkCleanup(b, repo, active)
	defer stop()
	stopOwnerChange := startRetentionBenchmarkOwnerChange(b, repo, ownerChange)
	defer stopOwnerChange()
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: strings.Repeat("x", 1024)})
	if err != nil {
		b.Fatal(err)
	}
	samples := make([]time.Duration, 0, b.N*500)
	b.ResetTimer()
	for run := 0; run < b.N; run++ {
		for index := 0; index < 500; index++ {
			sequence := int64(run*500 + index + 1)
			event := &models.AgentDeliveryEvent{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, StreamID: "retention-benchmark-stream", Sequence: sequence, EventType: streams.EventTypeMessageChunk, Payload: payload, ReceivedAt: time.Now().UTC()}
			setRetentionBenchmarkEventOwner(event)
			samples = append(samples, benchmarkRetentionEvent(b, repo, event))
		}
	}
	b.StopTimer()
	b.ReportMetric(retentionBenchmarkBacklog, "initial-inbox-events")
	b.ReportMetric(float64(repo.db.Stats().MaxOpenConnections), "writer-pool-limit")
	b.ReportMetric(float64(repo.ro.Stats().MaxOpenConnections), "reader-pool-limit")
	writeRetentionBenchmarkSamples(b, samples)
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	b.ReportMetric(float64(samples[(len(samples)-1)*95/100].Nanoseconds())/1000, "p95-us/event")
	b.ReportMetric(float64(len(samples)), "samples")
	var canonicalBytes, prompts int64
	if err := repo.db.QueryRow(repo.db.Rebind(`SELECT COALESCE(SUM(LENGTH(content)),0) FROM task_session_messages WHERE task_session_id=?`), session.ID).Scan(&canonicalBytes); err != nil {
		b.Fatal(err)
	}
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM agent_delivery_submissions`).Scan(&prompts); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(canonicalBytes), "canonical-bytes")
	b.ReportMetric(float64(prompts), "prompt-summaries")
}

func createRetentionBenchmarkSession(b testing.TB, repo *Repository) *models.TaskSession {
	b.Helper()
	ctx := context.Background()
	var workspaceID string
	if err := repo.db.Get(&workspaceID, `SELECT id FROM workspaces ORDER BY created_at LIMIT 1`); err != nil {
		b.Fatal(err)
	}
	task := &models.Task{ID: "retention-benchmark-task", WorkspaceID: workspaceID, Title: "Retention benchmark"}
	if err := repo.CreateTask(ctx, task); err != nil {
		b.Fatal(err)
	}
	session := &models.TaskSession{ID: "retention-benchmark-session", TaskID: task.ID, QueueIncarnationID: "retention-benchmark-incarnation", State: models.TaskSessionStateCreated}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		b.Fatal(err)
	}
	return session
}

func benchmarkRetentionEvent(b *testing.B, repo *Repository, event *models.AgentDeliveryEvent) time.Duration {
	b.Helper()
	ctx := context.Background()
	started := time.Now()
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence); err != nil || !inserted {
		b.Fatalf("receive: %v %v", inserted, err)
	}
	now := event.ReceivedAt
	effect := &models.AgentDeliveryEffect{EffectKey: fmt.Sprintf("agent_delivery.event:%s:%d", event.StreamID, event.Sequence), StreamID: event.StreamID, Sequence: event.Sequence, EffectType: "agent_delivery.event", State: models.DeliveryEffectCompleted, CreatedAt: now, CompletedAt: &now}
	if _, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, effect); err != nil {
		b.Fatal(err)
	}
	return time.Since(started)
}

func newRetentionBenchmarkRepo(b *testing.B, engine string) *Repository {
	b.Helper()
	var writer *sqlx.DB
	var reader *sqlx.DB
	if engine == "Postgres" {
		writer = testutil.OpenIsolatedPostgres(b, testutil.PostgresDSNFromEnv(b))
		writer.SetMaxOpenConns(25)
		writer.SetMaxIdleConns(5)
		reader = writer
	} else {
		path := filepath.Join(b.TempDir(), "retention.db")
		raw, err := db.OpenSQLite(path)
		if err != nil {
			b.Fatal(err)
		}
		writer = sqlx.NewDb(raw, "sqlite3")
		b.Cleanup(func() { _ = writer.Close() })
		readOnly, err := db.OpenSQLiteReader(path)
		if err != nil {
			b.Fatal(err)
		}
		reader = sqlx.NewDb(readOnly, "sqlite3")
		b.Cleanup(func() { _ = reader.Close() })
	}
	repo, err := NewWithDB(writer, reader, nil)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := messagequeue.NewSQLiteRepository(writer, writer); err != nil {
		b.Fatal(err)
	}
	return repo
}

func prepareRetentionBenchmarkOwner(b testing.TB, repo *Repository) {
	b.Helper()
	ctx := context.Background()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "retention-benchmark-running", SessionID: "retention-benchmark-session", TaskID: "retention-benchmark-task", ExecutorID: "retention-benchmark-executor", AgentExecutionID: "retention-benchmark-execution", Status: "running"}); err != nil {
		b.Fatal(err)
	}
	now := time.Now().UTC()
	if stored, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: "retention-benchmark-session", IncarnationID: "retention-benchmark-incarnation", Generation: 1, NativeSessionID: "native-retention-benchmark", CreationReason: "initial", CreatedAt: now, CommittedAt: now}, 0); err != nil || !stored {
		b.Fatalf("benchmark harness: %v %v", stored, err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "prompt:retention-benchmark", SessionID: "retention-benchmark-session", IncarnationID: "retention-benchmark-incarnation", HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "benchmark-prompt-hash", Payload: []byte("benchmark prompt"), State: models.DeliverySubmissionDispatching}); err != nil {
		b.Fatal(err)
	}
}
