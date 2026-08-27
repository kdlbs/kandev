package config

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// TestApplyImport_CreatesEveryFieldFromBundle imports a bundle with every
// field populated into an empty workspace and asserts each one arrives in the
// DB. A field dropped from applyAgents/applySkills/applyRoutines/applyProjects
// is silent in production — no error, no panic, just missing configuration —
// so this is the test that has to catch it.
func TestApplyImport_CreatesEveryFieldFromBundle(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	result, err := env.svc.ApplyImport(ctx, testWorkspaceID, fullBundle())
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	assertEqual(t, "created count", result.CreatedCount, 4)
	assertEqual(t, "updated count", result.UpdatedCount, 0)

	agent := agentByName(t, env, testWorkspaceID, "ada")
	assertEqual(t, "agent workspace", agent.WorkspaceID, testWorkspaceID)
	assertEqual(t, "agent role", string(agent.Role), "cto")
	assertEqual(t, "agent icon", agent.Icon, "🤖")
	assertEqual(t, "agent status", agent.Status, models.AgentStatusIdle)
	assertEqual(t, "agent budget", agent.BudgetMonthlyCents, 4200)
	assertEqual(t, "agent max sessions", agent.MaxConcurrentSessions, 3)
	assertEqual(t, "agent desired skills", agent.DesiredSkills, `["go","sql"]`)
	assertEqual(t, "agent executor preference", agent.ExecutorPreference, "local_docker")

	skill := skillBySlug(t, env, testWorkspaceID, "code-review")
	assertEqual(t, "skill workspace", skill.WorkspaceID, testWorkspaceID)
	assertEqual(t, "skill name", skill.Name, "Code Review")
	assertEqual(t, "skill description", skill.Description, "reviews diffs")
	assertEqual(t, "skill source type", string(skill.SourceType), "inline")
	assertEqual(t, "skill content", skill.Content, "# Code Review\n\nread the diff\n")

	routine := routineByName(t, env, testWorkspaceID, "standup")
	assertEqual(t, "routine workspace", routine.WorkspaceID, testWorkspaceID)
	assertEqual(t, "routine description", routine.Description, "daily standup")
	assertEqual(t, "routine task template", routine.TaskTemplate, "summarise yesterday")
	assertEqual(t, "routine concurrency", string(routine.ConcurrencyPolicy), "skip")
	assertEqual(t, "routine status", routine.Status, "active")

	project := projectByName(t, env, testWorkspaceID, "apollo")
	assertEqual(t, "project workspace", project.WorkspaceID, testWorkspaceID)
	assertEqual(t, "project description", project.Description, "moon shot")
	assertEqual(t, "project color", project.Color, "#ff00ff")
	assertEqual(t, "project budget", project.BudgetCents, 99000)
	assertEqual(t, "project repositories", project.Repositories, `["kdlbs/kandev"]`)
	assertEqual(t, "project executor config", project.ExecutorConfig, `{"type":"local_docker"}`)
}

// TestApplyImport_NameReferencesAreNotResolved documents the current state of
// the three cross-entity references in the DTO (agent reports_to, routine
// assignee_name, project lead_agent_name): agent reports_to IS resolved to
// the target agent's ID (see the reports_to-specific tests in
// import_reports_to_test.go for the full behavior), but routine assignee_name
// and project lead_agent_name remain export-only fields that import never
// resolves. Wiring resolution in for those must update this test.
func TestApplyImport_NameReferencesAreNotResolved(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Seed the referenced agent so the resolver has something to find.
	lead := seedAgent(t, env, testWorkspaceID, "grace")

	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, fullBundle()); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	agent := agentByName(t, env, testWorkspaceID, "ada")
	assertEqual(t, "agent reports_to", agent.ReportsTo, lead.ID)
	assertEqual(t, "routine assignee",
		routineByName(t, env, testWorkspaceID, "standup").AssigneeAgentProfileID, "")
	assertEqual(t, "project lead",
		projectByName(t, env, testWorkspaceID, "apollo").LeadAgentProfileID, "")
}

// TestApplyImport_ProjectStatusIsForcedActive documents that a created project
// ignores the bundle's status and starts active, and that updating an existing
// project leaves its status untouched.
func TestApplyImport_ProjectStatusIsForcedActive(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// fullBundle carries status "paused".
	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, fullBundle()); err != nil {
		t.Fatalf("ApplyImport create: %v", err)
	}
	assertEqual(t, "created project status",
		projectByName(t, env, testWorkspaceID, "apollo").Status, models.ProjectStatusActive)

	bundle := fullBundle()
	bundle.Projects[0].Status = "archived"
	bundle.Projects[0].Description = "changed"
	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle); err != nil {
		t.Fatalf("ApplyImport update: %v", err)
	}
	updated := projectByName(t, env, testWorkspaceID, "apollo")
	assertEqual(t, "updated project status", updated.Status, models.ProjectStatusActive)
	assertEqual(t, "updated project description", updated.Description, "changed")
}

