package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/settings"
	"github.com/kandev/kandev/internal/task/deliveryretention"
	"github.com/kandev/kandev/internal/task/models"
)

func seedRetentionStorageRows(t testing.TB, repo *Repository, event *models.AgentDeliveryEvent, count, payloadSize int) {
	t.Helper()
	payload := []byte(`{"type":"message_chunk","text":"` + strings.Repeat("x", payloadSize-34) + `"}`)
	if len(payload) != payloadSize {
		t.Fatalf("fixture payload bytes=%d want=%d", len(payload), payloadSize)
	}
	for first := 2; first <= count; first += 10000 {
		last := first + 9999
		if last > count {
			last = count
		}
		seedRetentionStorageRange(t, repo, event, first, last, payload)
	}
	advanceRetentionStorageHighWater(t, repo, event.StreamID, count)
}

func seedRetentionStorageRange(t testing.TB, repo *Repository, event *models.AgentDeliveryEvent, first, last int, payload []byte) {
	t.Helper()
	tx, err := repo.db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	old := event.ReceivedAt
	_, err = tx.Exec(repo.db.Rebind(`WITH RECURSIVE sequences(n) AS (
   SELECT CAST(? AS BIGINT) UNION ALL SELECT n+1 FROM sequences WHERE n < ?
  ) INSERT INTO agent_delivery_inbox
  (stream_id, sequence, session_id, incarnation_id, harness_generation, submission_id, event_type, payload, terminal, received_at, projected_at)
  SELECT ?, n, ?, ?, 1, ?, 'message_chunk', ?, 0, ?, ? FROM sequences`), first, last, event.StreamID, event.SessionID, event.IncarnationID, event.SubmissionID, payload, old, old)
	if err == nil {
		_, err = tx.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_effects
   (effect_key, stream_id, sequence, effect_type, state, created_at, completed_at)
   SELECT 'agent_delivery.event:' || stream_id || ':' || CAST(sequence AS TEXT), stream_id, sequence,
    'agent_delivery.event', 'completed', received_at, projected_at
   FROM agent_delivery_inbox WHERE stream_id = ? AND sequence BETWEEN ? AND ?`), event.StreamID, first, last)
	}
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func advanceRetentionStorageHighWater(t testing.TB, repo *Repository, streamID string, count int) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_cursors SET received_sequence = ?, projected_sequence = ?, remote_high_water = ? WHERE stream_id = ?`), count, count, count, streamID); err != nil {
		t.Fatal(err)
	}
}

