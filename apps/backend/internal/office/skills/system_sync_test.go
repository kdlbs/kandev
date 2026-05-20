package skills_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/skills"
)

// stubSyncRepo is an in-memory implementation of SystemSyncRepo so
// the table-driven tests can drive insert / update / remove paths
// without spinning up SQLite. Each map is keyed by (workspaceID,
// slug) so we exercise per-workspace isolation.
type stubSyncRepo struct {
	rows   map[string]map[string]*models.Skill                // workspaceID → slug → row
	agents map[string]map[string]*settingsmodels.AgentProfile // workspaceID → agentID → profile
}

func newStubSyncRepo() *stubSyncRepo {
	return &stubSyncRepo{
		rows:   map[string]map[string]*models.Skill{},
		agents: map[string]map[string]*settingsmodels.AgentProfile{},
	}
}

func (s *stubSyncRepo) ListSystemSkills(
	_ context.Context, workspaceID string,
) ([]*models.Skill, error) {
	ws := s.rows[workspaceID]
	out := make([]*models.Skill, 0, len(ws))
	for _, sk := range ws {
		if sk.IsSystem {
			out = append(out, sk)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

func (s *stubSyncRepo) GetSkillBySlug(
	_ context.Context, workspaceID, slug string,
) (*models.Skill, error) {
	if ws, ok := s.rows[workspaceID]; ok {
		if sk, ok := ws[slug]; ok {
			return sk, nil
		}
	}
	return nil, errors.New("not found")
}

func (s *stubSyncRepo) CreateSkill(_ context.Context, skill *models.Skill) error {
	if _, ok := s.rows[skill.WorkspaceID]; !ok {
		s.rows[skill.WorkspaceID] = map[string]*models.Skill{}
	}
	copy := *skill
	if copy.ID == "" {
		copy.ID = skill.WorkspaceID + ":" + skill.Slug
	}
	s.rows[skill.WorkspaceID][skill.Slug] = &copy
	return nil
}

func (s *stubSyncRepo) UpdateSkill(_ context.Context, skill *models.Skill) error {
	if ws, ok := s.rows[skill.WorkspaceID]; ok {
		if _, ok := ws[skill.Slug]; ok {
			copy := *skill
			ws[skill.Slug] = &copy
			return nil
		}
	}
	return errors.New("not found for update")
}

func (s *stubSyncRepo) DeleteSkill(_ context.Context, id string) error {
	for _, ws := range s.rows {
		for slug, sk := range ws {
			if sk.ID == id {
				delete(ws, slug)
				return nil
			}
		}
	}
	return errors.New("not found for delete")
}

func (s *stubSyncRepo) ListAgentInstances(
	_ context.Context, workspaceID string,
) ([]*settingsmodels.AgentProfile, error) {
	ws := s.agents[workspaceID]
	out := make([]*settingsmodels.AgentProfile, 0, len(ws))
	for _, a := range ws {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *stubSyncRepo) UpdateAgentInstance(
	_ context.Context, agent *settingsmodels.AgentProfile,
) error {
	ws, ok := s.agents[agent.WorkspaceID]
	if !ok {
		return errors.New("workspace not found")
	}
	if _, ok := ws[agent.ID]; !ok {
		return errors.New("agent not found")
	}
	copy := *agent
	ws[agent.ID] = &copy
	return nil
}

// TestSyncSystemSkills_InsertsBundledSkillsForFreshWorkspace pins
// that on a workspace with no rows yet, every embedded SKILL.md
// that declares `kandev.system: true` gets inserted with is_system
// = true and the role defaults from frontmatter.
func TestSyncSystemSkills_InsertsBundledSkillsForFreshWorkspace(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	report, err := skills.SyncSystemSkills(context.Background(), repo, []string{"ws-1"}, nil, log)
	if err != nil {
		t.Fatalf("SyncSystemSkills error: %v", err)
	}
	if len(report.Inserted) == 0 {
		t.Fatalf("expected inserts on fresh workspace, got none")
	}
	rows, _ := repo.ListSystemSkills(context.Background(), "ws-1")
	if len(rows) != len(report.Inserted) {
		t.Fatalf("row count mismatch: rows=%d inserted=%d", len(rows), len(report.Inserted))
	}
	for _, r := range rows {
		if !r.IsSystem {
			t.Errorf("row %s missing is_system flag", r.Slug)
		}
		if r.SourceType != skills.SourceTypeSystem {
			t.Errorf("row %s source_type = %q, want %q", r.Slug, r.SourceType, skills.SourceTypeSystem)
		}
		if r.ContentHash == "" {
			t.Errorf("row %s missing content_hash", r.Slug)
		}
	}
}

// TestSyncSystemSkills_UpdatesChangedContentInPlace pins that a
// content drift triggers an in-place UpdateSkill, preserving the
// row ID (and thereby per-agent desired_skills references).
func TestSyncSystemSkills_UpdatesChangedContentInPlace(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	if _, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, nil, log,
	); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	before, _ := repo.ListSystemSkills(context.Background(), "ws-1")
	if len(before) == 0 {
		t.Fatalf("expected rows after first sync")
	}
	// Mutate one row's content_hash so the second pass treats it as drifted.
	target := before[0]
	originalID := target.ID
	target.ContentHash = "stale"
	target.Content = "stale content"
	repo.rows["ws-1"][target.Slug] = target

	report, err := skills.SyncSystemSkills(context.Background(), repo, []string{"ws-1"}, nil, log)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(report.Updated) == 0 {
		t.Fatalf("expected at least one update, got %v", report.Updated)
	}
	got, _ := repo.GetSkillBySlug(context.Background(), "ws-1", target.Slug)
	if got.ID != originalID {
		t.Errorf("row ID changed across update: was %s, now %s", originalID, got.ID)
	}
	if got.ContentHash == "stale" {
		t.Error("content_hash was not refreshed")
	}
}

// TestSyncSystemSkills_RemovesOrphanedSystemRows pins that a
// previously-bundled slug which is no longer present in the embed
// gets deleted from office_skills. Simulates a kandev release that
// retires a system skill.
func TestSyncSystemSkills_RemovesOrphanedSystemRows(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	// Seed an orphan: a system row whose slug is NOT in the bundle.
	repo.rows["ws-1"] = map[string]*models.Skill{}
	orphan := &models.Skill{
		ID:          "orphan-1",
		WorkspaceID: "ws-1",
		Slug:        "kandev-legacy-removed",
		Name:        "kandev-legacy-removed",
		IsSystem:    true,
		SourceType:  skills.SourceTypeSystem,
	}
	repo.rows["ws-1"]["kandev-legacy-removed"] = orphan

	report, err := skills.SyncSystemSkills(context.Background(), repo, []string{"ws-1"}, nil, log)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Removed) == 0 {
		t.Fatalf("expected an orphan removal, got %v", report.Removed)
	}
	if _, err := repo.GetSkillBySlug(context.Background(), "ws-1", "kandev-legacy-removed"); err == nil {
		t.Error("orphan still present after sync")
	}
}

// TestSyncSystemSkills_NoChangeOnSecondPass pins that running the
// sync twice without any drift produces zero inserts/updates/
// removes (and therefore zero DB writes on a hot path).
func TestSyncSystemSkills_NoChangeOnSecondPass(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	if _, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, nil, log,
	); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	report, err := skills.SyncSystemSkills(context.Background(), repo, []string{"ws-1"}, nil, log)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(report.Inserted) != 0 || len(report.Updated) != 0 || len(report.Removed) != 0 {
		t.Errorf("expected no-op second pass, got %+v", report)
	}
}

// TestSyncSystemSkills_UpdatesContentWhenBundledHashDiffers pins the
// "kandev release rev'd the SKILL.md body" scenario: an existing row
// matches the slug but the bundled `bundled` spec carries a newer
// content + hash. SyncSystemSkills must run UpdateSkill so the row
// reflects the new body, version, and hash without changing the row
// ID. Uses an injected synthetic spec to avoid mutating the //go:embed
// FS.
func TestSyncSystemSkills_UpdatesContentWhenBundledHashDiffers(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	const slug = "drift-skill"
	const initialHash = "hash-v1"
	const newHash = "hash-v2"
	const newBody = "## Updated guidance body"

	// Seed an existing system row representing the prior release.
	repo.rows["ws-1"] = map[string]*models.Skill{
		slug: {
			ID:            "skill-drift-1",
			WorkspaceID:   "ws-1",
			Slug:          slug,
			Name:          "Drift Skill",
			SourceType:    skills.SourceTypeSystem,
			SourceLocator: "bundled:" + slug,
			Content:       "## Old guidance body",
			ContentHash:   initialHash,
			Version:       "1.0.0",
			IsSystem:      true,
			SystemVersion: "1.0.0",
			ApprovalState: "approved",
		},
	}

	bundled := []skills.SystemSkillSpec{{
		Slug:        slug,
		Name:        "Drift Skill",
		Description: "Drift demo",
		Version:     "2.0.0",
		Content:     newBody,
		ContentHash: newHash,
	}}

	report, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, bundled, log,
	)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Updated) != 1 || !strings.HasSuffix(report.Updated[0], slug) {
		t.Fatalf("expected one update for %s, got %v", slug, report.Updated)
	}
	got, err := repo.GetSkillBySlug(context.Background(), "ws-1", slug)
	if err != nil {
		t.Fatalf("get after sync: %v", err)
	}
	if got.ID != "skill-drift-1" {
		t.Errorf("row ID changed across update: now %s", got.ID)
	}
	if got.ContentHash != newHash {
		t.Errorf("content_hash = %q, want %q", got.ContentHash, newHash)
	}
	if got.Content != newBody {
		t.Errorf("content = %q, want %q", got.Content, newBody)
	}
	if got.Version != "2.0.0" {
		t.Errorf("version = %q, want 2.0.0", got.Version)
	}
}

