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
// concrete REQ-003 migration case this feature adds: once the bundled
// `memory` skill is renamed to `kandev-memory` for naming-scheme
// consistency (pending, tracked separately), a workspace with the old
// row must retire it and end up with the new canonical slug in its
// place. This test injects a synthetic bundled spec to exercise that
// path ahead of the real rename.
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

// TestSyncSystemSkills_RetirementBlockedWhenReplacementIsNotBundled ensures
// an old system row is not retired merely because a replacement row happens
// to exist in the database. The replacement must be part of the current
// bundle; otherwise this pass can delete both rows and strand references.
func TestSyncSystemSkills_RetirementBlockedWhenReplacementIsNotBundled(t *testing.T) {
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
		"kandev-task-ops": {
			ID:          "skill-task-ops",
			WorkspaceID: "ws-1",
			Slug:        "kandev-task-ops",
			Name:        "Task operations",
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

	// Neither existing row is present in this pass's bundle. The configured
	// replacement is therefore not a valid destination for retirement.
	report, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, []skills.SystemSkillSpec{}, log,
	)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Blocked) != 1 || !strings.HasSuffix(report.Blocked[0], "kandev-tasks") {
		t.Fatalf("expected kandev-tasks reported as blocked, got %v", report.Blocked)
	}
	if _, err := repo.GetSkillBySlug(context.Background(), "ws-1", "kandev-tasks"); err != nil {
		t.Fatalf("blocked retired row must remain: %v", err)
	}
	if _, err := repo.GetSkillBySlug(context.Background(), "ws-1", "kandev-task-ops"); err == nil {
		t.Fatal("unbundled replacement row should be retired independently")
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

// TestSyncSystemSkills_NormalizationRetriesAfterFailedReferenceRewrite
// pins the recovery contract in
// docs/specs/agents/system-design/injected-skill-naming-migration.md
// ("## Failure and recovery"): a normalization interrupted between the
// row rename and the agent-reference rewrite must be fully retried on
// the next pass, not silently left half-done. If the row were renamed
// to its canonical slug before the agent-reference rewrite is
// confirmed, a later pass would see the row as already canonical and
// skip it forever, stranding the stale agent reference.
func TestSyncSystemSkills_NormalizationRetriesAfterFailedReferenceRewrite(t *testing.T) {
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
	repo.failUpdateAgentFor["agent-1"] = true

	// SyncSystemSkills logs and continues past a per-workspace failure
	// (so one bad workspace doesn't block the rest) rather than
	// returning it, so the top-level call succeeds; the failure is
	// checked via the row/agent state left behind instead.
	report, err := skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, []skills.SystemSkillSpec{}, log,
	)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(report.Normalized) != 0 {
		t.Fatalf("normalization must not be reported while the reference rewrite failed, got %v", report.Normalized)
	}

	// The row must NOT have been committed to its canonical slug: a
	// half-done normalization (row renamed, reference not rewritten)
	// is exactly the state the recovery contract forbids, because the
	// canonical-slug gate would then skip this row on every later pass.
	if _, err := repo.GetSkillBySlug(context.Background(), "ws-1", "my-custom-skill"); err != nil {
		t.Fatalf("row must stay at its original slug after a failed rewrite, got err=%v", err)
	}
	if _, err := repo.GetSkillBySlug(context.Background(), "ws-1", "kandev-my-custom-skill"); err == nil {
		t.Fatal("row must not be committed to its canonical slug before the reference rewrite succeeds")
	}
	agent1 := repo.agents["ws-1"]["agent-1"]
	if got := decodeIDs(t, agent1.DesiredSkills); len(got) != 1 || got[0] != "my-custom-skill" {
		t.Errorf("agent-1.desired_skills must stay untouched after a failed rewrite, got %v", got)
	}

	// A later pass, with the transient failure gone, must fully
	// converge: this is the "next pass repeats the reference rewrite"
	// half of the recovery contract.
	repo.failUpdateAgentFor["agent-1"] = false
	report, err = skills.SyncSystemSkills(
		context.Background(), repo, []string{"ws-1"}, []skills.SystemSkillSpec{}, log,
	)
	if err != nil {
		t.Fatalf("retry sync: %v", err)
	}
	if len(report.Normalized) != 1 || !strings.HasSuffix(report.Normalized[0], "my-custom-skill->kandev-my-custom-skill") {
		t.Fatalf("expected my-custom-skill normalized on retry, got %v", report.Normalized)
	}
	if _, err := repo.GetSkillBySlug(context.Background(), "ws-1", "kandev-my-custom-skill"); err != nil {
		t.Fatalf("row must be canonical after the retry succeeds: %v", err)
	}
	agent1 = repo.agents["ws-1"]["agent-1"]
	if got := decodeIDs(t, agent1.DesiredSkills); len(got) != 1 || got[0] != "kandev-my-custom-skill" {
		t.Errorf("agent-1.desired_skills = %v, want [kandev-my-custom-skill] after retry", got)
	}
}

