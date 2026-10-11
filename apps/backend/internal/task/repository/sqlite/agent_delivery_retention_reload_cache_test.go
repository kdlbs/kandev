package sqlite

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionStorageReloadStatementShape(t *testing.T) {
	selected := make([]agentDeliveryRetentionCandidate, 200)
	for n := range selected {
		selected[n].event.StreamID = "shape"
		selected[n].event.Sequence = int64(n + 1)
	}
	query, _ := agentDeliverySelectedCandidateQuery(selected, 4<<20, time.Now())
	for _, count := range []int{1, 17, 200} {
		got, args := agentDeliverySelectedCandidateQuery(selected[:count], 1024, time.Now())
		if got != query || len(args) != 409 {
			t.Fatalf("locked reload must have one reusable bounded statement for %d selected identities: bytes=%d args=%d", count, len(got), len(args))
		}
		for slot := count * 2; slot < 400; slot++ {
			if args[slot] != nil {
				t.Fatalf("unused identity slots must be NULL: slot=%d value=%v", slot, args[slot])
			}
		}
	}
}

func TestDeliveryRetentionStorageReloadExactSelection(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, count := range []int{1, 17, 200} {
			t.Run(fmt.Sprintf("%s/%d", engine, count), func(t *testing.T) {
				repo, events := seedRetentionBoundsStream(t, engine, "reload-exact", 1, 0)
				seedRetentionStorageRows(t, repo, events[0], 201, 1024)
				for _, query := range []string{
					`INSERT INTO agent_delivery_cursors (stream_id, session_id, incarnation_id, harness_generation, received_sequence, projected_sequence, retired_sequence, updated_at)
					 SELECT 'reload-second-stream', session_id, incarnation_id, harness_generation, received_sequence, projected_sequence, retired_sequence, updated_at FROM agent_delivery_cursors`,
					`UPDATE agent_delivery_inbox SET stream_id='reload-second-stream' WHERE sequence % 2 = 0`,
					`UPDATE agent_delivery_effects SET stream_id='reload-second-stream' WHERE sequence % 2 = 0`,
				} {
					if _, err := repo.db.Exec(query); err != nil {
						t.Fatal(err)
					}
				}
				selected := make([]agentDeliveryRetentionCandidate, count)
				for n := range selected {
					selected[n].event.StreamID = events[0].StreamID
					selected[n].event.Sequence = int64(n + 2)
					if selected[n].event.Sequence%2 == 0 {
						selected[n].event.StreamID = "reload-second-stream"
					}
					selected[n].compactionSource = n%2 == 0
				}
				sort.Slice(selected, func(a, b int) bool {
					if selected[a].event.StreamID != selected[b].event.StreamID {
						return selected[a].event.StreamID < selected[b].event.StreamID
					}
					return selected[a].event.Sequence < selected[b].event.Sequence
				})
				if err := repo.prepareAgentDeliveryReloadStatement(t.Context()); err != nil {
					t.Fatal(err)
				}
				tx, err := repo.db.BeginTxx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback() }()
				rows, err := repo.reloadAgentDeliverySelectedCandidates(t.Context(), tx, selected, 2048, repo.nowUTC().Add(-time.Hour))
				if err != nil || len(rows) != count {
					t.Fatalf("exact reload selection rows=%d want=%d error=%v", len(rows), count, err)
				}
				for n, row := range rows {
					if row.event.Sequence != selected[n].event.Sequence || row.compactionSource != selected[n].compactionSource || row.payloadLoaded != (n < 2) {
						t.Fatalf("identity, source or byte budget changed at %d: %+v", n, row)
					}
				}
				if _, err := tx.ExecContext(t.Context(), repo.db.Rebind(`UPDATE agent_delivery_effects SET state='pending',completed_at=NULL WHERE stream_id=? AND sequence=?`), rows[0].event.StreamID, rows[0].event.Sequence); err != nil {
					t.Fatal(err)
				}
				fresh, err := repo.reloadAgentDeliverySelectedCandidates(t.Context(), tx, selected, 2048, repo.nowUTC().Add(-time.Hour))
				if err != nil || len(fresh) != count || !fresh[0].hasPendingEffect || fresh[0].canPrune {
					t.Fatalf("prepared statement must reload fresh protected evidence: rows=%d error=%v", len(fresh), err)
				}
			})
		}
	}
}