func TestDeliveryRetentionStorageQueryPlan(t *testing.T) {
	repo, events := seedRetentionBoundsStream(t, "SQLite", "plan", 1, 0)
	rows, err := repo.db.Query(`EXPLAIN QUERY PLAN SELECT MIN(sequence) FROM agent_delivery_inbox WHERE stream_id = ? AND terminal <> 0 AND sequence > ?`, events[0].StreamID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var details string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details += detail
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(details, "idx_agent_delivery_inbox_terminal_sequence") {
		t.Fatalf("terminal frontier needs a sparse covering index: %s", details)
	}
}

func assertRetentionStorageToolPolicy(t testing.TB, repo *Repository) {
	t.Helper()
	pool := db.NewPool(repo.db, repo.db)
	store, err := settings.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	// An absent policy uses the disabled default; this fixture starts no canonical cleanup service.
	if _, exists, err := store.Get(context.Background(), "tool_payload_retention"); err != nil || exists {
		t.Fatalf("canonical tool retention must use its disabled default: override=%t err=%v", exists, err)
	}
}

func TestDeliveryRetentionStorageTerminalPlan(t *testing.T) {
	repo, _ := seedRetentionBoundsStream(t, "SQLite", "bounded-terminal-plan", 1, 0)
	query, args := agentDeliveryRetentionCandidateQuery("", 0, nil, 200, 4<<20, repo.nowUTC().Add(-time.Hour), true)
	rows, err := repo.db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	checked := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "SEARCH terminal") {
			checked = true
			if !strings.Contains(detail, "idx_agent_delivery_inbox_terminal_sequence") || !strings.Contains(detail, "sequence<?") {
				t.Fatalf("terminal proof lookup must stop at the candidate rather than scan its future: %s", detail)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("terminal proof query plan was not observed")
	}
}

type retentionStorageRepository interface {
	RunAgentDeliveryRetentionBatch(context.Context, models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error)
	GetAgentDeliveryCursor(context.Context, string) (*models.AgentDeliveryCursor, error)
	awaitRetentionStorageSettlement(context.Context) error
}

func (r *Repository) awaitRetentionStorageSettlement(ctx context.Context) error {
	if r.db.DriverName() != "pgx" {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// A client deadline can precede the server's COMMIT. Acquire the same
	// lease before measuring so a previous pass cannot enter the next sample.
	if _, err := tx.ExecContext(ctx, `UPDATE agent_delivery_retention_state SET updated_at=updated_at WHERE id=1`); err != nil {
		return err
	}
	return tx.Rollback()
}

func settledRetentionStorageCursor(repo retentionStorageRepository, streamID string) (*models.AgentDeliveryCursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := repo.awaitRetentionStorageSettlement(ctx); err != nil {
		return nil, err
	}
	return repo.GetAgentDeliveryCursor(ctx, streamID)
}

func runRetentionStorageBatch(t *testing.T, repo retentionStorageRepository, batchRows int) (models.AgentDeliveryRetentionBatchResult, int) {
	t.Helper()
	return runRetentionStorageBatchWithCursor(t, repo, batchRows, "", 0)
}

func runRetentionStorageBatchWithCursor(t *testing.T, repo retentionStorageRepository, batchRows int, streamID string, retiredBefore int64) (models.AgentDeliveryRetentionBatchResult, int) {
	t.Helper()
	var lastErr error
	for retries := 0; retries < 3; retries++ {
		result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{MaxCandidates: batchRows})
		if err == nil {
			return result, retries
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		lastErr = err
		// A deadline may hide a committed transaction's acknowledgement.
		if streamID != "" {
			cursor, readErr := settledRetentionStorageCursor(repo, streamID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if cursor.RetiredSequence > retiredBefore {
				return result, retries + 1
			}
		}
		// Failed worker passes use exponential backoff before retrying.
		if retries < 2 {
			timer := time.NewTimer(time.Second << retries)
			<-timer.C
		}
	}
	t.Fatalf("retention did not make progress after three bounded deadline retries: %v", lastErr)
	return models.AgentDeliveryRetentionBatchResult{}, 0
}

func runRetentionStorageAccountedBatch(t *testing.T, repo retentionStorageRepository, event *models.AgentDeliveryEvent, payloadSize, batchRows int) (models.AgentDeliveryRetentionBatchResult, int, int64) {
	t.Helper()
	before, err := repo.GetAgentDeliveryCursor(context.Background(), event.StreamID)
	if err != nil {
		t.Fatal(err)
	}
	result, retries := runRetentionStorageBatchWithCursor(t, repo, batchRows, event.StreamID, before.RetiredSequence)
	after, err := settledRetentionStorageCursor(repo, event.StreamID)
	if err != nil {
		t.Fatal(err)
	}
	retired := after.RetiredSequence - before.RetiredSequence
	if retired < 0 || retired > int64((retries+1)*batchRows) {
		t.Fatalf("retirement exceeded bounded attempts: before=%d after=%d retries=%d", before.RetiredSequence, after.RetiredSequence, retries)
	}
	// The seeded stream has one original envelope followed by equal-size raw
	// payloads. Its committed prefix measures removal even if COMMIT timed out.
	removedBytes := retired * int64(payloadSize)
	if before.RetiredSequence == 0 && retired > 0 {
		removedBytes += int64(len(event.Payload) - payloadSize)
	}
	if result.PayloadBytesPruned > removedBytes || (retries == 0 && result.PayloadBytesPruned != removedBytes) {
		t.Fatalf("acknowledged bytes=%d durable removed bytes=%d retries=%d", result.PayloadBytesPruned, removedBytes, retries)
	}
	if result.PayloadBytesPruned != removedBytes {
		t.Logf("commit acknowledgement lost: durable_removed_bytes=%d acknowledged_bytes=%d deadline_retries=%d", removedBytes, result.PayloadBytesPruned, retries)
	}
	return result, retries, removedBytes
}

func TestDeliveryRetentionStorageProtectedGap(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			testRetentionStorageProtectedGap(t, engine, 200, 100)
		})
	}
}

