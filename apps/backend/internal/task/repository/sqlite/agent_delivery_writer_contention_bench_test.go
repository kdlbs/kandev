package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// Each simulated session emits one event per tick and projects a batch every
// contentionBatchSize events, matching the stream processor's 20ms window.
const (
	contentionEventInterval         = 10 * time.Millisecond
	contentionBatchSize             = 2
	contentionForegroundGap         = 5 * time.Millisecond
	contentionForegroundSampleCount = 5
	durableDeliveryAckCadence       = 20 * time.Millisecond
	contentionQueueSize             = 8192
)

type contentionVariant struct {
	name string
	// synchronous forces the writer pragma before the timed section. The
	// baseline reproduces the pre-change FULL writer; the other variants run
	// with the NORMAL writer that the durability barrier covers.
	synchronous string
	// combined admits and projects each batch in one writer transaction.
	combined bool
	// ackBarrier simulates one ACK worker per stream awaiting the durability
	// barrier on the ACK cadence.
	ackBarrier bool
}

var contentionVariants = []contentionVariant{
	{name: "baseline", synchronous: "FULL"},
	{name: "current", synchronous: "NORMAL"},
	{name: "combined", synchronous: "NORMAL", combined: true},
	{name: "final", synchronous: "NORMAL", combined: true, ackBarrier: true},
}

// BenchmarkDeliveryWriterContention measures how long an ordinary user write
// waits on the single SQLite writer while agent sessions stream durable
// delivery events. Run with a fixed count, for example -benchtime=100x.
func BenchmarkDeliveryWriterContention(b *testing.B) {
	for _, variant := range contentionVariants {
		for _, sessions := range []int{0, 4, 8} {
			b.Run(fmt.Sprintf("%s/sessions_%d", variant.name, sessions), func(b *testing.B) {
				runWriterContentionBenchmark(b, variant, sessions)
			})
		}
	}
}

func runWriterContentionBenchmark(b *testing.B, variant contentionVariant, sessions int) {
	b.Helper()
	repo := newRetentionBenchmarkRepo(b, "SQLite")
	if variant.synchronous != "" {
		setContentionSynchronous(b, repo, variant.synchronous)
	}
	var workspaceID string
	if err := repo.db.Get(&workspaceID, `SELECT id FROM workspaces ORDER BY created_at LIMIT 1`); err != nil {
		b.Fatal(err)
	}
	streamsCtx, stopStreams := context.WithCancel(context.Background())
	var events, offered atomic.Int64
	var wg sync.WaitGroup
	errs := make(chan error, sessions*2)
	for index := 0; index < sessions; index++ {
		owner := prepareContentionSession(b, repo, workspaceID, index)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := streamContentionEvents(streamsCtx, repo, owner, variant, &events, &offered); err != nil {
				errs <- err
			}
		}()
		if variant.ackBarrier {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := simulateContentionAckWorker(streamsCtx, repo); err != nil {
					errs <- err
				}
			}()
		}
	}
	time.Sleep(200 * time.Millisecond)
	before := repo.db.Stats()
	eventsBefore := events.Load()
	offeredBefore := offered.Load()
	barrierBefore := barrierSuccesses()
	started := time.Now()
	samples := measureContentionForeground(b, repo, workspaceID)
	elapsed := time.Since(started)
	eventsAfter := events.Load()
	offeredAfter := offered.Load()
	barrierAfter := barrierSuccesses()
	after := repo.db.Stats()
	stopStreams()
	wg.Wait()
	close(errs)
	for err := range errs {
		b.Fatal(err)
	}
	reportContentionSamples(b, samples)
	projected := float64(eventsAfter - eventsBefore)
	offeredCount := float64(offeredAfter - offeredBefore)
	b.ReportMetric(projected/elapsed.Seconds(), "stream-events/s")
	b.ReportMetric(offeredCount/elapsed.Seconds(), "offered-events/s")
	if offeredCount > 0 {
		b.ReportMetric(projected/offeredCount*100, "delivery-%")
	} else {
		b.ReportMetric(0, "delivery-%")
	}
	b.ReportMetric(float64(after.WaitDuration-before.WaitDuration)/float64(time.Millisecond)/elapsed.Seconds(), "writer-wait-ms/s")
	b.ReportMetric((barrierAfter-barrierBefore)/elapsed.Seconds(), "barrier-awaits/s")
}

