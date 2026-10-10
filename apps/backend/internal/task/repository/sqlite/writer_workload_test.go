package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type writerWorkload struct {
	repo     *Repository
	health   *requiredstores.Health
	sessions int
	bytes    int
	batch    int
	growing  bool
}

func newWriterWorkload(t testing.TB, sessions, payloadBytes, batch int) *writerWorkload {
	t.Helper()
	r := newSidebarReaderPool(t)
	seedWorkspace(t, r, "writer-workload")
	for i := 0; i < sessions; i++ {
		id := fmt.Sprintf("w%d", i)
		require.NoError(t, r.CreateTask(context.Background(), &models.Task{
			ID: id, WorkspaceID: "writer-workload", Title: id,
		}))
		require.NoError(t, r.CreateTaskSession(context.Background(), &models.TaskSession{ID: id, TaskID: id}))
		now := time.Now().UTC()
		_, err := r.db.Exec(`INSERT INTO task_session_turns
			(id, task_session_id, task_id, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			id, id, id, now, now, now)
		require.NoError(t, err)
	}
	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{{
		ID: "workload", OwnerPackage: "workload", Sweep: startup.StepStoresRepositories,
		RequiredTables: []string{"agent_delivery_inbox", "agent_delivery_cursors", "task_session_messages"},
	}})
	require.NoError(t, err)
	require.NoError(t, tracker.RecordSuccess("workload"))
	health := requiredstores.NewHealth(tracker, db.NewPool(r.db, r.ro), nil)
	require.NoError(t, health.Check(context.Background()))
	return &writerWorkload{repo: r, health: health, sessions: sessions, bytes: payloadBytes, batch: batch}
}

func (w *writerWorkload) events(session, round int) ([]*models.AgentDeliveryEvent, []*models.AgentDeliveryEffect) {
	id := fmt.Sprintf("w%d", session)
	stream := fmt.Sprintf("%s-r%d", id, round)
	if w.growing {
		stream = id + "-growing"
	}
	events := make([]*models.AgentDeliveryEvent, w.batch)
	effects := make([]*models.AgentDeliveryEffect, w.batch)
	payload, _ := json.Marshal(streams.AgentEvent{
		Type: streams.EventTypeMessageChunk, Text: strings.Repeat("x", w.bytes),
		TurnID: id, CanonicalMessageID: stream,
	})
	for i := range events {
		seq := int64(i + 1)
		if w.growing {
			seq += int64(round * w.batch)
		}
		events[i] = &models.AgentDeliveryEvent{SessionID: id, IncarnationID: id, HarnessGeneration: 1,
			StreamID: stream, Sequence: seq, EventType: streams.EventTypeMessageChunk, Payload: payload}
		effects[i] = &models.AgentDeliveryEffect{EffectKey: fmt.Sprintf("%s:%d", stream, seq),
			StreamID: stream, Sequence: seq, EffectType: "agent_delivery.event"}
	}
	return events, effects
}

func (w *writerWorkload) receive(ctx context.Context, events []*models.AgentDeliveryEvent) error {
	for _, event := range events {
		if err := writerWorkloadMeasure(ctx, "receive", func() error {
			_, err := w.repo.ReceiveAgentDeliveryEvent(ctx, event, events[len(events)-1].Sequence)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *writerWorkload) project(ctx context.Context, events []*models.AgentDeliveryEvent, effects []*models.AgentDeliveryEffect) error {
	return writerWorkloadMeasure(ctx, "project", func() error {
		_, err := w.repo.ProjectCanonicalAgentDeliveryEvents(ctx, events, effects)
		return err
	})
}

func (w *writerWorkload) prepareTerminal(ctx context.Context, session, round int) (*models.AgentDeliveryEvent, error) {
	id := fmt.Sprintf("w%d", session)
	stream := fmt.Sprintf("%s-terminal-%d", id, round)
	_, err := w.repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: stream, SessionID: id, IncarnationID: id, HarnessGeneration: 1,
		PayloadHash: "synthetic", Payload: []byte("synthetic"), State: models.DeliverySubmissionDispatching,
	})
	if err != nil {
		return nil, err
	}
	event := &models.AgentDeliveryEvent{SessionID: id, IncarnationID: id, HarnessGeneration: 1,
		StreamID: stream, SubmissionID: stream, Sequence: 1, EventType: "complete", Terminal: true,
		Payload: []byte(`{"turn_id":"` + id + `"}`)}
	if _, err = w.repo.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil {
		return nil, err
	}
	_, err = w.repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	return event, err
}

func (w *writerWorkload) settle(ctx context.Context, event *models.AgentDeliveryEvent) error {
	return writerWorkloadMeasure(ctx, "settle", func() error {
		settled, err := w.repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, 1, models.DeliverySubmissionCompleted, time.Now().UTC())
		if err == nil && !settled {
			return fmt.Errorf("terminal did not settle")
		}
		return err
	})
}

func (w *writerWorkload) sidebar(ctx context.Context) error {
	return writerWorkloadMeasure(ctx, "sidebar", func() error {
		page, err := w.repo.QuerySidebarTaskPage(ctx, "writer-workload", sidebarTaskQuery(1), models.SidebarTaskViewPreferences{})
		if err == nil && len(page.Tasks) != w.sessions {
			return fmt.Errorf("sidebar lost tasks: %d", len(page.Tasks))
		}
		return err
	})
}

// This fixture protects the combination of separate WAL readers, durable output,
// and terminal settlement; older delivery fixtures alias reader and writer.
func TestWriterWorkloadAttribution(t *testing.T) {
	w := newWriterWorkload(t, 4, 128, 4)
	o := newWriterWorkloadObservation()
	ctx := context.WithValue(context.Background(), writerWorkloadObservationKey{}, o)
	for session := 0; session < w.sessions; session++ {
		events, effects := w.events(session, 0)
		require.NoError(t, w.receive(ctx, events))
		require.NoError(t, w.project(ctx, events, effects))
		require.NoError(t, w.receive(ctx, events))
		require.NoError(t, w.project(ctx, events, effects))
		message, err := w.repo.GetMessage(ctx, events[0].StreamID)
		require.NoError(t, err)
		require.Equal(t, strings.Repeat("x", w.bytes*w.batch), message.Content)
		cursor, err := w.repo.GetAgentDeliveryCursor(ctx, events[0].StreamID)
		require.NoError(t, err)
		require.Equal(t, int64(w.batch), cursor.ProjectedSequence)
		event, err := w.prepareTerminal(ctx, session, 0)
		require.NoError(t, err)
		require.NoError(t, w.settle(ctx, event))
		submission, err := w.repo.GetAgentDeliverySubmission(ctx, event.SubmissionID)
		require.NoError(t, err)
		require.Equal(t, models.DeliverySubmissionCompleted, submission.State)
	}
	// A real admitted writer remains held while an independent sidebar snapshot completes.
	tx, err := w.repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	require.NoError(t, w.sidebar(readCtx))
	cancelled, cancelWrite := context.WithCancel(ctx)
	cancelWrite()
	events, _ := w.events(0, 99)
	_, err = w.repo.ReceiveAgentDeliveryEvent(cancelled, events[0], 1)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, tx.Rollback())
	require.NoError(t, w.health.Check(readCtx))
	missing, err := w.repo.GetAgentDeliveryCursor(ctx, events[0].StreamID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Nil(t, missing)
	if writerWorkloadInstrumented {
		require.NotEmpty(t, o.values["receive.span.commit.ok"])
		require.NotEmpty(t, o.values["project.span.commit.ok"])
		require.NotEmpty(t, o.values["settle.span.commit.ok"])
		require.Empty(t, o.active)
	}
	o.report(t, map[string]any{"fixture": "correctness"})
}
