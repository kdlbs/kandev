package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestProfileRevisionCoversEveryWriterAndABA(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		repo := openSQLiteProfileOrderRaceRepo(t)
		assertProfileRevisionCoversEveryWriterAndABA(t, repo)
	})
	t.Run("postgres", func(t *testing.T) {
		db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
		repo, err := newSQLiteRepositoryWithDB(db, db, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertProfileRevisionCoversEveryWriterAndABA(t, repo)
	})
}

func TestProfileRevisionIgnoresOperationalProfileUpdates(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		repo := openSQLiteProfileOrderRaceRepo(t)
		assertProfileRevisionIgnoresOperationalProfileUpdates(t, repo)
	})
	t.Run("postgres", func(t *testing.T) {
		db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
		repo, err := newSQLiteRepositoryWithDB(db, db, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertProfileRevisionIgnoresOperationalProfileUpdates(t, repo)
	})
}

func assertProfileRevisionIgnoresOperationalProfileUpdates(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	agent := &models.Agent{Name: "operational-revision-agent"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	profile := &models.AgentProfile{AgentID: agent.ID, Name: "operational-revision-profile", Model: "model-a"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	initial := readProfileRevision(t, repo, profile.ID)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE agent_profiles
		SET status = ?, working_run_id = ?, updated_at = ?, sort_order = ?,
			last_run_finished_at = ?, consecutive_failures = ?
		WHERE id = ?`), "working", "run-1", "2026-10-25T00:00:00Z", 4,
		"2026-10-25T00:00:00Z", 1, profile.ID); err != nil {
		t.Fatal(err)
	}
	if got := readProfileRevision(t, repo, profile.ID); got != initial {
		t.Fatalf("revision after operational update = %d, want unchanged %d", got, initial)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE agent_profiles SET model = ? WHERE id = ?`), "model-b", profile.ID); err != nil {
		t.Fatal(err)
	}
	if got := readProfileRevision(t, repo, profile.ID); got <= initial {
		t.Fatalf("revision after profile configuration update = %d, want > %d", got, initial)
	}
}

func assertProfileRevisionCoversEveryWriterAndABA(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	agent := &models.Agent{Name: "revision-agent"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	profile := &models.AgentProfile{AgentID: agent.ID, Name: "revision-profile", Model: "model-a"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	initial := readProfileRevision(t, repo, profile.ID)
	if initial <= 0 {
		t.Fatalf("initial revision = %d, want positive", initial)
	}
	assertProfileSnapshotRevision(t, repo, profile.ID, initial)
	for _, model := range []string{"model-b", "model-a"} {
		_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE agent_profiles SET model = ? WHERE id = ?`), model, profile.ID)
		if err != nil {
			t.Fatal(err)
		}
		changed := readProfileRevision(t, repo, profile.ID)
		if changed <= initial {
			t.Fatalf("revision after raw SQL update = %d, want > %d", changed, initial)
		}
		initial = changed
		assertProfileSnapshotRevision(t, repo, profile.ID, changed)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`DELETE FROM agent_profiles WHERE id = ?`), profile.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`), profile.ID, agent.ID, profile.Name, profile.Name, "model-a", profile.CreatedAt, profile.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if recreated := readProfileRevision(t, repo, profile.ID); recreated <= initial {
		t.Fatalf("revision after delete/reinsert = %d, want > %d", recreated, initial)
	}
}

func assertProfileSnapshotRevision(t *testing.T, repo *sqliteRepository, id string, want int64) {
	t.Helper()
	profile, err := repo.GetAgentProfile(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Revision != want {
		t.Fatalf("profile snapshot revision = %d, want %d", profile.Revision, want)
	}
}

func readProfileRevision(t *testing.T, repo *sqliteRepository, id string) int64 {
	t.Helper()
	revision, err := repo.GetAgentProfileRevision(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}