func barrierSuccesses() float64 {
	counter, ok := expvar.Get("agent_delivery_durability_barrier_total").(*expvar.Map)
	if !ok || counter == nil {
		return 0
	}
	value, ok := counter.Get("success").(*expvar.Int)
	if !ok || value == nil {
		return 0
	}
	return float64(value.Value())
}

func setContentionSynchronous(b *testing.B, repo *Repository, synchronous string) {
	b.Helper()
	if _, err := repo.db.Exec("PRAGMA synchronous=" + synchronous); err != nil {
		b.Fatal(err)
	}
	var level int
	if err := repo.db.Get(&level, "PRAGMA synchronous"); err != nil {
		b.Fatal(err)
	}
	want := map[string]int{"NORMAL": 1, "FULL": 2}[synchronous]
	if level != want {
		b.Fatalf("synchronous=%d want=%d", level, want)
	}
}

type contentionOwner struct {
	taskID, sessionID, incarnationID, executionID, submissionID, streamID string
}

func prepareContentionSession(b *testing.B, repo *Repository, workspaceID string, index int) contentionOwner {
	b.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("contention-%d", index)
	owner := contentionOwner{
		taskID: "task-" + suffix, sessionID: "session-" + suffix, incarnationID: "incarnation-" + suffix,
		executionID: "execution-" + suffix, submissionID: "prompt:" + suffix, streamID: "stream-" + suffix,
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: owner.taskID, WorkspaceID: workspaceID, Title: "Contention stream"}); err != nil {
		b.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: owner.sessionID, TaskID: owner.taskID, QueueIncarnationID: owner.incarnationID, State: models.TaskSessionStateCreated}); err != nil {
		b.Fatal(err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "running-" + suffix, SessionID: owner.sessionID, TaskID: owner.taskID, ExecutorID: "executor-" + suffix, AgentExecutionID: owner.executionID, Status: "running"}); err != nil {
		b.Fatal(err)
	}
	now := time.Now().UTC()
	if stored, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: owner.sessionID, IncarnationID: owner.incarnationID, Generation: 1, NativeSessionID: "native-" + suffix, CreationReason: "initial", CreatedAt: now, CommittedAt: now}, 0); err != nil || !stored {
		b.Fatalf("harness generation: %v %v", stored, err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: owner.submissionID, SessionID: owner.sessionID, IncarnationID: owner.incarnationID, HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash-" + suffix, Payload: []byte("prompt"), State: models.DeliverySubmissionDispatching}); err != nil {
		b.Fatal(err)
	}
	return owner
}

