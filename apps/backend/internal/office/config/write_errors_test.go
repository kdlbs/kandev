package config

import (
	"context"
	"strings"
	"testing"
)

// TestApplyImport_PropagatesCreateErrors breaks the write handle after the
// tables are readable, so every List succeeds and every Create fails. Each
// entity's create branch must surface the error instead of counting a row it
// never wrote.
func TestApplyImport_PropagatesCreateErrors(t *testing.T) {
	tests := []struct {
		name       string
		bundle     *ConfigBundle
		wantPrefix string
	}{
		{"agents", &ConfigBundle{Agents: []AgentConfig{{Name: "ada"}}}, "apply agents:"},
		{"skills", &ConfigBundle{Skills: []SkillConfig{{Slug: "code-review"}}}, "apply skills:"},
		{"routines", &ConfigBundle{Routines: []RoutineConfig{{Name: "standup"}}}, "apply routines:"},
		{"projects", &ConfigBundle{Projects: []ProjectConfig{{Name: "apollo"}}}, "apply projects:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newSplitTestEnv(t)
			env.breakWrites(t)

			result, err := env.svc.ApplyImport(context.Background(), testWorkspaceID, tc.bundle)
			if err == nil {
				t.Fatal("expected a write error, got nil")
			}
			if result != nil {
				t.Errorf("result should be nil on error, got %+v", result)
			}
			if !strings.HasPrefix(err.Error(), tc.wantPrefix) {
				t.Errorf("error %q does not start with %q", err.Error(), tc.wantPrefix)
			}
		})
	}
}

// TestApplyImport_PropagatesUpdateErrors is the same guard for the update
// branch: the rows already exist, so each stage takes the update path.
func TestApplyImport_PropagatesUpdateErrors(t *testing.T) {
	tests := []struct {
		name       string
		bundle     *ConfigBundle
		wantPrefix string
	}{
		{"agents", &ConfigBundle{Agents: []AgentConfig{{Name: "ada"}}}, "apply agents:"},
		{"skills", &ConfigBundle{Skills: []SkillConfig{{Slug: "code-review"}}}, "apply skills:"},
		{"routines", &ConfigBundle{Routines: []RoutineConfig{{Name: "standup"}}}, "apply routines:"},
		{"projects", &ConfigBundle{Projects: []ProjectConfig{{Name: "apollo"}}}, "apply projects:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newSplitTestEnv(t)
			seedAgent(t, env, testWorkspaceID, "ada")
			seedSkill(t, env, testWorkspaceID, "code-review")
			seedRoutine(t, env, testWorkspaceID, "standup")
			seedProject(t, env, testWorkspaceID, "apollo")
			env.breakWrites(t)

			_, err := env.svc.ApplyImport(context.Background(), testWorkspaceID, tc.bundle)
			if err == nil {
				t.Fatal("expected a write error, got nil")
			}
			if !strings.HasPrefix(err.Error(), tc.wantPrefix) {
				t.Errorf("error %q does not start with %q", err.Error(), tc.wantPrefix)
			}
		})
	}
}

// TestDeleteRowsMissingFromBundle_PropagatesDeleteErrors covers the prune
// path's write failures, one entity at a time.
//
// deleteRowsMissingFromBundle returns repository errors unwrapped, so the
// error text cannot say which stage failed. The stage is isolated by the
// fixture instead: each subtest seeds only its own entity, so every earlier
// stage finds nothing to delete and returns before attempting a write. The
// surviving-row assertion then confirms the delete was both attempted and
// refused, rather than skipped.
func TestDeleteRowsMissingFromBundle_PropagatesDeleteErrors(t *testing.T) {
	tests := []struct {
		name    string
		seed    func(t *testing.T, env *testEnv)
		remains func(t *testing.T, env *testEnv)
	}{
		{
			"agents",
			func(t *testing.T, env *testEnv) { seedAgent(t, env, testWorkspaceID, "gone") },
			func(t *testing.T, env *testEnv) {
				assertRemainingNames(t, env, testWorkspaceID, []string{"gone"}, nil, nil, nil)
			},
		},
		{
			"skills",
			func(t *testing.T, env *testEnv) { seedSkill(t, env, testWorkspaceID, "gone") },
			func(t *testing.T, env *testEnv) {
				assertRemainingNames(t, env, testWorkspaceID, nil, []string{"gone"}, nil, nil)
			},
		},
		{
			"routines",
			func(t *testing.T, env *testEnv) { seedRoutine(t, env, testWorkspaceID, "gone") },
			func(t *testing.T, env *testEnv) {
				assertRemainingNames(t, env, testWorkspaceID, nil, nil, []string{"gone"}, nil)
			},
		},
		{
			"projects",
			func(t *testing.T, env *testEnv) { seedProject(t, env, testWorkspaceID, "gone") },
			func(t *testing.T, env *testEnv) {
				assertRemainingNames(t, env, testWorkspaceID, nil, nil, nil, []string{"gone"})
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newSplitTestEnv(t)
			tc.seed(t, env)
			env.breakWrites(t)

			err := env.svc.deleteRowsMissingFromBundle(
				context.Background(), testWorkspaceID, &ConfigBundle{})
			if err == nil {
				t.Fatal("expected a delete error, got nil")
			}
			tc.remains(t, env)
		})
	}
}

// TestApplyIncoming_PropagatesPruneError proves a failed prune fails the whole
// sync rather than returning a success result for a half-applied state.
func TestApplyIncoming_PropagatesPruneError(t *testing.T) {
	env := newSplitTestEnv(t)
	seedAgent(t, env, testWorkspaceID, "gone")
	env.breakWrites(t)

	result, err := env.svc.ApplyIncoming(context.Background(), testWorkspaceID)
	if err == nil {
		t.Fatal("expected a prune error, got nil")
	}
	if result != nil {
		t.Errorf("result should be nil on error, got %+v", result)
	}
}

// TestPreviewImport_PropagatesPerEntityErrors drops one table at a time so
// each preview stage fails on its own.
func TestPreviewImport_PropagatesPerEntityErrors(t *testing.T) {
	for _, table := range []string{
		"agent_profiles", "office_skills", "office_routines", "office_projects",
	} {
		t.Run(table, func(t *testing.T) {
			env := newTestEnv(t)
			env.dropTable(t, table)

			preview, err := env.svc.PreviewImport(
				context.Background(), testWorkspaceID, fullBundle())
			if err == nil {
				t.Fatalf("expected an error after dropping %s, got nil", table)
			}
			if preview != nil {
				t.Errorf("preview should be nil on error, got %+v", preview)
			}
		})
	}
}
