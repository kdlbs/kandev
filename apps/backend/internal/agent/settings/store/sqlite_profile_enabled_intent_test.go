package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// @covers AC-PLATFORM-AGENT-SETTINGS-PARITY-002.2 AC-PLATFORM-AGENT-SETTINGS-PARITY-002.10
func TestProfileEnabledIntentStorage(t *testing.T) {
	for _, initial := range []bool{true, false} {
		for _, mode := range []string{"omitted", "explicit_true", "explicit_false", "legacy"} {
			t.Run(fmt.Sprintf("%s_initial_%t", mode, initial), func(t *testing.T) {
				repo := newFreshRepo(t)
				repo.db.SetMaxOpenConns(1)
				ctx := context.Background()
				id := seedAgentProfile(t, repo, "Original", "intent-agent")
				if _, err := repo.UpdateAgentProfileEnabled(ctx, id, initial); err != nil {
					t.Fatal(err)
				}
				profile, err := repo.GetAgentProfile(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := repo.UpdateAgentProfileEnabled(ctx, id, !initial); err != nil {
					t.Fatal(err)
				}
				profile.Name = "Edited"
				profile.UserModified = true
				wanted := !initial
				var enabled *bool
				if mode == "explicit_true" || mode == "explicit_false" {
					wanted = mode == "explicit_true"
					enabled = &wanted
				}
				if mode == "legacy" {
					wanted = initial
					err = repo.UpdateAgentProfile(ctx, profile)
				} else {
					err = repo.UpdateAgentProfileWithEnabledIntent(ctx, profile, enabled)
				}
				if err != nil {
					t.Fatal(err)
				}
				saved, err := repo.GetAgentProfile(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if saved.Enabled != wanted || profile.Enabled != wanted || saved.Name != "Edited" || saved.Model != "test-model" {
					t.Fatalf("saved=%s/%s/%t returned=%t want enabled=%t", saved.Name, saved.Model, saved.Enabled, profile.Enabled, wanted)
				}
				if !saved.UserModified || !saved.UpdatedAt.Equal(profile.UpdatedAt) {
					t.Fatal("saved mutation metadata does not match returned metadata")
				}
			})
		}
	}
	t.Run("missing_deleted_and_canceled", testEnabledIntentStorageFailures)
	t.Run("statement_failure", testEnabledIntentStatementFailure)
	t.Run("dynamic_rollback", testEnabledIntentDynamicRollback)
	t.Run("legacy_dynamic", testEnabledIntentLegacyDynamic)
}

func testEnabledIntentStorageFailures(t *testing.T) {
	repo := newFreshRepo(t)
	id := seedAgentProfile(t, repo, "Original", "intent-agent")
	ctx := context.Background()
	profile, err := repo.GetAgentProfile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := repo.UpdateAgentProfileWithEnabledIntent(canceled, profile, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled write error=%v", err)
	}
	if err := repo.DeleteAgentProfile(ctx, id); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{id, "missing-profile"} {
		profile.ID = target
		if err := repo.UpdateAgentProfileWithEnabledIntent(ctx, profile, nil); err == nil {
			t.Fatalf("write to missing/deleted profile %s succeeded", target)
		}
	}
}

func testEnabledIntentStatementFailure(t *testing.T) {
	repo := newFreshRepo(t)
	id := seedAgentProfile(t, repo, "Original", "intent-agent")
	ctx := context.Background()
	before, err := repo.GetAgentProfile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`CREATE TRIGGER reject_profile_write BEFORE UPDATE ON agent_profiles
		BEGIN SELECT RAISE(ABORT, 'profile update rejected'); END`); err != nil {
		t.Fatal(err)
	}
	changed := *before
	changed.Name = "Rejected"
	if err := repo.UpdateAgentProfileWithEnabledIntent(ctx, &changed, nil); err == nil {
		t.Fatal("rejected statement reported success")
	}
	after, err := repo.GetAgentProfile(ctx, id)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected statement changed saved profile: error=%v", err)
	}
}

func enabledIntentStoredDynamic(t *testing.T, repo *sqliteRepository) (*models.AgentProfile, *models.DynamicAgentProfile, []models.DynamicAgentRoute) {
	t.Helper()
	repo.db.SetMaxOpenConns(1)
	if _, err := repo.db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	id := seedAgentProfile(t, repo, "Dynamic", "dynamic-agent")
	candidate := seedAgentProfile(t, repo, "Candidate", "candidate-agent")
	profile, err := repo.GetAgentProfile(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	dynamic := &models.DynamicAgentProfile{ProfileID: id}
	routes := []models.DynamicAgentRoute{{DynamicProfileID: id, Position: 0, ExecutionProfileID: candidate, Enabled: true, RulesJSON: `{}`}}
	if err := repo.CreateDynamicAgentProfile(context.Background(), dynamic, routes); err != nil {
		t.Fatal(err)
	}
	return profile, dynamic, routes
}

func testEnabledIntentDynamicRollback(t *testing.T) {
	for _, failure := range []string{"stale_version", "missing_parent", "route_insert"} {
		t.Run(failure, func(t *testing.T) {
			repo := newFreshRepo(t)
			profile, dynamic, routes := enabledIntentStoredDynamic(t, repo)
			ctx := context.Background()
			if failure == "missing_parent" {
				if _, err := repo.db.Exec(`DELETE FROM dynamic_agent_profiles WHERE profile_id = ?`, profile.ID); err != nil {
					t.Fatal(err)
				}
			}
			before, err := repo.GetAgentProfile(ctx, profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			expected := dynamic.Version
			if failure == "stale_version" {
				expected++
			}
			if failure == "route_insert" {
				routes[0].ExecutionProfileID = "missing-candidate"
			}
			profile.Name, profile.Enabled = "Rejected", false
			if err := repo.UpdateAgentProfileWithDynamicEnabledIntent(ctx, profile, dynamic, expected, routes, &profile.Enabled); err == nil {
				t.Fatal("invalid route transaction reported success")
			}
			after, err := repo.GetAgentProfile(ctx, profile.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("failed dynamic write changed base row: error=%v", err)
			}
			if failure != "missing_parent" {
				savedDynamic, savedRoutes, err := repo.GetDynamicAgentProfile(ctx, profile.ID)
				if err != nil || savedDynamic.Version != 1 || len(savedRoutes) != 1 || savedRoutes[0].ExecutionProfileID == "missing-candidate" {
					t.Fatalf("failed dynamic write changed route document: %#v error=%v", savedDynamic, err)
				}
			}
		})
	}
}

func testEnabledIntentLegacyDynamic(t *testing.T) {
	repo := newFreshRepo(t)
	profile, dynamic, routes := enabledIntentStoredDynamic(t, repo)
	ctx := context.Background()
	if _, err := repo.UpdateAgentProfileEnabled(ctx, profile.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateAgentProfileWithDynamic(ctx, profile, dynamic, dynamic.Version, routes); err != nil {
		t.Fatal(err)
	}
	saved, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil || !saved.Enabled || dynamic.Version != 2 {
		t.Fatalf("legacy full dynamic semantics changed: saved=%#v dynamic=%#v error=%v", saved, dynamic, err)
	}
}