// streamContentionEvents follows the production commit pattern selected by the
// variant. Baseline and current receive every event in its own writer
// transaction and project in small contiguous batches. The combined variants
// admit and project each batch in one transaction. The final variant also
// awaits the durability barrier on the ACK cadence.
func streamContentionEvents(
	ctx context.Context,
	repo *Repository,
	owner contentionOwner,
	variant contentionVariant,
	projected, offered *atomic.Int64,
) error {
	payload, err := json.Marshal(streams.AgentEvent{
		Type: streams.EventTypeMessageChunk, Text: strings.Repeat("x", 200),
	})
	if err != nil {
		return err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	events := make(chan *models.AgentDeliveryEvent, contentionQueueSize)
	producerDone := make(chan struct{})
	go func() {
		defer func() {
			close(events)
			close(producerDone)
		}()
		ticker := time.NewTicker(contentionEventInterval)
		defer ticker.Stop()
		for sequence := int64(1); ; sequence++ {
			select {
			case <-streamCtx.Done():
				return
			case <-ticker.C:
			}
			// The offered rate stays fixed at 100 events/s even when the writer
			// cannot keep up; the queue retains the backlog for the measurement.
			offered.Add(1)
			event := contentionEvent(owner, sequence, payload)
			select {
			case events <- event:
			case <-streamCtx.Done():
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-producerDone
	}()

	var batch []*models.AgentDeliveryEvent
	for {
		event, ok := <-events
		if !ok {
			return nil
		}
		if streamCtx.Err() != nil {
			return nil
		}
		if !variant.combined {
			if _, err := repo.ReceiveAgentDeliveryEvent(streamCtx, event, event.Sequence); err != nil {
				return ignoreContentionShutdown(streamCtx, err)
			}
		}
		batch = append(batch, event)
		if len(batch) < contentionBatchSize {
			continue
		}
		if variant.combined {
			if _, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(streamCtx, batch, contentionEffects(batch), event.Sequence); err != nil {
				return ignoreContentionShutdown(streamCtx, err)
			}
		} else if _, err := repo.ProjectCanonicalAgentDeliveryEvents(streamCtx, batch, contentionEffects(batch)); err != nil {
			return ignoreContentionShutdown(streamCtx, err)
		}
		projected.Add(int64(len(batch)))
		batch = nil
	}
}

// simulateContentionAckWorker awaits the durability barrier on the cumulative
// ACK cadence without coupling stream output to ACK latency.
func simulateContentionAckWorker(ctx context.Context, repo *Repository) error {
	ticker := time.NewTicker(durableDeliveryAckCadence)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := repo.AwaitAgentDeliveryDurability(ctx); err != nil {
				return ignoreContentionShutdown(ctx, err)
			}
		}
	}
}

func ignoreContentionShutdown(ctx context.Context, err error) error {
	if ctx.Err() != nil && errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func contentionEvent(owner contentionOwner, sequence int64, payload []byte) *models.AgentDeliveryEvent {
	return &models.AgentDeliveryEvent{
		SessionID: owner.sessionID, IncarnationID: owner.incarnationID, HarnessGeneration: 1,
		StreamID: owner.streamID, Sequence: sequence, EventType: streams.EventTypeMessageChunk,
		Payload: payload, ReceivedAt: time.Now().UTC(), TaskID: owner.taskID, ExecutionID: owner.executionID,
		OwnerGeneration: 1, ExpectedStreamID: owner.streamID, SubmissionID: owner.submissionID,
	}
}

func contentionEffects(batch []*models.AgentDeliveryEvent) []*models.AgentDeliveryEffect {
	effects := make([]*models.AgentDeliveryEffect, len(batch))
	for index, event := range batch {
		now := event.ReceivedAt
		effects[index] = &models.AgentDeliveryEffect{
			EffectKey: fmt.Sprintf("agent_delivery.event:%s:%d", event.StreamID, event.Sequence),
			StreamID:  event.StreamID, Sequence: event.Sequence, EffectType: "agent_delivery.event",
			State: models.DeliveryEffectCompleted, CreatedAt: now, CompletedAt: &now,
		}
	}
	return effects
}

func measureContentionForeground(b *testing.B, repo *Repository, workspaceID string) []time.Duration {
	b.Helper()
	ctx := context.Background()
	samples := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for run := 0; run < b.N; run++ {
		task := &models.Task{ID: fmt.Sprintf("foreground-%d", run), WorkspaceID: workspaceID, Title: "Foreground write"}
		started := time.Now()
		if err := repo.CreateTask(ctx, task); err != nil {
			b.Fatal(err)
		}
		samples = append(samples, time.Since(started))
		time.Sleep(contentionForegroundGap)
	}
	b.StopTimer()
	return samples
}

func reportContentionSamples(b *testing.B, samples []time.Duration) {
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	percentile := func(p int) float64 {
		return float64(samples[(len(samples)-1)*p/100]) / float64(time.Millisecond)
	}
	b.ReportMetric(percentile(50), "p50-ms")
	b.ReportMetric(percentile(95), "p95-ms")
	b.ReportMetric(percentile(99), "p99-ms")
}
