package skills_test

import (
	"context"
	"strings"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/skills"
)

// TestSyncSystemSkills_ReplacesRetiredMemorySkillReference pins the
// concrete REQ-003 migration case this feature adds: the bundled
// `memory` skill was renamed to `kandev-memory` for naming-scheme
// consistency, so a workspace with the old row must retire it and end
// up with the new canonical slug in its place.
func TestSyncSystemSkills_ReplacesRetiredMemorySkillReference(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	repo.rows["ws-1"] = map[string]*models.Skill{
		"memory": {
			ID:          "skill-old-memory",
			WorkspaceID: "ws-1",
			Slug:        "memory",
			Name:        "Memory",
			IsSystem:    true,
			SourceType:  skills.SourceTypeSystem,
		},
	}

	bundled := []skills.SystemSkillSpec{{
		Slug:        "kandev-memory",
		Name:        "Memory",
		Description: "Memory guidance",
		Version:     "1.0.0",
		Content:     "---\nname: kandev-memory\ndescription: Memory guidance\n---\n# Memory\n",
		ContentHash: "hash-kandev-memory",
	}}

	report, err := skills.SyncSystemSkills(context.Background(), repo, []string{"ws-1"}, bundled, log)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Removed) != 1 || !strings.HasSuffix(report.Removed[0], "memory") {
		t.Fatalf("expected retirement of memory, got removed=%v", report.Removed)
	}
	if _, err := repo.GetSkillBySlug(context.Background(), "ws-1", "memory"); err == nil {
		t.Error("retired memory row still present after sync")
	}
	got, err := repo.GetSkillBySlug(context.Background(), "ws-1", "kandev-memory")
	if err != nil {
		t.Fatalf("replacement kandev-memory missing: %v", err)
	}
	if !got.IsSystem {
		t.Error("kandev-memory replacement missing is_system flag")
	}
}

// TestSyncSystemSkills_RetirementBlockedWhenReplacementMissing pins the
// F19-round fix: a retired skill with a configured replacement must
// not be deleted (and its agent references detached) until the
// replacement row actually exists in the workspace. Deleting first
// would strand every agent that referenced it with no rewritten
// reference. When the replacement did not land this pass, the row
// stays in place and is reported as blocked for a later pass to
// retry.
func TestSyncSystemSkills_RetirementBlockedWhenReplacementMissing(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	const oldTasksID = "skill-old-tasks"
	repo.rows["ws-1"] = map[string]*models.Skill{
		"kandev-tasks": {
			ID:          oldTasksID,
			WorkspaceID: "ws-1",
			Slug:        "kandev-tasks",
			Name:        "Tasks",
			IsSystem:    true,
			SourceType:  skills.SourceTypeSystem,
		},
	}
	repo.agents["ws-1"] = map[string]*settingsmodels.AgentProfile{
		"agent-1": {
			ID:            "agent-1",
			WorkspaceID:   "ws-1",
			SkillIDs:      mustJSONArray(t, []string{oldTasksID}),
			DesiredSkills: mustJSONArray(t, []string{"kandev-tasks"}),
		},
	}

	// kandev-task-ops (the configured replacement for kandev-tasks) is
	// neither an existing row nor part of this pass's bundle, so it
	// cannot be created this pass either.
	report, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, []skills.SystemSkillSpec{}, log,
	)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Removed) != 0 {
		t.Fatalf("expected no removal while replacement is missing, got %v", report.Removed)
	}
	if len(report.Blocked) != 1 || !strings.HasSuffix(report.Blocked[0], "kandev-tasks") {
		t.Fatalf("expected kandev-tasks reported as blocked, got %v", report.Blocked)
	}

	if _, err := repo.GetSkillBySlug(context.Background(), "ws-1", "kandev-tasks"); err != nil {
		t.Error("blocked retirement must leave the office_skills row in place")
	}
	agent1 := repo.agents["ws-1"]["agent-1"]
	if got := decodeIDs(t, agent1.SkillIDs); len(got) != 1 || got[0] != oldTasksID {
		t.Errorf("blocked retirement must leave agent skill_ids untouched, got %v", got)
	}
	if got := decodeIDs(t, agent1.DesiredSkills); len(got) != 1 || got[0] != "kandev-tasks" {
		t.Errorf("blocked retirement must leave agent desired_skills untouched, got %v", got)
	}
}

