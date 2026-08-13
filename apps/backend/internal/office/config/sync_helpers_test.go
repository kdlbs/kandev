package config

import (
	"context"
	"sort"
	"testing"
)

// TestDiffBundles_ClassifiesEveryEntityType feeds an incoming bundle that
// creates, updates and deletes one of each entity type, and asserts each
// bucket. Inverting any of the four membership checks flips a create into an
// update here.
func TestDiffBundles_ClassifiesEveryEntityType(t *testing.T) {
	existing := &ConfigBundle{
		Agents:   []AgentConfig{{Name: "ada"}, {Name: "gone-agent"}},
		Skills:   []SkillConfig{{Slug: "code-review"}, {Slug: "gone-skill"}},
		Routines: []RoutineConfig{{Name: "standup"}, {Name: "gone-routine"}},
		Projects: []ProjectConfig{{Name: "apollo"}, {Name: "gone-project"}},
	}
	incoming := &ConfigBundle{
		Agents:   []AgentConfig{{Name: "ada"}, {Name: "new-agent"}},
		Skills:   []SkillConfig{{Slug: "code-review"}, {Slug: "new-skill"}},
		Routines: []RoutineConfig{{Name: "standup"}, {Name: "new-routine"}},
		Projects: []ProjectConfig{{Name: "apollo"}, {Name: "new-project"}},
	}

	preview := diffBundles(incoming, existing)

	assertStrings(t, "agents updated", preview.Agents.Updated, []string{"ada"})
	assertStrings(t, "agents created", preview.Agents.Created, []string{"new-agent"})
	assertStrings(t, "agents deleted", preview.Agents.Deleted, []string{"gone-agent"})

	assertStrings(t, "skills updated", preview.Skills.Updated, []string{"code-review"})
	assertStrings(t, "skills created", preview.Skills.Created, []string{"new-skill"})
	assertStrings(t, "skills deleted", preview.Skills.Deleted, []string{"gone-skill"})

	assertStrings(t, "routines updated", preview.Routines.Updated, []string{"standup"})
	assertStrings(t, "routines created", preview.Routines.Created, []string{"new-routine"})
	assertStrings(t, "routines deleted", preview.Routines.Deleted, []string{"gone-routine"})

	assertStrings(t, "projects updated", preview.Projects.Updated, []string{"apollo"})
	assertStrings(t, "projects created", preview.Projects.Created, []string{"new-project"})
	assertStrings(t, "projects deleted", preview.Projects.Deleted, []string{"gone-project"})
}

// TestDiffBundles_SkillsKeyOnSlug guards the one entity that does not key on
// Name: a skill whose slug matches but whose name changed is an update, and a
// skill whose name matches but whose slug changed is a create plus a delete.
func TestDiffBundles_SkillsKeyOnSlug(t *testing.T) {
	existing := &ConfigBundle{Skills: []SkillConfig{{Name: "Code Review", Slug: "code-review"}}}

	renamed := diffBundles(
		&ConfigBundle{Skills: []SkillConfig{{Name: "Deep Review", Slug: "code-review"}}}, existing)
	assertStrings(t, "renamed updated", renamed.Skills.Updated, []string{"code-review"})
	assertStrings(t, "renamed deleted", renamed.Skills.Deleted, nil)

	reslugged := diffBundles(
		&ConfigBundle{Skills: []SkillConfig{{Name: "Code Review", Slug: "deep-review"}}}, existing)
	assertStrings(t, "reslugged created", reslugged.Skills.Created, []string{"deep-review"})
	assertStrings(t, "reslugged deleted", reslugged.Skills.Deleted, []string{"code-review"})
}

