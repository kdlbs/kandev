package skills_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/skills"
)

func TestListSkillsFromConfigBackfillsExistingCEOWhenLazySyncAddsDefaults(t *testing.T) {
	t.Parallel()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("provide settings store: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	ctx := context.Background()
	agentService := officeagents.NewAgentService(repo, logger.Default(), &noopActivitySkills{})
	skillService := skills.NewSkillService(repo, logger.Default(), &noopActivitySkills{}, agentService, nil)
	agent := &models.AgentInstance{
		ID:            "agent-ceo",
		WorkspaceID:   "ws-lazy-skills",
		Name:          "CEO",
		Role:          models.AgentRoleCEO,
		Status:        models.AgentStatusIdle,
		DesiredSkills: "[]",
		SkillIDs:      "[]",
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create pre-existing CEO: %v", err)
	}

	if _, err := skillService.ListSkillsFromConfig(ctx, agent.WorkspaceID); err != nil {
		t.Fatalf("list skills after lazy sync: %v", err)
	}
	got, err := agentService.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get backfilled CEO: %v", err)
	}
	var desiredSlugs []string
	if err := json.Unmarshal([]byte(got.DesiredSkills), &desiredSlugs); err != nil {
		t.Fatalf("decode desired skills %q: %v", got.DesiredSkills, err)
	}
	for _, slug := range []string{"kandev-protocol", "memory", "kandev-team-admin"} {
		found := false
		for _, desiredSlug := range desiredSlugs {
			if desiredSlug == slug {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("desired skills %v do not contain role default %q", desiredSlugs, slug)
		}
	}
}
