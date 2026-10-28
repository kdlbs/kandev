package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	testInventoryCommit = "0123456789abcdef0123456789abcdef01234567"
	testInventoryRoot   = "/srv/workspace"
)

func providerPreparedProvider() models.ExecutorProvider {
	provider := capabilityLaunchProvider()
	provider.RequiredCapabilities = append(provider.RequiredCapabilities,
		models.ExecutorProviderCapabilityRequirement{Name: "repository_intent", Version: 1})
	provider.WorkspacePreparation = "provider"
	return provider
}

func repositoryState(id, path, branch string) *pluginsdk.ExecutorRepositoryState {
	return &pluginsdk.ExecutorRepositoryState{Id: id, Path: path, Branch: branch, HeadCommit: testInventoryCommit}
}

func preparedResource(states ...*pluginsdk.ExecutorRepositoryState) *pluginsdk.ExecutorResourceDescriptor {
	resource := workspaceResource(testInventoryRoot, "worker-7")
	resource.Repositories = states
	return resource
}

func preparedHarness(t *testing.T, resource *pluginsdk.ExecutorResourceDescriptor, ids ...string) *workspaceLaunchHarness {
	t.Helper()
	harness := newWorkspaceLaunchHarness(t, providerPreparedProvider(), resource)
	for _, id := range ids {
		harness.request.PluginExecutor.Repositories = append(harness.request.PluginExecutor.Repositories,
			PluginRepositorySource{RepositoryID: id, Locator: "https://example.test/" + id + ".git", Branch: "main"})
	}
	return harness
}

func TestProviderPreparedLaunchSendsIntentAndRecordsTheVerifiedInventory(t *testing.T) {
	harness := preparedHarness(t, preparedResource(
		repositoryState("a", testInventoryRoot+"/a", "main"), repositoryState("b", "b", "main")), "b", "a")

	if _, err := harness.runtime.CreateInstance(context.Background(), harness.request); err != nil {
		t.Fatalf("CreateInstance(): %v", err)
	}
	sent := harness.operations.provisionRequest.GetRepositories()
	if len(sent) != 2 || sent[0].GetId() != "a" || sent[1].GetId() != "b" {
		t.Fatalf("intent = %v, want a then b", sent)
	}
	ready := harness.checkpoints[len(harness.checkpoints)-1]
	if ready.WorkspacePreparation != "provider" || len(ready.Repositories) != 2 || ready.Repositories[0].ID != "a" {
		t.Fatalf("recorded preparation = %q repositories = %#v", ready.WorkspacePreparation, ready.Repositories)
	}
}

func TestProviderPreparedInventoryMismatchRunsCleanupWithItsReason(t *testing.T) {
	inside := testInventoryRoot + "/a"
	cases := map[string]*pluginsdk.ExecutorResourceDescriptor{
		"missing":          preparedResource(repositoryState("a", inside, "main")),
		"extra":            preparedResource(repositoryState("a", inside, "main"), repositoryState("b", testInventoryRoot+"/b", "main"), repositoryState("c", testInventoryRoot+"/c", "main")),
		"duplicate":        preparedResource(repositoryState("a", inside, "main"), repositoryState("a", inside, "main")),
		"outside":          preparedResource(repositoryState("a", inside, "main"), repositoryState("b", "/elsewhere/b", "main")),
		"traversal":        preparedResource(repositoryState("a", inside, "main"), repositoryState("b", "../b", "main")),
		"short commit":     preparedResource(repositoryState("a", inside, "main"), &pluginsdk.ExecutorRepositoryState{Id: "b", Path: "b", Branch: "main", HeadCommit: "abc123"}),
		"different branch": preparedResource(repositoryState("a", inside, "main"), repositoryState("b", "b", "other")),
	}
	for name, resource := range cases {
		t.Run(name, func(t *testing.T) {
			harness := preparedHarness(t, resource, "a", "b")
			if _, err := harness.runtime.CreateInstance(context.Background(), harness.request); err == nil {
				t.Fatal("CreateInstance() unexpectedly succeeded")
			}
			destroy := harness.operations.destroyRequest
			if destroy == nil || destroy.GetCleanupReason() != "repository_inventory_mismatch" {
				t.Fatalf("destroy request = %v, want cleanup reason repository_inventory_mismatch", destroy)
			}
			if harness.instance.ID != "" {
				t.Fatal("no agentctl instance may be created from a mismatched inventory")
			}
		})
	}
}

func TestEmptyRepositoryListIsAValidProviderPreparedLaunch(t *testing.T) {
	harness := preparedHarness(t, preparedResource())
	if _, err := harness.runtime.CreateInstance(context.Background(), harness.request); err != nil {
		t.Fatalf("CreateInstance(): %v", err)
	}
	if got := harness.operations.provisionRequest.GetRepositories(); len(got) != 0 {
		t.Fatalf("intent = %v, want empty", got)
	}
}