func TestDiffBundles_EmptySides(t *testing.T) {
	full := fullBundle()

	fromEmpty := diffBundles(full, &ConfigBundle{})
	assertStrings(t, "everything created", fromEmpty.Agents.Created, []string{"ada"})
	assertStrings(t, "nothing deleted", fromEmpty.Agents.Deleted, nil)

	toEmpty := diffBundles(&ConfigBundle{}, full)
	assertStrings(t, "nothing created", toEmpty.Agents.Created, nil)
	assertStrings(t, "everything deleted", toEmpty.Agents.Deleted, []string{"ada"})
	assertStrings(t, "skills deleted", toEmpty.Skills.Deleted, []string{"code-review"})
	assertStrings(t, "routines deleted", toEmpty.Routines.Deleted, []string{"standup"})
	assertStrings(t, "projects deleted", toEmpty.Projects.Deleted, []string{"apollo"})

	both := diffBundles(&ConfigBundle{}, &ConfigBundle{})
	assertStrings(t, "empty created", both.Agents.Created, nil)
	assertStrings(t, "empty updated", both.Agents.Updated, nil)
	assertStrings(t, "empty deleted", both.Agents.Deleted, nil)
}

// TestAppendDeletions_ReportsRowsMissingFromBundle covers all four entity
// types and confirms the created/updated buckets are left alone.
func TestAppendDeletions_ReportsRowsMissingFromBundle(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedAgent(t, env, testWorkspaceID, "ada")
	seedAgent(t, env, testWorkspaceID, "gone-agent")
	seedSkill(t, env, testWorkspaceID, "code-review")
	seedSkill(t, env, testWorkspaceID, "gone-skill")
	seedRoutine(t, env, testWorkspaceID, "standup")
	seedRoutine(t, env, testWorkspaceID, "gone-routine")
	seedProject(t, env, testWorkspaceID, "apollo")
	seedProject(t, env, testWorkspaceID, "gone-project")

	preview := &ImportPreview{}
	preview.Agents.Created = []string{"pre-existing"}

	if err := env.svc.appendDeletions(ctx, testWorkspaceID, fullBundle(), preview); err != nil {
		t.Fatalf("appendDeletions: %v", err)
	}

	assertStrings(t, "agents deleted", preview.Agents.Deleted, []string{"gone-agent"})
	assertStrings(t, "skills deleted", preview.Skills.Deleted, []string{"gone-skill"})
	assertStrings(t, "routines deleted", preview.Routines.Deleted, []string{"gone-routine"})
	assertStrings(t, "projects deleted", preview.Projects.Deleted, []string{"gone-project"})
	assertStrings(t, "created bucket untouched", preview.Agents.Created, []string{"pre-existing"})
}

// TestAppendDeletions_IgnoresOtherWorkspaces makes sure a row that only exists
// in another workspace is never proposed for deletion.
func TestAppendDeletions_IgnoresOtherWorkspaces(t *testing.T) {
	env := newTestEnv(t)
	seedAgent(t, env, "ws-other", "foreign")

	preview := &ImportPreview{}
	if err := env.svc.appendDeletions(
		context.Background(), testWorkspaceID, fullBundle(), preview); err != nil {
		t.Fatalf("appendDeletions: %v", err)
	}
	assertStrings(t, "agents deleted", preview.Agents.Deleted, nil)
}

func TestAppendDeletions_PropagatesRepositoryErrors(t *testing.T) {
	for _, table := range []string{
		"agent_profiles", "office_skills", "office_routines", "office_projects",
	} {
		t.Run(table, func(t *testing.T) {
			env := newTestEnv(t)
			env.dropTable(t, table)

			err := env.svc.appendDeletions(
				context.Background(), testWorkspaceID, fullBundle(), &ImportPreview{})
			if err == nil {
				t.Fatalf("expected an error after dropping %s, got nil", table)
			}
		})
	}
}

