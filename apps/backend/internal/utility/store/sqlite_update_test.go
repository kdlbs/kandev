package store

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/utility/models"
)

func TestUpdateAgentDisableSurvivesSchemaReplay(t *testing.T) {
	cases := []struct {
		name           string
		creationOffset time.Duration
		wantAdvance    bool
	}{
		{name: "equal timestamps", wantAdvance: true},
		{name: "later within the same microsecond", creationOffset: -100 * time.Nanosecond, wantAdvance: true},
		{name: "clock before creation", creationOffset: time.Hour, wantAdvance: true},
		{name: "later microsecond", creationOffset: -time.Microsecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			database := openTestDB(t)
			repo, err := newSQLiteRepositoryWithDB(database, database)
			if err != nil {
				t.Fatalf("initialize schema: %v", err)
			}
			synctest.Test(t, func(t *testing.T) {
				// Put the frozen clock inside a microsecond so a later instant
				// can still have the same stored PostgreSQL timestamp.
				time.Sleep(500 * time.Nanosecond)
				now := time.Now().UTC()
				ctx := context.Background()
				agent := &models.UtilityAgent{
					Name:      "timestamp-regression",
					Prompt:    "prompt",
					AgentID:   "claude-acp",
					Model:     "test-model",
					Enabled:   true,
					CreatedAt: now.Add(tc.creationOffset),
				}
				if err := repo.CreateAgent(ctx, agent); err != nil {
					t.Fatalf("create agent: %v", err)
				}
				agent, err := repo.GetAgentByID(ctx, agent.ID)
				if err != nil {
					t.Fatalf("read created agent: %v", err)
				}
				agent.Enabled = false
				if err := repo.UpdateAgent(ctx, agent); err != nil {
					t.Fatalf("disable agent: %v", err)
				}
				want := now
				if tc.wantAdvance {
					want = agent.CreatedAt.Add(time.Microsecond)
				}
				if !agent.UpdatedAt.Equal(want) {
					t.Fatalf("updated_at = %s, want %s", agent.UpdatedAt, want)
				}
				assertUtilityDisableSurvivesReplay(t, database, agent.ID)
			})
		})
	}
}

func TestUpdateAgentDisableSurvivesPostgresSchemaReplay(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(database, database)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	ctx := context.Background()
	agent := &models.UtilityAgent{
		Name:      "timestamp-regression",
		Prompt:    "prompt",
		AgentID:   "claude-acp",
		Model:     "test-model",
		Enabled:   true,
		CreatedAt: time.Now().UTC().Add(time.Hour),
	}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err = repo.GetAgentByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("read created agent: %v", err)
	}
	agent.Enabled = false
	if err := repo.UpdateAgent(ctx, agent); err != nil {
		t.Fatalf("disable agent: %v", err)
	}
	assertUtilityDisableSurvivesReplay(t, database, agent.ID)
}

func assertUtilityDisableSurvivesReplay(t *testing.T, database *sqlx.DB, id string) {
	t.Helper()
	for pass := range 2 {
		repo, err := newSQLiteRepositoryWithDB(database, database)
		if err != nil {
			t.Fatalf("replay schema %d: %v", pass, err)
		}
		agent, err := repo.GetAgentByID(context.Background(), id)
		if err != nil {
			t.Fatalf("read disabled agent: %v", err)
		}
		if agent.Enabled {
			t.Fatalf("schema replay %d enabled the explicitly disabled agent", pass)
		}
		if !agent.UpdatedAt.Truncate(time.Microsecond).After(agent.CreatedAt.Truncate(time.Microsecond)) {
			t.Fatalf("stored updated_at %s is not after created_at %s at microsecond precision", agent.UpdatedAt, agent.CreatedAt)
		}
	}
}