// TestSyncSystemSkills_BundledInsertConflictsWithExistingUserSkill pins
// AC-003.3: a bundled slug not yet backed by a system row, but already
// held by a non-system (user or provider-imported) row, must not be
// inserted over that row. It is skipped and recorded as a conflict so
// the mismatch is visible without silently overwriting user data.
func TestSyncSystemSkills_BundledInsertConflictsWithExistingUserSkill(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	repo.rows["ws-1"] = map[string]*models.Skill{
		"kandev-custom-thing": {
			ID:          "user-skill-1",
			WorkspaceID: "ws-1",
			Slug:        "kandev-custom-thing",
			Name:        "My Custom Thing",
			IsSystem:    false,
			SourceType:  "inline",
		},
	}

	bundled := []skills.SystemSkillSpec{{
		Slug:        "kandev-custom-thing",
		Name:        "Custom Thing",
		Description: "Bundled version",
		Version:     "1.0.0",
		Content:     "---\nname: kandev-custom-thing\ndescription: Bundled version\n---\n# Custom Thing\n",
		ContentHash: "hash-custom-thing",
	}}

	report, err := skills.SyncSystemSkills(context.Background(), repo, []string{"ws-1"}, bundled, log)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Inserted) != 0 {
		t.Fatalf("expected no insert over an existing user skill, got %v", report.Inserted)
	}
	if len(report.Conflicted) != 1 || !strings.HasSuffix(report.Conflicted[0], "kandev-custom-thing") {
		t.Fatalf("expected kandev-custom-thing reported as conflicted, got %v", report.Conflicted)
	}
	got, err := repo.GetSkillBySlug(context.Background(), "ws-1", "kandev-custom-thing")
	if err != nil {
		t.Fatalf("existing user row missing after sync: %v", err)
	}
	if got.IsSystem {
		t.Error("existing user row must not become a system row")
	}
	if got.ID != "user-skill-1" {
		t.Errorf("existing user row ID changed: now %s", got.ID)
	}
}

// TestSyncSystemSkills_NormalizesWellFormedUserSlugToCanonical pins
// AC-003.8: a well-formed but non-canonical user/provider skill slug
// is rewritten to its canonical kandev- prefixed form in place,
// preserving the row ID, and every agent's desired_skills reference
// is rewritten to match.
func TestSyncSystemSkills_NormalizesWellFormedUserSlugToCanonical(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	repo.rows["ws-1"] = map[string]*models.Skill{
		"my-custom-skill": {
			ID:          "user-skill-1",
			WorkspaceID: "ws-1",
			Slug:        "my-custom-skill",
			Name:        "My Custom Skill",
			IsSystem:    false,
			SourceType:  "inline",
		},
	}
	repo.agents["ws-1"] = map[string]*settingsmodels.AgentProfile{
		"agent-1": {
			ID:            "agent-1",
			WorkspaceID:   "ws-1",
			DesiredSkills: mustJSONArray(t, []string{"my-custom-skill"}),
		},
	}

	report, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, []skills.SystemSkillSpec{}, log,
	)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Normalized) != 1 || !strings.HasSuffix(report.Normalized[0], "my-custom-skill->kandev-my-custom-skill") {
		t.Fatalf("expected my-custom-skill normalized, got %v", report.Normalized)
	}

	if _, err := repo.GetSkillBySlug(context.Background(), "ws-1", "my-custom-skill"); err == nil {
		t.Error("old slug should no longer resolve after normalization")
	}
	got, err := repo.GetSkillBySlug(context.Background(), "ws-1", "kandev-my-custom-skill")
	if err != nil {
		t.Fatalf("normalized row missing: %v", err)
	}
	if got.ID != "user-skill-1" {
		t.Errorf("normalization must preserve row ID, got %s", got.ID)
	}

	agent1 := repo.agents["ws-1"]["agent-1"]
	if got := decodeIDs(t, agent1.DesiredSkills); len(got) != 1 || got[0] != "kandev-my-custom-skill" {
		t.Errorf("agent-1.desired_skills = %v, want [kandev-my-custom-skill]", got)
	}
}

