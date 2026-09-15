package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
)

// webhookAutomationWithSelector builds an Automation whose single webhook
// trigger declares repository.selector_path, plus a matching
// AutomationTriggeredEvent — the shared fixture for every disposition-token
// test below.
func webhookAutomationWithSelector(workspaceID, selectorPath string, repoIDs []string) (*automation.Automation, *automation.AutomationTriggeredEvent) {
	triggerID := "trig-selector"
	cfg, _ := json.Marshal(automation.WebhookTriggerConfig{
		Repository: &automation.WebhookRepositorySelector{SelectorPath: selectorPath},
	})
	a := &automation.Automation{
		WorkspaceID: workspaceID,
		Triggers:    []automation.AutomationTrigger{{ID: triggerID, Type: automation.TriggerTypeWebhook, Config: cfg}},
	}
	a.RepositoryIDs = append(a.RepositoryIDs, repoIDs...)
	evt := &automation.AutomationTriggeredEvent{
		TriggerID:   triggerID,
		TriggerType: automation.TriggerTypeWebhook,
	}
	return a, evt
}

func TestResolveAutomationRepository_SelectorUnresolved(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-sel", []string{"acme/api"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a, evt := webhookAutomationWithSelector("ws-sel", "repo_name", []string{"acme/api"})
	evt.TriggerData = json.RawMessage(`{"other":"x"}`)

	resolved, reason := svc.resolveAutomationRepository(context.Background(), a, evt)
	require.Nil(t, resolved)
	require.Equal(t, "selector_unresolved", reason)
}

func TestResolveAutomationRepository_SelectorNoneConfigured(t *testing.T) {
	repo := setupTestRepo(t)
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "ws-sel-none", Name: "Test", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a, evt := webhookAutomationWithSelector("ws-sel-none", "repo_name", nil)
	evt.TriggerData = json.RawMessage(`{"repo_name":"acme/api"}`)

	resolved, reason := svc.resolveAutomationRepository(context.Background(), a, evt)
	require.Nil(t, resolved)
	require.Equal(t, "repository_none_configured", reason)
}

func TestResolveAutomationRepository_SelectorNoMatch(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-sel-nomatch", []string{"acme/api"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a, evt := webhookAutomationWithSelector("ws-sel-nomatch", "repo_name", []string{"acme/api"})
	evt.TriggerData = json.RawMessage(`{"repo_name":"acme/other"}`)

	resolved, reason := svc.resolveAutomationRepository(context.Background(), a, evt)
	require.Nil(t, resolved)
	require.Equal(t, "selector_no_match: acme/other", reason)
}

func TestResolveAutomationRepository_SelectorAmbiguous(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-sel-ambig", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	for _, id := range []string{"repo-1", "repo-2"} {
		require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "ws-sel-ambig", Name: "acme/api", SourceType: "local",
			LocalPath: "/tmp/" + id, DefaultBranch: "main", CreatedAt: now, UpdatedAt: now,
		}))
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a, evt := webhookAutomationWithSelector("ws-sel-ambig", "repo_name", []string{"repo-1", "repo-2"})
	evt.TriggerData = json.RawMessage(`{"repo_name":"acme/api"}`)

	resolved, reason := svc.resolveAutomationRepository(ctx, a, evt)
	require.Nil(t, resolved)
	require.Equal(t, "selector_ambiguous: acme/api", reason)
}

func TestResolveAutomationRepository_SelectorMatchesExactlyOneBindsIt(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-sel-ok", []string{"acme/api"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a, evt := webhookAutomationWithSelector("ws-sel-ok", "repo_name", []string{"acme/api"})
	evt.TriggerData = json.RawMessage(`{"repo_name":"acme/api"}`)

	resolved, reason := svc.resolveAutomationRepository(context.Background(), a, evt)
	require.Empty(t, reason)
	require.Len(t, resolved, 1)
	require.Equal(t, "acme/api", resolved[0].RepositoryID)
}

// A partial repository load failure that still leaves exactly one match
// binds that match and records nothing — the plan's explicit "out of scope"
// carve-out: a load failure only forces repository_load_failed when it
// leaves the match count uncertain (not exactly one).
func TestResolveAutomationRepository_PartialLoadFailureWithOneMatch_BindsSilently(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-sel-partial", []string{"acme/api"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a, evt := webhookAutomationWithSelector("ws-sel-partial", "repo_name", []string{"acme/api", "repo-does-not-exist"})
	evt.TriggerData = json.RawMessage(`{"repo_name":"acme/api"}`)

	resolved, reason := svc.resolveAutomationRepository(context.Background(), a, evt)
	require.Empty(t, reason)
	require.Len(t, resolved, 1)
	require.Equal(t, "acme/api", resolved[0].RepositoryID)
}

// A partial repository load failure that leaves zero or more than one match
// can't be trusted as a real "no match"/"ambiguous" outcome — the missing
// repository might have been the actual match — so it must record
// repository_load_failed instead of a misleading selector_no_match.
func TestResolveAutomationRepository_PartialLoadFailureWithoutOneMatch_RecordsLoadFailed(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-sel-partial2", []string{"acme/api"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a, evt := webhookAutomationWithSelector("ws-sel-partial2", "repo_name", []string{"acme/api", "repo-does-not-exist"})
	evt.TriggerData = json.RawMessage(`{"repo_name":"acme/unrelated"}`)

	resolved, reason := svc.resolveAutomationRepository(context.Background(), a, evt)
	require.Nil(t, resolved)
	require.Equal(t, "repository_load_failed", reason)
}

// A store unavailable for GetRepository can never certify a match — always
// repository_load_failed regardless of selector value.
func TestResolveAutomationRepository_StoreUnavailable_RecordsLoadFailed(t *testing.T) {
	svc := &Service{logger: testLogger(), repo: nil}
	a, evt := webhookAutomationWithSelector("ws-sel-nostore", "repo_name", []string{"acme/api"})
	evt.TriggerData = json.RawMessage(`{"repo_name":"acme/api"}`)

	resolved, reason := svc.resolveAutomationRepository(context.Background(), a, evt)
	require.Nil(t, resolved)
	require.Equal(t, "repository_load_failed", reason)
}

// D5: a selector value shaped like "owner/name" that simply doesn't match
// any configured repository's Name is a plain no-match — the webhook path
// never parses owner/name out of the value or falls back to any other
// resolution rule.
func TestResolveAutomationRepository_OwnerNameShapedValueNoMatch_NoParsing(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-sel-shape", []string{"acme/api"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a, evt := webhookAutomationWithSelector("ws-sel-shape", "repo_name", []string{"acme/api"})
	// "other/api" shares the "/api" suffix with the configured repo's Name but
	// must not fuzzy- or component-match it.
	evt.TriggerData = json.RawMessage(`{"repo_name":"other/api"}`)

	resolved, reason := svc.resolveAutomationRepository(context.Background(), a, evt)
	require.Nil(t, resolved)
	require.Equal(t, "selector_no_match: other/api", reason)
}

// No selector declared at all preserves today's behavior: whatever
// resolveExplicitRepositories resolves is returned with no reason.
func TestResolveAutomationRepository_NoSelectorDeclared_PreservesLegacyBehavior(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-no-sel", []string{"repo-a"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a := &automation.Automation{WorkspaceID: "ws-no-sel", RepositoryIDs: []string{"repo-a"}}
	evt := &automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeWebhook, TriggerData: json.RawMessage(`{}`)}

	resolved, reason := svc.resolveAutomationRepository(context.Background(), a, evt)
	require.Empty(t, reason)
	require.Len(t, resolved, 1)
}