// TestSyncSystemSkills_RemovesOrphanedSlugAndDetachesFromAgents pins
// the "kandev release retired a bundled skill" scenario: a system
// row whose slug is missing from the injected `bundled` slice must be
// deleted, AND its ID must be stripped from every agent_profiles
// row's skill_ids JSON array in the same workspace. Other agents'
// untouched IDs and other-workspace agents must not be modified.
func TestSyncSystemSkills_RemovesOrphanedSlugAndDetachesFromAgents(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	const orphanID = "skill-retired-1"
	const keptID = "skill-other-1"

	// Seed a system skill that is no longer in the bundle, plus one
	// that is — so we can prove only the orphan is detached.
	repo.rows["ws-1"] = map[string]*models.Skill{
		"retired-slug": {
			ID:          orphanID,
			WorkspaceID: "ws-1",
			Slug:        "retired-slug",
			Name:        "Retired",
			IsSystem:    true,
			SourceType:  skills.SourceTypeSystem,
			ContentHash: "hash-old",
		},
	}

	// One agent in ws-1 references both the orphan and a kept ID.
	repo.agents["ws-1"] = map[string]*settingsmodels.AgentProfile{
		"agent-1": {
			ID:          "agent-1",
			WorkspaceID: "ws-1",
			SkillIDs:    mustJSONArray(t, []string{orphanID, keptID}),
		},
		"agent-2": {
			ID:          "agent-2",
			WorkspaceID: "ws-1",
			SkillIDs:    mustJSONArray(t, []string{keptID}),
		},
	}
	// Agent in a different workspace that happens to share the orphan
	// ID — must NOT be touched because we only scrub the workspace
	// whose system row was deleted.
	repo.agents["ws-2"] = map[string]*settingsmodels.AgentProfile{
		"agent-other": {
			ID:          "agent-other",
			WorkspaceID: "ws-2",
			SkillIDs:    mustJSONArray(t, []string{orphanID}),
		},
	}

	// Bundled set is empty for ws-1 → orphan must be deleted.
	report, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, []skills.SystemSkillSpec{}, log,
	)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Removed) != 1 || !strings.HasSuffix(report.Removed[0], "retired-slug") {
		t.Fatalf("expected one removal for retired-slug, got %v", report.Removed)
	}

	if _, err := repo.GetSkillBySlug(context.Background(), "ws-1", "retired-slug"); err == nil {
		t.Error("orphan office_skills row still present after sync")
	}

	// Agent in ws-1 that had the orphan ID must now omit it but keep the other ID.
	a1 := repo.agents["ws-1"]["agent-1"]
	got1 := decodeIDs(t, a1.SkillIDs)
	if containsID(got1, orphanID) {
		t.Errorf("agent-1.skill_ids still contains orphan %q: %v", orphanID, got1)
	}
	if !containsID(got1, keptID) {
		t.Errorf("agent-1.skill_ids dropped kept ID %q: %v", keptID, got1)
	}

	// Agent in ws-1 that didn't reference the orphan must be untouched.
	a2 := repo.agents["ws-1"]["agent-2"]
	got2 := decodeIDs(t, a2.SkillIDs)
	if len(got2) != 1 || got2[0] != keptID {
		t.Errorf("agent-2.skill_ids unexpectedly mutated: %v", got2)
	}

	// Agent in ws-2 must not be touched even though its skill_ids
	// references the same string ID — the sync scope is per-workspace.
	other := repo.agents["ws-2"]["agent-other"]
	gotOther := decodeIDs(t, other.SkillIDs)
	if len(gotOther) != 1 || gotOther[0] != orphanID {
		t.Errorf("ws-2 agent must not be scrubbed: %v", gotOther)
	}
}

func mustJSONArray(t *testing.T, ids []string) string {
	t.Helper()
	b, err := json.Marshal(ids)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func decodeIDs(t *testing.T, raw string) []string {
	t.Helper()
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal %q: %v", raw, err)
	}
	return out
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