func TestCoreModeProviderMustReturnNoRepositories(t *testing.T) {
	provider := capabilityLaunchProvider()
	harness := newWorkspaceLaunchHarness(t, provider, preparedResource(repositoryState("a", "a", "main")))
	_, err := harness.runtime.CreateInstance(context.Background(), harness.request)
	if !errors.Is(err, models.ErrExecutorConflict) {
		t.Fatalf("CreateInstance() error = %v, want conflict", err)
	}
}

func TestManagedGitCredentialsWithProviderPreparationAreRejectedBeforeAllocation(t *testing.T) {
	harness := preparedHarness(t, preparedResource(), "a")
	harness.request.Env[githubauth.CredentialBrokerURLEnv] = "https://broker.example/lease"

	_, err := harness.runtime.CreateInstance(context.Background(), harness.request)
	if !errors.Is(err, models.ErrExecutorInvalidConfig) || !strings.Contains(err.Error(), "managed Git credentials") {
		t.Fatalf("CreateInstance() error = %v, want invalid_config naming the credential mode", err)
	}
	if harness.operations.provisionRequest != nil || len(harness.checkpoints) != 0 {
		t.Fatal("a rejected credential mode must allocate and record nothing")
	}
}

func TestManagedGitCredentialsAreAcceptedWhenTheCoreMaterializes(t *testing.T) {
	provider := capabilityLaunchProvider()
	provider.WorkspacePreparation = "core"
	if err := rejectManagedCredentialsForProviderPreparation(provider, map[string]string{githubauth.CredentialBrokerURLEnv: "https://broker.example/lease"}); err != nil {
		t.Fatalf("core mode error = %v, want none", err)
	}
}

func TestChangedIntentUnderOneOperationIsDetectedByTheDigest(t *testing.T) {
	profile := models.ExecutorProviderLaunchProfile{}
	first, _ := buildPluginRepositoryIntent([]PluginRepositorySource{{RepositoryID: "a", Locator: "https://example.test/a.git", Branch: "main"}})
	changed, _ := buildPluginRepositoryIntent([]PluginRepositorySource{{RepositoryID: "a", Locator: "https://example.test/a.git", Branch: "dev"}})
	a, _ := pluginExecutorInputDigestWith(profile, first, nil)
	b, _ := pluginExecutorInputDigestWith(profile, changed, nil)
	if a == b {
		t.Fatal("a changed intent must change the digest so the same operation ID conflicts")
	}
}

func TestRecoveredInventoryIsReverifiedAgainstThePersistedSet(t *testing.T) {
	inventory := pluginExecutorInventory{
		WorkspacePreparation: "provider",
		WorkspaceDescriptor:  nil,
		Repositories:         []pluginExecutorRepository{{ID: "a", Path: "a", HeadCommit: testInventoryCommit}},
	}
	if err := verifyRecoveredPluginExecutorRepositories(inventory, preparedResource(repositoryState("a", "a", "anything"))); err == nil {
		t.Fatal("an inventory without a recorded descriptor must fail closed")
	}
	inventory.WorkspaceDescriptor = recordedDescriptor()
	if err := verifyRecoveredPluginExecutorRepositories(inventory, preparedResource(repositoryState("a", "a", "moved-branch"))); err != nil {
		t.Fatalf("a changed branch must not fail recovery: %v", err)
	}
	for name, resource := range map[string]*pluginsdk.ExecutorResourceDescriptor{
		"missing": preparedResource(),
		"extra":   preparedResource(repositoryState("a", "a", "x"), repositoryState("b", "b", "x")),
		"outside": preparedResource(repositoryState("a", "/elsewhere/a", "x")),
	} {
		if err := verifyRecoveredPluginExecutorRepositories(inventory, resource); err == nil {
			t.Fatalf("%s: recovery unexpectedly accepted the environment", name)
		}
	}
}

func TestCoreModeRecoveryAndPublicationIgnoreRepositories(t *testing.T) {
	inventory := pluginExecutorInventory{WorkspacePreparation: "core"}
	if err := verifyRecoveredPluginExecutorRepositories(inventory, preparedResource(repositoryState("a", "a", "x"))); err != nil {
		t.Fatalf("core mode recovery error = %v, want none", err)
	}
	if repos := publishedPluginExecutorRepos(inventory); repos != nil {
		t.Fatalf("core mode projection = %v, want nil", repos)
	}
}

func TestMaximumRepositoryInventoryFitsTheEnvelopeBudget(t *testing.T) {
	inventory := pluginExecutorInventory{
		NegotiatedCapabilities: []string{"workspace_descriptor", "repository_intent"},
		WorkspacePreparation:   "provider",
		Phase:                  pluginExecutorPhaseProvisioned,
	}
	for i := 0; i < pluginRepositoryIntentMaxEntries; i++ {
		inventory.Repositories = append(inventory.Repositories, pluginExecutorRepository{
			ID:         "6f1c2b1e-9a4d-4a53-8f67-" + strings.Repeat("a", 12),
			Path:       "/srv/workspace/" + strings.Repeat("r", 40),
			HeadCommit: testInventoryCommit,
		})
	}
	if err := checkPluginExecutorEnvelopeBudget(inventory, pluginExecutorDefaultStateBudget); err != nil {
		t.Fatalf("32 repositories must fit the repositories budget: %v", err)
	}
}
