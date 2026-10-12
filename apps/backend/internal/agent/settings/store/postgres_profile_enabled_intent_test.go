package store

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/settings/models"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

func enabledIntentPostgresConnection(t *testing.T, dsn, schema string) *sqlx.DB {
	t.Helper()
	raw, err := internaldb.OpenPostgres(dsn, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(raw, "pgx")
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec("SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	return database
}

func enabledIntentPostgresFixture(t *testing.T, dsn string, dynamic bool) (*sqliteRepository, *sqlx.DB, *sqlx.DB, *models.AgentProfile, *models.DynamicAgentProfile, []models.DynamicAgentRoute) {
	t.Helper()
	database := testutil.OpenIsolatedPostgres(t, dsn)
	var schema string
	if err := database.Get(&schema, "SELECT current_schema()"); err != nil {
		t.Fatal(err)
	}
	holder := enabledIntentPostgresConnection(t, dsn, schema)
	monitor := enabledIntentPostgresConnection(t, dsn, schema)
	repo, err := newSQLiteRepositoryWithDB(database, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := seedAgentProfile(t, repo, "Original", "intent-agent")
	profile, err := repo.GetAgentProfile(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !dynamic {
		return repo, holder, monitor, profile, nil, nil
	}
	candidate := seedAgentProfile(t, repo, "Candidate", "candidate-agent")
	doc := &models.DynamicAgentProfile{ProfileID: id}
	routes := []models.DynamicAgentRoute{{DynamicProfileID: id, Position: 0, ExecutionProfileID: candidate, Enabled: true, RulesJSON: `{}`}}
	if err := repo.CreateDynamicAgentProfile(context.Background(), doc, routes); err != nil {
		t.Fatal(err)
	}
	return repo, holder, monitor, profile, doc, routes
}

// @covers AC-PLATFORM-AGENT-SETTINGS-PARITY-002.2 AC-PLATFORM-AGENT-SETTINGS-PARITY-002.10
func TestProfileEnabledIntentPostgres(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	for _, dynamic := range []bool{false, true} {
		for _, initial := range []bool{false, true} {
			for _, commit := range []bool{false, true} {
				t.Run(fmt.Sprintf("dynamic_%t_initial_%t_holder_commit_%t", dynamic, initial, commit), func(t *testing.T) {
					testEnabledIntentPostgresWait(t, dsn, dynamic, initial, commit)
				})
			}
		}
	}
	t.Run("rollback_and_explicit_controls", func(t *testing.T) { testEnabledIntentPostgresControls(t, dsn) })
}

func enabledIntentPostgresPIDs(t *testing.T, writer, holder, monitor *sqlx.DB) int {
	t.Helper()
	pids := make([]int, 3)
	for i, database := range []*sqlx.DB{writer, holder, monitor} {
		if err := database.Get(&pids[i], "SELECT pg_backend_pid()"); err != nil {
			t.Fatal(err)
		}
	}
	if pids[0] == pids[1] || pids[0] == pids[2] || pids[1] == pids[2] {
		t.Fatalf("connections are not physically distinct: %v", pids)
	}
	t.Logf("physical PostgreSQL writer/holder/monitor PIDs: %v", pids)
	return pids[0]
}

func waitEnabledIntentPostgresLock(ctx context.Context, monitor *sqlx.DB, pid int) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		err := monitor.GetContext(ctx, &waiting, "SELECT COALESCE(wait_event_type = 'Lock', false) FROM pg_stat_activity WHERE pid = $1", pid)
		if err != nil {
			return err
		}
		if waiting {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func testEnabledIntentPostgresWait(t *testing.T, dsn string, dynamic, initial, commit bool) {
	t.Helper()
	repo, holder, monitor, profile, doc, routes := enabledIntentPostgresFixture(t, dsn, dynamic)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	writerPID := enabledIntentPostgresPIDs(t, repo.db, holder, monitor)
	if _, err := repo.UpdateAgentProfileEnabled(ctx, profile.ID, initial); err != nil {
		t.Fatal(err)
	}
	profile.Enabled, profile.Name = initial, "Edited"
	tx, err := holder.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	holderValue := 0
	if !initial {
		holderValue = 1
	}
	if _, err := tx.ExecContext(ctx, "UPDATE agent_profiles SET enabled = $1 WHERE id = $2", holderValue, profile.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		if dynamic {
			done <- repo.UpdateAgentProfileWithDynamicEnabledIntent(ctx, profile, doc, doc.Version, routes, nil)
		} else {
			done <- repo.UpdateAgentProfileWithEnabledIntent(ctx, profile, nil)
		}
	}()
	joined := false
	defer func() {
		cancel()
		_ = tx.Rollback()
		if !joined {
			<-done
		}
	}()
	if err := waitEnabledIntentPostgresLock(ctx, monitor, writerPID); err != nil {
		t.Fatal(err)
	}
	t.Logf("observed actual row lock wait for writer PID %d", writerPID)
	wanted := initial
	if commit {
		wanted = !initial
		err = tx.Commit()
	} else {
		err = tx.Rollback()
	}
	if err != nil {
		t.Fatal(err)
	}
	writeErr := <-done
	joined = true
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	saved, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil || saved.Enabled != wanted || profile.Enabled != wanted || saved.Name != "Edited" || saved.Model != "test-model" {
		t.Fatalf("saved=%#v captured=%t want=%t error=%v", saved, profile.Enabled, wanted, err)
	}
	if dynamic {
		savedDoc, savedRoutes, err := repo.GetDynamicAgentProfile(ctx, profile.ID)
		if err != nil || savedDoc.Version != 2 || len(savedRoutes) != 1 {
			t.Fatalf("dynamic result=%#v routes=%#v error=%v", savedDoc, savedRoutes, err)
		}
	}
}

func testEnabledIntentPostgresControls(t *testing.T, dsn string) {
	for _, failure := range []string{"stale_version", "missing_parent", "route_insert"} {
		t.Run(failure, func(t *testing.T) {
			repo, _, _, profile, doc, routes := enabledIntentPostgresFixture(t, dsn, true)
			ctx := context.Background()
			if _, err := repo.UpdateAgentProfileEnabled(ctx, profile.ID, false); err != nil {
				t.Fatal(err)
			}
			if failure == "missing_parent" {
				if _, err := repo.db.Exec("DELETE FROM dynamic_agent_profiles WHERE profile_id = $1", profile.ID); err != nil {
					t.Fatal(err)
				}
			}
			before, err := repo.GetAgentProfile(ctx, profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			expected := doc.Version
			if failure == "stale_version" {
				expected++
			}
			if failure == "route_insert" {
				routes[0].ExecutionProfileID = "missing-candidate"
			}
			profile.Name = "Rejected"
			if err := repo.UpdateAgentProfileWithDynamicEnabledIntent(ctx, profile, doc, expected, routes, nil); err == nil {
				t.Fatal("invalid atomic update succeeded")
			}
			after, err := repo.GetAgentProfile(ctx, profile.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("rollback changed base row: before=%#v after=%#v error=%v", before, after, err)
			}
			if failure != "missing_parent" {
				savedDoc, savedRoutes, err := repo.GetDynamicAgentProfile(ctx, profile.ID)
				if err != nil || savedDoc.Version != 1 || len(savedRoutes) != 1 || savedRoutes[0].ExecutionProfileID == "missing-candidate" {
					t.Fatalf("rollback changed routes: %#v %#v %v", savedDoc, savedRoutes, err)
				}
			}
		})
	}
	for _, dynamic := range []bool{false, true} {
		for _, legacy := range []bool{false, true} {
			for _, wanted := range []bool{false, true} {
				t.Run(fmt.Sprintf("explicit_dynamic_%t_legacy_%t_value_%t", dynamic, legacy, wanted), func(t *testing.T) {
					repo, _, _, profile, doc, routes := enabledIntentPostgresFixture(t, dsn, dynamic)
					ctx := context.Background()
					if _, err := repo.UpdateAgentProfileEnabled(ctx, profile.ID, !wanted); err != nil {
						t.Fatal(err)
					}
					profile.Enabled, profile.Name = wanted, "Explicit"
					var err error
					switch {
					case dynamic && legacy:
						err = repo.UpdateAgentProfileWithDynamic(ctx, profile, doc, doc.Version, routes)
					case dynamic:
						err = repo.UpdateAgentProfileWithDynamicEnabledIntent(ctx, profile, doc, doc.Version, routes, &wanted)
					case legacy:
						err = repo.UpdateAgentProfile(ctx, profile)
					default:
						err = repo.UpdateAgentProfileWithEnabledIntent(ctx, profile, &wanted)
					}
					if err != nil {
						t.Fatal(err)
					}
					saved, err := repo.GetAgentProfile(ctx, profile.ID)
					if err != nil || saved.Enabled != wanted || profile.Enabled != wanted || saved.Name != "Explicit" {
						t.Fatalf("explicit/full update=%#v captured=%t error=%v", saved, profile.Enabled, err)
					}
				})
			}
		}
	}
}