func testRetentionStorageProtectedGap(t *testing.T, engine string, count, gap int) {
	t.Helper()
	repo, events := seedRetentionBoundsStream(t, engine, "storage-gap", 1, 0)
	assertRetentionStorageToolPolicy(t, repo)
	const batchRows = deliveryretention.BackgroundBatchRows
	seedRetentionStorageRows(t, repo, events[0], count, 1024)
	before := retentionCanonicalContent(t, repo, events[0].SessionID)
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET terminal=1,event_type='complete',payload=?,content_hash='',hash_version=0,payload_bytes=0 WHERE stream_id=? AND sequence=?`), []byte(`{"type":"complete"}`), events[0].StreamID, gap); err != nil {
		t.Fatal(err)
	}
	for batch := 0; batch < 2*count/batchRows+10; batch++ {
		result, _ := runRetentionStorageBatch(t, repo, batchRows)
		if result.Candidates > batchRows || result.PayloadBytesRead > 4<<20 {
			t.Fatalf("batch exceeded work budget: %+v", result)
		}
		var compactedThrough int64
		if err := repo.db.QueryRow(`SELECT last_compaction_sequence FROM agent_delivery_retention_state WHERE id=1`).Scan(&compactedThrough); err != nil {
			t.Fatal(err)
		}
		if compactedThrough >= int64(count) {
			break
		}
	}
	if err := repo.awaitRetentionStorageSettlement(context.Background()); err != nil {
		t.Fatal(err)
	}
	cursor, err := repo.GetAgentDeliveryCursor(context.Background(), events[0].StreamID)
	if err != nil || cursor.RetiredSequence != int64(gap-1) {
		t.Fatalf("protected frontier: %+v %v", cursor, err)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=?", events[0].StreamID, count-gap+1)
	if got := retentionCanonicalContent(t, repo, events[0].SessionID); got != before {
		t.Fatal("protected-gap cleanup changed canonical output")
	}
	storage := measureRetentionStorage(t, repo)
	if storage.RawPayloadBytes != int64(len(`{"type":"complete"}`)) || storage.CompactRows != int64(count-gap) {
		t.Fatalf("protected terminal must retain its payload while later output becomes receipts: %+v", storage)
	}
	t.Logf("engine=%s seeded=%d protected_backlog=%d storage=%+v", engine, count, count-gap+1, storage)
}

func TestDeliveryRetentionStorageSeekPlan(t *testing.T) {
	repo, events := seedRetentionBoundsStream(t, "SQLite", "seek-plan", 1, 0)
	for _, locked := range []bool{false, true} {
		t.Run(fmt.Sprintf("locked_%t", locked), func(t *testing.T) {
			var ids []string
			if locked {
				ids = []string{events[0].StreamID}
			}
			query, args := agentDeliveryRetentionCandidateQuery(events[0].StreamID, 500_000, ids, 200, 4<<20, repo.nowUTC().Add(-time.Hour), locked)
			rows, err := repo.db.Query("EXPLAIN QUERY PLAN "+query, args...)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rows.Close() }()
			checked := false
			sequenceSeek := false
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(detail, "SCAN i USING") {
					t.Fatalf("progress must seek beyond retained historical rows: %s", detail)
				}
				if strings.Contains(detail, "SEARCH i USING") {
					checked = true
					sequenceSeek = sequenceSeek || strings.Contains(detail, "sequence>?")
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !checked {
				t.Fatal("inbox progress seek was not observed")
			}
			if locked && !sequenceSeek {
				t.Fatal("locked stream must seek its sequence frontier")
			}
		})
	}
}

func TestDeliveryRetentionStorageCompactionPlan(t *testing.T) {
	repo, events := seedRetentionBoundsStream(t, "SQLite", "compaction-plan", 1, 0)
	var compact []*agentDeliveryRetentionCandidate
	for sequence := int64(1); sequence <= 200; sequence++ {
		event := *events[0]
		event.Sequence = sequence
		compact = append(compact, &agentDeliveryRetentionCandidate{event: event})
	}
	values, args := agentDeliveryCompactionValues(compact)
	rows, err := repo.db.Query("EXPLAIN QUERY PLAN "+strings.Replace(agentDeliveryCompactHashesSQL, "/* values */", values, 1), args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	checked := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "SCAN inbox") {
			t.Fatalf("bounded compaction must probe inbox identities: %s", detail)
		}
		if strings.Contains(detail, "SEARCH inbox") {
			checked = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("compaction inbox lookup was not observed")
	}
}
