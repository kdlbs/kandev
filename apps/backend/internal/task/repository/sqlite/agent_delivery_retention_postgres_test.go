package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestDeliveryRetentionPostgresMVCCAndMonotonicCheckpoint(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	writerDB := testutil.OpenIsolatedPostgres(t, dsn)
	var schema string
	if err := writerDB.QueryRowx(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("read isolated schema: %v", err)
	}
	readerRaw, err := internaldb.OpenPostgres(dsn, 1, 1)
	if err != nil {
		t.Fatalf("open independent PostgreSQL pool: %v", err)
	}
	readerDB := sqlx.NewDb(readerRaw, "pgx")
	readerDB.SetMaxOpenConns(1)
	readerDB.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = readerDB.Close() })
	if _, err := readerDB.Exec(`SET search_path TO "` + schema + `"`); err != nil {
		t.Fatalf("set independent connection schema: %v", err)
	}
	writerRepo, err := NewWithDB(writerDB, writerDB, nil)
	if err != nil {
		t.Fatalf("initialize writer repository: %v", err)
	}
	readerRepo, err := NewWithDB(readerDB, readerDB, nil)
	if err != nil {
		t.Fatalf("initialize reader repository: %v", err)
	}

	event := seedRetentionCrashEventInRepo(t, writerRepo)
	reachedCursor := make(chan struct{})
	releaseCommit := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCommit) }) }
	defer release()
	writerRepo.agentDeliveryRetentionFailpoint = func(point string) error {
		if point == "after_retired_cursor" {
			close(reachedCursor)
			<-releaseCommit
		}
		return nil
	}
	writerDone := make(chan error, 1)
	go func() {
		_, err := writerRepo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
		writerDone <- err
	}()
	select {
	case <-reachedCursor:
	case <-time.After(2 * time.Second):
		t.Fatal("retention transaction did not reach the retired cursor boundary")
	}
	var visiblePayload []byte
	var visibleCursor int64
	var visibleEffects int
	if err := readerDB.QueryRowx(readerDB.Rebind(`
		SELECT payload FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`),
		event.StreamID, event.Sequence).Scan(&visiblePayload); err != nil {
		t.Fatalf("read uncommitted inbox deletion: %v", err)
	}
	if err := readerDB.QueryRowx(readerDB.Rebind(`
		SELECT retired_sequence FROM agent_delivery_cursors WHERE stream_id = ?`), event.StreamID).Scan(&visibleCursor); err != nil {
		t.Fatalf("read uncommitted cursor update: %v", err)
	}
	if err := readerDB.QueryRowx(readerDB.Rebind(`
		SELECT COUNT(*) FROM agent_delivery_effects WHERE effect_key = ?`),
		fmt.Sprintf("agent_delivery.event:%s:1", event.StreamID)).Scan(&visibleEffects); err != nil {
		t.Fatalf("read uncommitted receipt deletion: %v", err)
	}
	if len(visiblePayload) == 0 || visibleCursor != 0 || visibleEffects != 1 {
		t.Fatalf("reader saw partial retention commit: payload=%d bytes, cursor=%d, effects=%d", len(visiblePayload), visibleCursor, visibleEffects)
	}
	release()
	if err := <-writerDone; err != nil {
		t.Fatalf("commit retention transaction: %v", err)
	}
	writerRepo.agentDeliveryRetentionFailpoint = nil
	var committedInbox int
	if err := readerDB.QueryRowx(readerDB.Rebind(`
		SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`),
		event.StreamID, event.Sequence).Scan(&committedInbox); err != nil {
		t.Fatalf("read committed inbox deletion: %v", err)
	}
	if err := readerDB.QueryRowx(readerDB.Rebind(`
		SELECT retired_sequence FROM agent_delivery_cursors WHERE stream_id = ?`), event.StreamID).Scan(&visibleCursor); err != nil {
		t.Fatalf("read committed cursor update: %v", err)
	}
	if err := readerDB.QueryRowx(readerDB.Rebind(`
		SELECT COUNT(*) FROM agent_delivery_effects WHERE effect_key = ?`),
		fmt.Sprintf("agent_delivery.event:%s:1", event.StreamID)).Scan(&visibleEffects); err != nil {
		t.Fatalf("read committed receipt deletion: %v", err)
	}
	if committedInbox != 0 || visibleCursor != 1 || visibleEffects != 0 {
		t.Fatalf("reader saw invalid committed state: inbox=%d cursor=%d effects=%d", committedInbox, visibleCursor, visibleEffects)
	}

	for sequence := int64(2); sequence <= 3; sequence++ {
		payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: fmt.Sprintf("concurrent output %d", sequence)})
		if err != nil {
			t.Fatalf("marshal event %d: %v", sequence, err)
		}
		next := &models.AgentDeliveryEvent{
			SessionID: event.SessionID, IncarnationID: event.IncarnationID,
			HarnessGeneration: event.HarnessGeneration, StreamID: event.StreamID,
			Sequence: sequence, EventType: streams.EventTypeMessageChunk,
			Payload: payload, ReceivedAt: event.ReceivedAt.Add(time.Duration(sequence) * time.Second),
		}
		if inserted, err := writerRepo.ReceiveAgentDeliveryEvent(context.Background(), next, sequence); err != nil || !inserted {
			t.Fatalf("receive concurrent event %d = %v, err %v", sequence, inserted, err)
		}
		if _, err := writerRepo.ProjectCanonicalAgentDeliveryEvent(context.Background(), next, completedRetentionEventEffect(next)); err != nil {
			t.Fatalf("project concurrent event %d: %v", sequence, err)
		}
		if _, err := writerDB.ExecContext(context.Background(), writerDB.Rebind(`
			UPDATE agent_delivery_inbox SET projected_at = ? WHERE stream_id = ? AND sequence = ?`),
			event.ReceivedAt, event.StreamID, sequence); err != nil {
			t.Fatalf("age concurrent event %d: %v", sequence, err)
		}
	}

	var workers sync.WaitGroup
	results := make(chan error, 2)
	for _, repo := range []*Repository{writerRepo, readerRepo} {
		workers.Add(1)
		go func(repo *Repository) {
			defer workers.Done()
			result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 1})
			if err == nil && result.EventRowsRetired != 1 {
				err = fmt.Errorf("concurrent batch retired %d rows, want 1", result.EventRowsRetired)
			}
			results <- err
		}(repo)
	}
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent retention batch: %v", err)
		}
	}
	cursor, err := readerRepo.GetAgentDeliveryCursor(context.Background(), event.StreamID)
	if err != nil || cursor.RetiredSequence != 3 {
		t.Fatalf("monotonic retired checkpoint = %+v, err %v, want 3", cursor, err)
	}
}
