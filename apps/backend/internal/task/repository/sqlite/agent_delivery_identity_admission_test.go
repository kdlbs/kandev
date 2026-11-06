package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestAgentDeliveryIdentityAdmissionUsesIndexes(t *testing.T) {
	repo := newRepoForSessionTests(t)
	query, args := agentDeliveryIdentityRetainedQuery("plan-session")
	rows, err := repo.db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(detail, "SCAN ") && detail != "SCAN CONSTANT ROW" {
			t.Fatalf("identity admission runs inside the writer transaction and must not scan delivery history: %s", detail)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentDeliveryIdentityAdmissionRejectsRetainedReferences(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			now := time.Date(2026, 11, 5, 10, 25, 15, 0, time.UTC)
			seed := []struct {
				name    string
				session string
				query   string
				args    []any
			}{
				{"cleanup job", "s-cleanup", `INSERT INTO agent_delivery_cleanup_jobs
					(session_id, incarnation_id, retired_at, phase, created_at, updated_at) VALUES (?, 'i', ?, 'pending', ?, ?)`,
					[]any{"s-cleanup", now, now, now}},
				{"submission", "s-submission", `INSERT INTO agent_delivery_submissions
					(id, session_id, incarnation_id, harness_generation, owner_generation, payload_hash, payload, state, created_at, updated_at)
					VALUES ('sub-1', ?, 'i', 1, 1, 'h', ?, 'accepted', ?, ?)`,
					[]any{"s-submission", []byte("{}"), now, now}},
				{"inbox", "s-inbox", `INSERT INTO agent_delivery_inbox
					(stream_id, sequence, session_id, incarnation_id, harness_generation, event_type, payload, received_at)
					VALUES ('stream-inbox', 1, ?, 'i', 1, 'message_chunk', ?, ?)`,
					[]any{"s-inbox", []byte("{}"), now}},
				{"cursor", "s-cursor", `INSERT INTO agent_delivery_cursors
					(stream_id, session_id, incarnation_id, harness_generation, updated_at) VALUES ('stream-cursor', ?, 'i', 1, ?)`,
					[]any{"s-cursor", now}},
				{"owned effect", "s-effect", `INSERT INTO agent_delivery_effects
					(effect_key, stream_id, sequence, effect_type, state, created_at, session_id)
					VALUES ('effect-owned', 'stream-without-cursor', 1, 'workflow.on_turn_complete', 'pending', ?, ?)`,
					[]any{now, "s-effect"}},
				{"stream-bound effect", "s-stream", `INSERT INTO agent_delivery_cursors
					(stream_id, session_id, incarnation_id, harness_generation, updated_at) VALUES ('stream-bound', ?, 'i', 1, ?)`,
					[]any{"s-stream", now}},
			}
			for _, row := range seed {
				if _, err := repo.db.Exec(repo.db.Rebind(row.query), row.args...); err != nil {
					t.Fatalf("seed %s: %v", row.name, err)
				}
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_effects
				(effect_key, stream_id, sequence, effect_type, state, created_at)
				VALUES ('effect-stream', 'stream-bound', 1, 'workflow.on_turn_complete', 'pending', ?)`), now); err != nil {
				t.Fatalf("seed stream-bound effect: %v", err)
			}

			ctx := context.Background()
			for _, row := range seed {
				err := repo.ensureAgentDeliveryIdentityAvailable(ctx, repo.db, row.session)
				if !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityRetiring) {
					t.Fatalf("%s: retained identity %q must be rejected, got %v", row.name, row.session, err)
				}
			}
			if err := repo.ensureAgentDeliveryIdentityAvailable(ctx, repo.db, "s-unrelated"); err != nil {
				t.Fatalf("unrelated identity must stay admissible: %v", err)
			}
		})
	}
}
