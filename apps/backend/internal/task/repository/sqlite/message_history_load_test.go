package sqlite

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	loadHistoryMessageCount = 1_000_000
	loadHistoryPayloadCount = 64
	loadHistoryPayloadBytes = 64 * 1024 * 1024
)

// TestListMessagesPaginatedBoundsMultiGigabyteEquivalentHistory proves the
// normal history path stays page-bounded with a million-row transcript and
// four GiB of logically externalized tool output. The payload bodies are
// deliberately tiny sentinels: payload integrity/rehydration has dedicated
// tests, while this fixture isolates the list path's cardinality and I/O
// contract without committing or generating a multi-gigabyte test artifact.
func TestListMessagesPaginatedBoundsMultiGigabyteEquivalentHistory(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-load-history"
		sessionID = "session-load-history"
		turnID    = "turn-load-history"
		pageSize  = 50
	)
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	seedLargeHistoryFixture(t, repo, taskID, sessionID, turnID)
	assertIndexedBoundedMessageQuery(t, repo, sessionID, pageSize)

	hydrationsBefore := totalPayloadHydrations(t)
	durations := make([]time.Duration, 0, 15)
	for i := 0; i < cap(durations); i++ {
		started := time.Now()
		messages, hasMore, err := repo.ListMessagesPaginated(ctx, sessionID, models.ListMessagesOptions{
			Limit: pageSize,
			Sort:  "desc",
		})
		durations = append(durations, time.Since(started))
		if err != nil {
			t.Fatalf("read page %d: %v", i, err)
		}
		if len(messages) != pageSize || !hasMore {
			t.Fatalf("read page %d = (%d rows, hasMore=%v), want (%d, true)", i, len(messages), hasMore, pageSize)
		}
		assertLightweightMessagePage(t, messages)
	}
	if delta := totalPayloadHydrations(t) - hydrationsBefore; delta != 0 {
		t.Fatalf("payload hydrations during list = %d, want 0", delta)
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	t.Logf("bounded-history load: rows=%d logical_payload_bytes=%d page=%d samples=%d p50=%s p95=%s max=%s",
		loadHistoryMessageCount,
		int64(loadHistoryPayloadCount)*loadHistoryPayloadBytes,
		pageSize,
		len(durations),
		durations[len(durations)/2],
		durations[(len(durations)*95-1)/100],
		durations[len(durations)-1],
	)
}

func seedLargeHistoryFixture(t *testing.T, repo *Repository, taskID, sessionID, turnID string) {
	t.Helper()
	tx, err := repo.db.Beginx()
	if err != nil {
		t.Fatalf("begin large-history seed: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.Exec(`
		WITH RECURSIVE payloads(n) AS (
			SELECT 1 UNION ALL SELECT n + 1 FROM payloads WHERE n < ?
		)
		INSERT INTO task_message_payloads
			(digest, compressed_content, uncompressed_size, compressed_size, created_at)
		SELECT printf('load-payload-%03d', n), x'1f8b08000000000000ff03000000000000000000', ?, 20, CURRENT_TIMESTAMP
		FROM payloads
	`, loadHistoryPayloadCount, loadHistoryPayloadBytes); err != nil {
		t.Fatalf("seed payloads: %v", err)
	}
	if _, err = tx.Exec(`
		WITH RECURSIVE messages(n) AS (
			SELECT 1 UNION ALL SELECT n + 1 FROM messages WHERE n < ?
		)
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content,
			 requests_input, type, metadata, created_at, updated_at, prompt_seq,
			 payload_digest, payload_size)
		SELECT printf('load-message-%07d', n), ?, ?, ?, 'agent', '', 'ok',
		       0, 'tool_call', '{}',
		       printf('2026-01-%02d %02d:%02d:%02d.000000+00:00',
		              ((n - 1) / 86400) % 28 + 1, ((n - 1) / 3600) % 24,
		              ((n - 1) / 60) % 60, (n - 1) % 60),
		       CURRENT_TIMESTAMP, 0,
		       CASE WHEN n <= ? THEN printf('load-payload-%03d', n) ELSE '' END,
		       CASE WHEN n <= ? THEN ? ELSE 0 END
		FROM messages
	`, loadHistoryMessageCount, sessionID, taskID, turnID,
		loadHistoryPayloadCount, loadHistoryPayloadCount, loadHistoryPayloadBytes); err != nil {
		t.Fatalf("seed messages: %v", err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatalf("commit large-history seed: %v", err)
	}
}

func assertIndexedBoundedMessageQuery(t *testing.T, repo *Repository, sessionID string, limit int) {
	t.Helper()
	query, args := buildListMessagesQuery(
		repo.ro.DriverName(), sessionID, models.ListMessagesOptions{Sort: "desc"}, nil, "", "DESC", limit,
	)
	if strings.Contains(query, "task_message_payloads") || !strings.Contains(query, "LIMIT ?") {
		t.Fatalf("history query is not payload-independent and bounded: %s", query)
	}
	rows, err := repo.ro.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("explain bounded history query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan query plan: %v", err)
		}
		details = append(details, detail)
	}
	joined := strings.Join(details, "\n")
	if !strings.Contains(joined, "idx_messages_prompt_order") {
		t.Fatalf("bounded history query did not use prompt-order index:\n%s", joined)
	}
	if !strings.Contains(query, dialect.NormalizedMicrosecond(repo.ro.DriverName(), "created_at")) {
		t.Fatal("history query ordering diverged from the indexed normalized timestamp expression")
	}
}

func assertLightweightMessagePage(t *testing.T, messages []*models.Message) {
	t.Helper()
	returnedBytes := 0
	for _, message := range messages {
		returnedBytes += len(message.Content)
		if message.PayloadDigest != "" || message.PayloadSize != 0 {
			t.Fatalf("list eagerly materialized payload reference: %+v", message)
		}
	}
	if returnedBytes > len(messages)*len("ok") {
		t.Fatalf("returned content bytes = %d, want <= %d", returnedBytes, len(messages)*len("ok"))
	}
}

func totalPayloadHydrations(t *testing.T) int64 {
	t.Helper()
	return metricMapValue(t, messagePayloadHydrationsTotal, "outcome=success") +
		metricMapValue(t, messagePayloadHydrationsTotal, "outcome=error")
}
