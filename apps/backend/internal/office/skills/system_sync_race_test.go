package skills_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/skills"
)

var syncTestHooks = struct {
	sync.Mutex
	createSkillErr       map[*stubSyncRepo]map[string]error
	normalizeConflictFor map[*stubSyncRepo]map[string]bool
}{
	createSkillErr:       map[*stubSyncRepo]map[string]error{},
	normalizeConflictFor: map[*stubSyncRepo]map[string]bool{},
}

func syncTestCreateSkillError(repo *stubSyncRepo, slug string) error {
	syncTestHooks.Lock()
	defer syncTestHooks.Unlock()
	return syncTestHooks.createSkillErr[repo][slug]
}

func setSyncTestCreateSkillError(repo *stubSyncRepo, slug string, err error) {
	syncTestHooks.Lock()
	defer syncTestHooks.Unlock()
	if syncTestHooks.createSkillErr[repo] == nil {
		syncTestHooks.createSkillErr[repo] = map[string]error{}
	}
	syncTestHooks.createSkillErr[repo][slug] = err
}

func setSyncTestNormalizeConflict(repo *stubSyncRepo, slug string) {
	syncTestHooks.Lock()
	defer syncTestHooks.Unlock()
	if syncTestHooks.normalizeConflictFor[repo] == nil {
		syncTestHooks.normalizeConflictFor[repo] = map[string]bool{}
	}
	syncTestHooks.normalizeConflictFor[repo][slug] = true
}

func syncTestNormalizeConflict(repo *stubSyncRepo, slug string) bool {
	syncTestHooks.Lock()
	defer syncTestHooks.Unlock()
	return syncTestHooks.normalizeConflictFor[repo][slug]
}

// NormalizeSkillSlug mirrors the repository contract: the slug row and
// every affected desired_skills profile are committed as one operation.
// The in-memory implementation applies all changes only after it has
// confirmed that every profile update can succeed.
func (s *stubSyncRepo) NormalizeSkillSlug(
	_ context.Context, workspaceID, skillID, oldSlug, newSlug string,
) (bool, error) {
	ws := s.rows[workspaceID]
	row, ok := ws[oldSlug]
	if !ok || row.ID != skillID || row.IsSystem {
		return false, nil
	}
	if syncTestNormalizeConflict(s, oldSlug) {
		ws[newSlug] = &models.Skill{
			ID:          "concurrent-" + newSlug,
			WorkspaceID: workspaceID,
			Slug:        newSlug,
			IsSystem:    false,
		}
		return false, &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "office_skills_workspace_id_slug_key",
		}
	}
	if _, exists := ws[newSlug]; exists {
		return false, &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "office_skills_workspace_id_slug_key",
		}
	}

	type agentUpdate struct {
		id      string
		desired string
	}
	updates := make([]agentUpdate, 0)
	for id, agent := range s.agents[workspaceID] {
		desired, changed := replaceStubJSONArrayValue(agent.DesiredSkills, oldSlug, newSlug)
		if !changed {
			continue
		}
		if s.failUpdateAgentFor[id] {
			return false, errors.New("injected failure: agent update unavailable")
		}
		updates = append(updates, agentUpdate{id: id, desired: desired})
	}

	delete(ws, oldSlug)
	copyRow := *row
	copyRow.Slug = newSlug
	ws[newSlug] = &copyRow
	for _, update := range updates {
		copyAgent := *s.agents[workspaceID][update.id]
		copyAgent.DesiredSkills = update.desired
		s.agents[workspaceID][update.id] = &copyAgent
	}
	return true, nil
}

func replaceStubJSONArrayValue(raw, oldValue, newValue string) (string, bool) {
	values := skills.ParseDesiredSlugs(raw)
	if len(values) == 0 {
		return raw, false
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	changed := false
	for _, value := range values {
		if value == oldValue {
			value = newValue
			changed = true
		}
		if value == "" || seen[value] {
			if value != "" {
				changed = true
			}
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	if !changed {
		return raw, false
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return raw, false
	}
	return string(encoded), true
}

func TestSyncSystemSkills_NormalizationConflictLeavesReferencesUntouched(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	const oldSlug = "my-custom-skill"
	repo.rows["ws-1"] = map[string]*models.Skill{
		oldSlug: {
			ID:          "user-skill-1",
			WorkspaceID: "ws-1",
			Slug:        oldSlug,
			Name:        "My Custom Skill",
			IsSystem:    false,
			SourceType:  "inline",
		},
	}
	repo.agents["ws-1"] = map[string]*settingsmodels.AgentProfile{
		"agent-1": {
			ID:            "agent-1",
			WorkspaceID:   "ws-1",
			DesiredSkills: mustJSONArray(t, []string{oldSlug}),
		},
	}
	setSyncTestNormalizeConflict(repo, oldSlug)

	report, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, []skills.SystemSkillSpec{}, log,
	)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Normalized) != 0 {
		t.Fatalf("conflicting normalization must not be reported as complete: %v", report.Normalized)
	}
	if len(report.Conflicted) != 1 || !strings.HasSuffix(report.Conflicted[0], oldSlug) {
		t.Fatalf("expected %s reported as conflicted, got %v", oldSlug, report.Conflicted)
	}
	if _, err := repo.GetSkillBySlug(context.Background(), "ws-1", oldSlug); err != nil {
		t.Fatalf("original user row must remain after conflict: %v", err)
	}
	agent1 := repo.agents["ws-1"]["agent-1"]
	if got := decodeIDs(t, agent1.DesiredSkills); len(got) != 1 || got[0] != oldSlug {
		t.Errorf("agent desired_skills changed despite atomic conflict: %v", got)
	}
}

func TestSyncSystemSkills_ContinuesAfterPostgresSlugConflict(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()
	setSyncTestCreateSkillError(repo, "kandev-conflicting", &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "office_skills_workspace_id_slug_key",
	})

	bundled := []skills.SystemSkillSpec{
		{Slug: "kandev-conflicting", Name: "Conflict", ContentHash: "hash-conflict"},
		{Slug: "kandev-next", Name: "Next", ContentHash: "hash-next"},
	}
	report, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, bundled, log,
	)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Conflicted) != 1 || !strings.HasSuffix(report.Conflicted[0], "kandev-conflicting") {
		t.Fatalf("expected PostgreSQL slug conflict to be reported, got %v", report.Conflicted)
	}
	if len(report.Inserted) != 1 || !strings.HasSuffix(report.Inserted[0], "kandev-next") {
		t.Fatalf("sync should continue after PostgreSQL conflict, inserted=%v", report.Inserted)
	}
}