// TestSyncSystemSkills_LeavesNotWellFormedUserSlugUntouched pins
// AC-003.9: a legacy row whose slug predates write-time validation
// and is not well-formed (contains characters outside the safe path
// component set) must never be normalized — there is no safe
// canonical rewrite for it, so the sync leaves it exactly as it was
// found.
func TestSyncSystemSkills_LeavesNotWellFormedUserSlugUntouched(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	const badSlug = "legacy slug!"
	repo.rows["ws-1"] = map[string]*models.Skill{
		badSlug: {
			ID:          "user-skill-legacy",
			WorkspaceID: "ws-1",
			Slug:        badSlug,
			Name:        "Legacy",
			IsSystem:    false,
			SourceType:  "inline",
		},
	}

	report, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, []skills.SystemSkillSpec{}, log,
	)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Normalized) != 0 {
		t.Fatalf("not-well-formed slug must never be normalized, got %v", report.Normalized)
	}
	if len(report.Conflicted) != 0 {
		t.Fatalf("not-well-formed slug is not a conflict, got %v", report.Conflicted)
	}
	got, err := repo.GetSkillBySlug(context.Background(), "ws-1", badSlug)
	if err != nil {
		t.Fatalf("legacy row missing after sync: %v", err)
	}
	if got.Slug != badSlug {
		t.Errorf("legacy row slug changed: now %q", got.Slug)
	}
}

// TestSyncSystemSkills_LeavesConflictingNormalizedUserSlugUntouched
// pins the collision branch of AC-003.8: when the canonical form a
// well-formed slug would normalize to is already held by another row,
// neither row is touched and the collision is reported so an operator
// can resolve it manually.
func TestSyncSystemSkills_LeavesConflictingNormalizedUserSlugUntouched(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	repo.rows["ws-1"] = map[string]*models.Skill{
		"my-thing": {
			ID:          "user-skill-a",
			WorkspaceID: "ws-1",
			Slug:        "my-thing",
			Name:        "My Thing (legacy)",
			IsSystem:    false,
			SourceType:  "inline",
		},
		"kandev-my-thing": {
			ID:          "user-skill-b",
			WorkspaceID: "ws-1",
			Slug:        "kandev-my-thing",
			Name:        "My Thing (canonical)",
			IsSystem:    false,
			SourceType:  "inline",
		},
	}

	report, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, []skills.SystemSkillSpec{}, log,
	)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Normalized) != 0 {
		t.Fatalf("colliding slug must not be normalized, got %v", report.Normalized)
	}
	if len(report.Conflicted) != 1 || !strings.HasSuffix(report.Conflicted[0], "my-thing") {
		t.Fatalf("expected my-thing reported as conflicted, got %v", report.Conflicted)
	}

	a, err := repo.GetSkillBySlug(context.Background(), "ws-1", "my-thing")
	if err != nil || a.ID != "user-skill-a" {
		t.Errorf("legacy row must stay at its original slug, got %+v err=%v", a, err)
	}
	b, err := repo.GetSkillBySlug(context.Background(), "ws-1", "kandev-my-thing")
	if err != nil || b.ID != "user-skill-b" {
		t.Errorf("canonical row must be untouched, got %+v err=%v", b, err)
	}
}