// TestApplyImport_UpdatesExistingRowsInPlace changes every mutable field and
// asserts the update path carries each one through without creating a second
// row.
func TestApplyImport_UpdatesExistingRowsInPlace(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, fullBundle()); err != nil {
		t.Fatalf("seed import: %v", err)
	}
	agentID := agentByName(t, env, testWorkspaceID, "ada").ID
	skillID := skillBySlug(t, env, testWorkspaceID, "code-review").ID
	routineID := routineByName(t, env, testWorkspaceID, "standup").ID
	projectID := projectByName(t, env, testWorkspaceID, "apollo").ID

	changed := fullBundle()
	changed.Agents[0].Role = "engineer"
	changed.Agents[0].Icon = "🦾"
	changed.Agents[0].BudgetMonthlyCents = 1
	changed.Agents[0].MaxConcurrentSessions = 9
	changed.Agents[0].DesiredSkills = `["rust"]`
	changed.Agents[0].ExecutorPreference = "local_pc"
	changed.Skills[0].Name = "Deep Review"
	changed.Skills[0].Description = "reads everything"
	changed.Skills[0].SourceType = "git"
	changed.Skills[0].Content = "# Deep Review\n"
	changed.Routines[0].Description = "weekly"
	changed.Routines[0].TaskTemplate = "summarise the week"
	changed.Routines[0].ConcurrencyPolicy = "queue"
	changed.Projects[0].Description = "mars shot"
	changed.Projects[0].Color = "#00ff00"
	changed.Projects[0].BudgetCents = 1234
	changed.Projects[0].Repositories = `["kdlbs/other"]`
	changed.Projects[0].ExecutorConfig = `{"type":"local_pc"}`

	result, err := env.svc.ApplyImport(ctx, testWorkspaceID, changed)
	if err != nil {
		t.Fatalf("ApplyImport update: %v", err)
	}
	assertEqual(t, "created count", result.CreatedCount, 0)
	assertEqual(t, "updated count", result.UpdatedCount, 4)

	agent := agentByName(t, env, testWorkspaceID, "ada")
	assertEqual(t, "agent id stable", agent.ID, agentID)
	assertEqual(t, "agent role", string(agent.Role), "engineer")
	assertEqual(t, "agent icon", agent.Icon, "🦾")
	assertEqual(t, "agent budget", agent.BudgetMonthlyCents, 1)
	assertEqual(t, "agent max sessions", agent.MaxConcurrentSessions, 9)
	assertEqual(t, "agent desired skills", agent.DesiredSkills, `["rust"]`)
	assertEqual(t, "agent executor preference", agent.ExecutorPreference, "local_pc")

	skill := skillBySlug(t, env, testWorkspaceID, "code-review")
	assertEqual(t, "skill id stable", skill.ID, skillID)
	assertEqual(t, "skill name", skill.Name, "Deep Review")
	assertEqual(t, "skill description", skill.Description, "reads everything")
	assertEqual(t, "skill source type", string(skill.SourceType), "git")
	assertEqual(t, "skill content", skill.Content, "# Deep Review\n")

	routine := routineByName(t, env, testWorkspaceID, "standup")
	assertEqual(t, "routine id stable", routine.ID, routineID)
	assertEqual(t, "routine description", routine.Description, "weekly")
	assertEqual(t, "routine task template", routine.TaskTemplate, "summarise the week")
	assertEqual(t, "routine concurrency", string(routine.ConcurrencyPolicy), "queue")

	project := projectByName(t, env, testWorkspaceID, "apollo")
	assertEqual(t, "project id stable", project.ID, projectID)
	assertEqual(t, "project description", project.Description, "mars shot")
	assertEqual(t, "project color", project.Color, "#00ff00")
	assertEqual(t, "project budget", project.BudgetCents, 1234)
	assertEqual(t, "project repositories", project.Repositories, `["kdlbs/other"]`)
	assertEqual(t, "project executor config", project.ExecutorConfig, `{"type":"local_pc"}`)
}