func TestDeliveryRetentionStorageReloadCanceledPreparation(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, _ := seedRetentionBoundsStream(t, engine, "reload-cancel", 2, 0)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if err := repo.prepareAgentDeliveryReloadStatement(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled preparation=%v", err)
			}
			if _, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{}); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled first batch=%v", err)
			}
			result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || result.EventRowsRetired != 2 {
				t.Fatalf("cancellation must not poison following reload: %+v %v", result, err)
			}
		})
	}
}

func TestDeliveryRetentionStorageReloadBlockedPreparation(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, _ := seedRetentionBoundsStream(t, engine, "reload-blocked", 2, 0)
			holder, err := repo.db.BeginTxx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
			err = repo.prepareAgentDeliveryReloadStatement(ctx)
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				_ = holder.Rollback()
				t.Fatalf("blocked first preparation must cancel: %v", err)
			}
			if err := holder.Rollback(); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.agentDeliveryReloadStatement(); err == nil {
				t.Fatal("failed preparation must not publish a cached statement")
			}
			result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || result.EventRowsRetired != 2 {
				t.Fatalf("retry after canceled driver preparation failed: %+v %v", result, err)
			}
		})
	}
}

func TestDeliveryRetentionStorageReloadReopen(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "reload-reopen", 1, 0)
			seedRetentionStorageRows(t, repo, events[0], 201, 1024)
			result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || result.EventRowsRetired != 200 {
				t.Fatalf("initial cached batch=%+v %v", result, err)
			}
			if _, err := repo.agentDeliveryReloadStatement(); err != nil {
				t.Fatal(err)
			}
			if err := repo.closeAgentDeliveryReloadStatement(); err != nil {
				t.Fatal(err)
			}
			var one int
			if err := repo.db.QueryRow(`SELECT 1`).Scan(&one); err != nil {
				t.Fatalf("closing owned statements must preserve shared database: %v", err)
			}
			reopened := reopenRetentionStorageRepository(t, repo)
			if engine == "Postgres" {
				connection, err := reopened.db.Conn(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				err = connection.Raw(func(any) error { return driver.ErrBadConn })
				_ = connection.Close()
				if !errors.Is(err, driver.ErrBadConn) {
					t.Fatalf("discard reopen fixture connection: %v", err)
				}
			}
			if err := reopened.CheckAgentDeliveryRetentionReady(t.Context()); err != nil {
				t.Fatal(err)
			}
			result, err = reopened.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || result.EventRowsRetired != 1 {
				t.Fatalf("reopened cache must prepare against current ready schema: %+v %v", result, err)
			}
		})
	}
}

func TestDeliveryRetentionStorageReloadReuseAndReadiness(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "reload-reuse", 1, 0)
			seedRetentionStorageRows(t, repo, events[0], 300, 1024)
			if err := repo.prepareAgentDeliveryReloadStatement(t.Context()); err != nil {
				t.Fatal(err)
			}
			statement, err := repo.agentDeliveryReloadStatement()
			if err != nil {
				t.Fatal(err)
			}
			for _, count := range []int{1, 17, 200} {
				result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{MaxCandidates: count})
				if err != nil || result.EventRowsRetired != count {
					t.Fatalf("reused statement batch limit %d: %+v %v", count, result, err)
				}
				current, err := repo.agentDeliveryReloadStatement()
				if err != nil || current != statement {
					t.Fatal("bounded sizes must reuse the same statement without caching evidence")
				}
			}
			if _, err := repo.db.Exec(`DROP INDEX idx_agent_delivery_inbox_retention_eligible`); err != nil {
				t.Fatal(err)
			}
			if err := repo.CheckAgentDeliveryRetentionReady(t.Context()); err == nil {
				t.Fatal("cached statement must not hide an unavailable retention schema")
			}
			if err := repo.initAgentDeliveryRetentionSchema(); err != nil {
				t.Fatal(err)
			}
			result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || result.EventRowsRetired != 82 {
				t.Fatalf("cached statement must revalidate after schema repair: %+v %v", result, err)
			}
		})
	}
}