// TestSyncSystemSkills_RetiredSystemSlugFreedForSamePassNormalization
// pins a sequencing edge in syncWorkspace: retireOrphanedSystemSkills
// runs before normalizeUserSkillSlugs, and a system row it retires in
// this same pass must not block a user skill from normalizing onto
// that now-freed slug. Treating the just-deleted row as still "taken"
// would report a false conflict for a slug that is actually free.
func TestSyncSystemSkills_RetiredSystemSlugFreedForSamePassNormalization(t *testing.T) {
	repo := newStubSyncRepo()
	log := logger.Default()

	repo.rows["ws-1"] = map[string]*models.Skill{
		// A stale system row for a slug no longer in the bundle, with
		// no configured replacement, so it retires outright this pass.
		"kandev-legacy-widget": {
			ID:          "system-skill-legacy",
			WorkspaceID: "ws-1",
			Slug:        "kandev-legacy-widget",
			Name:        "Legacy Widget",
			IsSystem:    true,
			SourceType:  skills.SourceTypeSystem,
		},
		// A user skill whose canonical target is exactly the slug the
		// system row above is retiring in this same pass.
		"legacy-widget": {
			ID:          "user-skill-1",
			WorkspaceID: "ws-1",
			Slug:        "legacy-widget",
			Name:        "My Legacy Widget",
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
	if len(report.Removed) != 1 || !strings.HasSuffix(report.Removed[0], "kandev-legacy-widget") {
		t.Fatalf("expected kandev-legacy-widget retired, got removed=%v", report.Removed)
	}
	if len(report.Conflicted) != 0 {
		t.Fatalf("slug freed by same-pass retirement must not be reported as conflicted, got %v", report.Conflicted)
	}
	if len(report.Normalized) != 1 || !strings.HasSuffix(report.Normalized[0], "legacy-widget->kandev-legacy-widget") {
		t.Fatalf("expected legacy-widget normalized onto the freed slug, got %v", report.Normalized)
	}
	got, err := repo.GetSkillBySlug(context.Background(), "ws-1", "kandev-legacy-widget")
	if err != nil {
		t.Fatalf("normalized row missing: %v", err)
	}
	if got.ID != "user-skill-1" {
		t.Errorf("normalization must preserve the user row's ID, got %s", got.ID)
	}
	if got.IsSystem {
		t.Error("normalized user row must not become a system row")
	}
}

// TestSyncSystemSkills_RenameRewritesLegacyCommaSeparatedDesiredSkills
// pins a format gap in the agent-reference rewrite: desired_skills
// predates the JSON-array persistence format on some rows and is
// still a legacy comma-separated string (ParseDesiredSlugs reads
// both; see injection.go). A slug rename must rewrite that legacy
// value too — otherwise the agent is left holding a reference to a
// slug that no longer exists once the row is renamed to its
// canonical form.
func TestSyncSystemSkills_RenameRewritesLegacyCommaSeparatedDesiredSkills(t *testing.T) {
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
			DesiredSkills: "my-custom-skill,other-skill",
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

	agent1 := repo.agents["ws-1"]["agent-1"]
	got := decodeIDs(t, agent1.DesiredSkills)
	want := []string{"kandev-my-custom-skill", "other-skill"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("agent-1.desired_skills = %q (parsed %v), want %v", agent1.DesiredSkills, got, want)
	}
}