// TestApplyImport_IsIdempotent imports the same bundle twice and asserts the
// second pass updates rather than duplicates, leaving exactly one row per
// entity with unchanged identity.
func TestApplyImport_IsIdempotent(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	bundle := fullBundle()

	first, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	agentID := agentByName(t, env, testWorkspaceID, "ada").ID

	second, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	assertEqual(t, "first created", first.CreatedCount, 4)
	assertEqual(t, "second created", second.CreatedCount, 0)
	assertEqual(t, "second updated", second.UpdatedCount, 4)

	agents, err := env.repo.ListAgentInstances(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	assertEqual(t, "agent row count", len(agents), 1)
	assertEqual(t, "agent id unchanged", agents[0].ID, agentID)

	skills, err := env.repo.ListSkills(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	assertEqual(t, "skill row count", len(skills), 1)

	routines, err := env.repo.ListRoutines(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	assertEqual(t, "routine row count", len(routines), 1)

	projects, err := env.repo.ListProjects(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	assertEqual(t, "project row count", len(projects), 1)
}

// TestApplyImport_MatchesExistingByNameNotIdentity proves the dedup key: an
// agent already present under the same name is updated (not duplicated) even
// though the bundle carries no ID, and skills key on slug rather than name.
func TestApplyImport_MatchesExistingByNameNotIdentity(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	existingAgent := seedAgent(t, env, testWorkspaceID, "ada")
	existingSkill := seedSkill(t, env, testWorkspaceID, "code-review")

	bundle := fullBundle()
	// Same slug, different display name: still an update.
	bundle.Skills[0].Name = "Renamed Review"

	result, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle)
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	assertEqual(t, "created count", result.CreatedCount, 2) // routine + project
	assertEqual(t, "updated count", result.UpdatedCount, 2) // agent + skill

	assertEqual(t, "agent id reused", agentByName(t, env, testWorkspaceID, "ada").ID, existingAgent.ID)
	skill := skillBySlug(t, env, testWorkspaceID, "code-review")
	assertEqual(t, "skill id reused", skill.ID, existingSkill.ID)
	assertEqual(t, "skill renamed", skill.Name, "Renamed Review")
}

// TestApplyImport_IgnoresOtherWorkspaces confirms an import scoped to one
// workspace neither reads nor rewrites a same-named row in another.
func TestApplyImport_IgnoresOtherWorkspaces(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	foreign := seedAgent(t, env, "ws-other", "ada")

	result, err := env.svc.ApplyImport(ctx, testWorkspaceID, fullBundle())
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	assertEqual(t, "created count", result.CreatedCount, 4)

	mine := agentByName(t, env, testWorkspaceID, "ada")
	if mine.ID == foreign.ID {
		t.Fatal("import adopted another workspace's agent row")
	}
	other := agentByName(t, env, "ws-other", "ada")
	assertEqual(t, "foreign agent icon untouched", other.Icon, "🛠")
	assertEqual(t, "foreign agent budget untouched", other.BudgetMonthlyCents, 100)
}

func TestApplyImport_EmptyBundleIsANoOp(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedAgent(t, env, testWorkspaceID, "ada")

	result, err := env.svc.ApplyImport(ctx, testWorkspaceID, &ConfigBundle{})
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	assertEqual(t, "created count", result.CreatedCount, 0)
	assertEqual(t, "updated count", result.UpdatedCount, 0)

	agents, err := env.repo.ListAgentInstances(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	assertEqual(t, "existing rows survive", len(agents), 1)
}

// TestApplyImport_LogsActivityWithCounts asserts the audit entry the import
// emits, including the counts embedded in its details string.
func TestApplyImport_LogsActivityWithCounts(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedAgent(t, env, testWorkspaceID, "ada")

	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, fullBundle()); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	if len(env.activity.entries) != 1 {
		t.Fatalf("activity entries: got %d, want 1", len(env.activity.entries))
	}
	got := env.activity.entries[0]
	assertEqual(t, "activity workspace", got.wsID, testWorkspaceID)
	assertEqual(t, "activity actor type", got.actorType, "user")
	assertEqual(t, "activity action", got.action, "config_imported")
	assertEqual(t, "activity target type", got.targetType, "workspace")
	assertEqual(t, "activity target id", got.targetID, testWorkspaceID)
	assertEqual(t, "activity details", got.details, "created=3 updated=1")
}

// TestApplyImport_WrapsRepositoryErrors breaks one entity's table at a time so
// each apply stage fails on its own, and asserts the stage names itself in the
// wrapped error rather than surfacing a bare SQL message.
//
// wantAgentSurvives records the other half of the contract: the import is not
// wrapped in a transaction, so rows written by stages that ran before the
// failure stay in the DB. Adding a transaction here should flip these.
func TestApplyImport_WrapsRepositoryErrors(t *testing.T) {
	tests := []struct {
		name              string
		dropTable         string
		wantPrefix        string
		wantAgentSurvives bool
	}{
		{"agents", "agent_profiles", "apply agents:", false},
		{"skills", "office_skills", "apply skills:", true},
		{"routines", "office_routines", "apply routines:", true},
		{"projects", "office_projects", "apply projects:", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.dropTable(t, tc.dropTable)

			result, err := env.svc.ApplyImport(context.Background(), testWorkspaceID, fullBundle())
			if err == nil {
				t.Fatalf("expected an error after dropping %s, got nil", tc.dropTable)
			}
			if result != nil {
				t.Errorf("result should be nil on error, got %+v", result)
			}
			if !strings.HasPrefix(err.Error(), tc.wantPrefix) {
				t.Errorf("error %q does not start with %q", err.Error(), tc.wantPrefix)
			}
			if len(env.activity.entries) != 0 {
				t.Errorf("failed import should not log activity, got %d entries",
					len(env.activity.entries))
			}
			if !tc.wantAgentSurvives {
				return
			}
			// The agent stage ran to completion before the failure. Its row
			// is committed and is not rolled back.
			agent := agentByName(t, env, testWorkspaceID, "ada")
			assertEqual(t, "partially imported agent survives", agent.Name, "ada")
		})
	}
}

// TestPreviewImport_ClassifiesCreatesAndUpdates checks every entity type is
// split against existing DB state on the right key, and that preview writes
// nothing.
func TestPreviewImport_ClassifiesCreatesAndUpdates(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedAgent(t, env, testWorkspaceID, "ada")
	seedSkill(t, env, testWorkspaceID, "code-review")
	seedRoutine(t, env, testWorkspaceID, "standup")
	seedProject(t, env, testWorkspaceID, "apollo")

	bundle := fullBundle()
	bundle.Agents = append(bundle.Agents, AgentConfig{Name: "grace"})
	bundle.Skills = append(bundle.Skills, SkillConfig{Slug: "triage"})
	bundle.Routines = append(bundle.Routines, RoutineConfig{Name: "retro"})
	bundle.Projects = append(bundle.Projects, ProjectConfig{Name: "gemini"})

	preview, err := env.svc.PreviewImport(ctx, testWorkspaceID, bundle)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}

	assertStrings(t, "agents updated", preview.Agents.Updated, []string{"ada"})
	assertStrings(t, "agents created", preview.Agents.Created, []string{"grace"})
	assertStrings(t, "skills updated", preview.Skills.Updated, []string{"code-review"})
	assertStrings(t, "skills created", preview.Skills.Created, []string{"triage"})
	assertStrings(t, "routines updated", preview.Routines.Updated, []string{"standup"})
	assertStrings(t, "routines created", preview.Routines.Created, []string{"retro"})
	assertStrings(t, "projects updated", preview.Projects.Updated, []string{"apollo"})
	assertStrings(t, "projects created", preview.Projects.Created, []string{"gemini"})

	// PreviewImport never populates deletions — that is IncomingDiff's job.
	assertStrings(t, "agents deleted", preview.Agents.Deleted, nil)
	assertStrings(t, "routines deleted", preview.Routines.Deleted, nil)

	agents, err := env.repo.ListAgentInstances(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	assertEqual(t, "preview wrote no rows", len(agents), 1)
}

func TestPreviewImport_EmptyBundleYieldsEmptyDiffs(t *testing.T) {
	env := newTestEnv(t)
	seedAgent(t, env, testWorkspaceID, "ada")

	preview, err := env.svc.PreviewImport(context.Background(), testWorkspaceID, &ConfigBundle{})
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	assertStrings(t, "agents created", preview.Agents.Created, nil)
	assertStrings(t, "agents updated", preview.Agents.Updated, nil)
	assertStrings(t, "skills created", preview.Skills.Created, nil)
	assertStrings(t, "projects created", preview.Projects.Created, nil)
}

// TestPreviewImport_ScopesToWorkspace confirms an existing row in another
// workspace does not turn a create into an update.
func TestPreviewImport_ScopesToWorkspace(t *testing.T) {
	env := newTestEnv(t)
	seedAgent(t, env, "ws-other", "ada")

	preview, err := env.svc.PreviewImport(context.Background(), testWorkspaceID, fullBundle())
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	assertStrings(t, "agents created", preview.Agents.Created, []string{"ada"})
	assertStrings(t, "agents updated", preview.Agents.Updated, nil)
}

func TestPreviewImport_PropagatesRepositoryErrors(t *testing.T) {
	env := newTestEnv(t)
	env.closeDB(t)

	preview, err := env.svc.PreviewImport(context.Background(), testWorkspaceID, fullBundle())
	if err == nil {
		t.Fatal("expected an error from a closed database, got nil")
	}
	if preview != nil {
		t.Errorf("preview should be nil on error, got %+v", preview)
	}
}