// TestDeleteRowsMissingFromBundle_RemovesOnlyAbsentRows seeds two rows per
// entity type, keeps one of each in the bundle, and asserts exactly the other
// is gone.
func TestDeleteRowsMissingFromBundle_RemovesOnlyAbsentRows(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedAgent(t, env, testWorkspaceID, "ada")
	seedAgent(t, env, testWorkspaceID, "gone-agent")
	seedSkill(t, env, testWorkspaceID, "code-review")
	seedSkill(t, env, testWorkspaceID, "gone-skill")
	seedRoutine(t, env, testWorkspaceID, "standup")
	seedRoutine(t, env, testWorkspaceID, "gone-routine")
	seedProject(t, env, testWorkspaceID, "apollo")
	seedProject(t, env, testWorkspaceID, "gone-project")

	if err := env.svc.deleteRowsMissingFromBundle(ctx, testWorkspaceID, fullBundle()); err != nil {
		t.Fatalf("deleteRowsMissingFromBundle: %v", err)
	}

	assertRemainingNames(t, env, testWorkspaceID, []string{"ada"}, []string{"code-review"},
		[]string{"standup"}, []string{"apollo"})
}

// TestDeleteRowsMissingFromBundle_EmptyBundleClearsWorkspace is the
// destructive end of the range: nothing on disk means nothing in the DB.
func TestDeleteRowsMissingFromBundle_EmptyBundleClearsWorkspace(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedAgent(t, env, testWorkspaceID, "ada")
	seedSkill(t, env, testWorkspaceID, "code-review")
	seedRoutine(t, env, testWorkspaceID, "standup")
	seedProject(t, env, testWorkspaceID, "apollo")

	if err := env.svc.deleteRowsMissingFromBundle(ctx, testWorkspaceID, &ConfigBundle{}); err != nil {
		t.Fatalf("deleteRowsMissingFromBundle: %v", err)
	}

	assertRemainingNames(t, env, testWorkspaceID, nil, nil, nil, nil)
}

// TestDeleteRowsMissingFromBundle_LeavesOtherWorkspacesAlone is the guard that
// keeps an FS sync of one workspace from wiping another.
func TestDeleteRowsMissingFromBundle_LeavesOtherWorkspacesAlone(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedAgent(t, env, "ws-other", "foreign-agent")
	seedSkill(t, env, "ws-other", "foreign-skill")
	seedRoutine(t, env, "ws-other", "foreign-routine")
	seedProject(t, env, "ws-other", "foreign-project")

	if err := env.svc.deleteRowsMissingFromBundle(ctx, testWorkspaceID, &ConfigBundle{}); err != nil {
		t.Fatalf("deleteRowsMissingFromBundle: %v", err)
	}

	assertRemainingNames(t, env, "ws-other", []string{"foreign-agent"}, []string{"foreign-skill"},
		[]string{"foreign-routine"}, []string{"foreign-project"})
}

func TestDeleteRowsMissingFromBundle_PropagatesRepositoryErrors(t *testing.T) {
	for _, table := range []string{
		"agent_profiles", "office_skills", "office_routines", "office_projects",
	} {
		t.Run(table, func(t *testing.T) {
			env := newTestEnv(t)
			env.dropTable(t, table)

			err := env.svc.deleteRowsMissingFromBundle(
				context.Background(), testWorkspaceID, fullBundle())
			if err == nil {
				t.Fatalf("expected an error after dropping %s, got nil", table)
			}
		})
	}
}

// assertRemainingNames compares the workspace's surviving rows, sorted, with
// the expected names for each entity type.
func assertRemainingNames(
	t *testing.T, env *testEnv, wsID string, agents, skills, routines, projects []string,
) {
	t.Helper()
	ctx := context.Background()

	gotAgents, err := env.repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	assertSorted(t, "agents", agentNames(gotAgents), agents)

	gotSkills, err := env.repo.ListSkills(ctx, wsID)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	assertSorted(t, "skills", skillSlugs(gotSkills), skills)

	gotRoutines, err := env.repo.ListRoutines(ctx, wsID)
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	assertSorted(t, "routines", routineNames(gotRoutines), routines)

	gotProjects, err := env.repo.ListProjects(ctx, wsID)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	assertSorted(t, "projects", projectNames(gotProjects), projects)
}

func assertSorted(t *testing.T, label string, got, want []string) {
	t.Helper()
	sort.Strings(got)
	if want == nil {
		want = []string{}
	}
	assertStrings(t, label, got, want)
}
