package github

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type forkResolverFakeClient struct {
	user       string
	repos      map[string]*GitHubRepository
	forks      []*GitHubRepository
	getErrors  map[string]error
	getCalls   map[string]int
	readyAfter int
	created    []string
	createRepo *GitHubRepository
}

type contributionDestinationVerificationClient struct {
	Client
	repository *GitHubRepository
}

func (c *contributionDestinationVerificationClient) GetRepository(context.Context, string, string) (*GitHubRepository, error) {
	return copyGitHubRepository(c.repository), nil
}

func (f *forkResolverFakeClient) GetAuthenticatedUser(context.Context) (string, error) {
	return f.user, nil
}

func (f *forkResolverFakeClient) GetRepository(_ context.Context, owner, repo string) (*GitHubRepository, error) {
	key := owner + "/" + repo
	if f.getCalls == nil {
		f.getCalls = make(map[string]int)
	}
	f.getCalls[key]++
	if err := f.getErrors[key]; err != nil {
		return nil, err
	}
	repository, ok := f.repos[key]
	if !ok {
		return nil, &GitHubAPIError{StatusCode: 404, Endpoint: "/repos/" + key}
	}
	if f.readyAfter > 0 && key != "kdlbs/kandev" && f.getCalls[key] >= f.readyAfter {
		repository.PushAccess = true
		repository.AdminAccess = true
	}
	return repository, nil
}

func (f *forkResolverFakeClient) CreateFork(_ context.Context, owner, repo string) (*GitHubRepository, error) {
	f.created = append(f.created, owner+"/"+repo)
	if f.createRepo == nil {
		return nil, errors.New("fake fork creation failed")
	}
	f.repos[f.createRepo.FullName] = f.createRepo
	return f.createRepo, nil
}

func (f *forkResolverFakeClient) ListRepositoryForks(context.Context, string, string) ([]*GitHubRepository, error) {
	result := make([]*GitHubRepository, 0, len(f.forks))
	for _, fork := range f.forks {
		result = append(result, copyGitHubRepository(fork))
	}
	return result, nil
}

func TestResolveContributionForkUsesDirectTargetWrite(t *testing.T) {
	canonical := testGitHubRepository("kdlbs/kandev", "100", true)
	client := &forkResolverFakeClient{
		user:  "alice",
		repos: map[string]*GitHubRepository{"kdlbs/kandev": canonical},
	}

	result, err := resolveContributionFork(
		context.Background(), client, AuthPrincipal{Kind: AuthPrincipalHuman, Login: "alice"},
		"kdlbs", "kandev", true,
	)
	if err != nil {
		t.Fatalf("resolveContributionFork: %v", err)
	}
	if result.Status != ContributionForkStatusDirectWrite {
		t.Fatalf("status = %q, want %q", result.Status, ContributionForkStatusDirectWrite)
	}
	if result.Destination != nil {
		t.Fatalf("direct target returned destination: %#v", result.Destination)
	}
	if len(client.created) != 0 {
		t.Fatalf("created forks = %v, want none", client.created)
	}
}

func TestResolveContributionForkAcceptsOnlyExactWritableFork(t *testing.T) {
	canonical := testGitHubRepository("kdlbs/kandev", "100", false)
	fork := testGitHubRepository("alice/kandev", "200", true)
	fork.Fork = true
	fork.ParentID = canonical.ID
	fork.ParentFullName = canonical.FullName
	client := &forkResolverFakeClient{
		user:  "alice",
		repos: map[string]*GitHubRepository{"kdlbs/kandev": canonical, "alice/kandev": fork},
	}

	result, err := resolveContributionFork(
		context.Background(), client, AuthPrincipal{Kind: AuthPrincipalHuman, Login: "alice"},
		"kdlbs", "kandev", true,
	)
	if err != nil {
		t.Fatalf("resolveContributionFork: %v", err)
	}
	if result.Status != ContributionForkStatusReady {
		t.Fatalf("status = %q, want %q", result.Status, ContributionForkStatusReady)
	}
	if result.Destination == nil {
		t.Fatal("exact fork did not return a destination")
	}
	if got := result.Destination.TargetRepository.Path; got != "alice/kandev" {
		t.Fatalf("destination target = %q, want alice/kandev", got)
	}
	if len(client.created) != 0 {
		t.Fatalf("created forks = %v, want none", client.created)
	}
}

func TestResolveContributionForkReusesRenamedForkFromForkNetwork(t *testing.T) {
	canonical := testGitHubRepository("kdlbs/kandev", "100", false)
	fork := testGitHubRepository("alice/kandev-renamed", "200", true)
	fork.Fork = true
	fork.ParentID = canonical.ID
	fork.ParentFullName = canonical.FullName
	client := &forkResolverFakeClient{
		user:  "alice",
		repos: map[string]*GitHubRepository{"kdlbs/kandev": canonical, "alice/kandev-renamed": fork},
		getErrors: map[string]error{
			"alice/kandev": &GitHubAPIError{StatusCode: 404, Endpoint: "/repos/alice/kandev"},
		},
		forks: []*GitHubRepository{fork},
	}

	result, err := resolveContributionFork(
		context.Background(), client, AuthPrincipal{Kind: AuthPrincipalHuman, Login: "alice"},
		"kdlbs", "kandev", true,
	)
	if err != nil {
		t.Fatalf("resolveContributionFork: %v", err)
	}
	if result.Destination == nil || result.Destination.TargetRepository.Path != "alice/kandev-renamed" {
		t.Fatalf("destination = %#v, want the renamed fork", result.Destination)
	}
	if len(client.created) != 0 {
		t.Fatalf("created forks = %v, want none", client.created)
	}
	second, err := resolveContributionFork(
		context.Background(), client, AuthPrincipal{Kind: AuthPrincipalHuman, Login: "alice"},
		"kdlbs", "kandev", true,
	)
	if err != nil {
		t.Fatalf("second resolveContributionFork: %v", err)
	}
	if second.Destination == nil || second.Destination.TargetRepository.Path != "alice/kandev-renamed" {
		t.Fatalf("second destination = %#v, want the same renamed fork", second.Destination)
	}
	if len(client.created) != 0 {
		t.Fatalf("created forks after second resolution = %v, want none", client.created)
	}
}

func TestBindContributionDestinationCredentialCapturesConnectionGeneration(t *testing.T) {
	canonical := testGitHubRepository("kdlbs/kandev", "100", false)
	fork := testGitHubRepository("alice/kandev", "200", true)
	fork.Fork = true
	fork.ParentID = canonical.ID
	fork.ParentFullName = canonical.FullName
	destination, err := makeContributionDestination(canonical, fork)
	if err != nil {
		t.Fatalf("makeContributionDestination: %v", err)
	}
	resolved := &resolvedServiceClient{
		credential: &ResolvedCredential{
			Principal:            AuthPrincipal{Source: ConnectionSourcePAT, Login: "alice"},
			CredentialGeneration: 9,
		},
	}

	if err := bindContributionDestinationCredential(destination, resolved); err != nil {
		t.Fatalf("bindContributionDestinationCredential: %v", err)
	}
	if destination.CredentialBinding == nil || destination.CredentialBinding.Source != string(ConnectionSourcePAT) ||
		destination.CredentialBinding.Login != "alice" || destination.CredentialBinding.CredentialGeneration != 9 {
		t.Fatalf("credential binding = %#v", destination.CredentialBinding)
	}
}

func TestVerifyContributionDestinationForWorkspaceRejectsRecreatedTarget(t *testing.T) {
	canonical := testGitHubRepository("kdlbs/kandev", "100", false)
	fork := testGitHubRepository("automation/kandev", "200", true)
	fork.Fork = true
	fork.ParentID = canonical.ID
	fork.ParentFullName = canonical.FullName
	client := &contributionDestinationVerificationClient{Client: NewMockClient(), repository: fork}
	connections := testConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace-1": {
			WorkspaceID: "workspace-1", Source: ConnectionSourcePAT, Login: "automation",
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}}
	resolver := NewCredentialResolver(connections, fakeAuthSecrets{
		WorkspacePATSecretKey("workspace-1"): "workspace-token",
	})
	resolver.SetAutomationProvider(testAutomationCredentialProvider{client: client})
	service := &Service{resolver: resolver}

	verify := func() error {
		return service.VerifyContributionDestinationForWorkspace(
			context.Background(), "workspace-1", "kdlbs", "kandev", "100", "automation", "kandev", "200",
		)
	}
	if err := verify(); err != nil {
		t.Fatalf("VerifyContributionDestinationForWorkspace() error = %v", err)
	}

	client.repository.ID = 201
	if err := verify(); err == nil {
		t.Fatal("recreated target with a different provider ID was accepted")
	}
	client.repository.ID = 200
	client.repository.ParentID = 101
	if err := verify(); err == nil {
		t.Fatal("target with a different parent provider ID was accepted")
	}
}

func TestResolveContributionForkRejectsExistingNonWritableFork(t *testing.T) {
	canonical := testGitHubRepository("kdlbs/kandev", "100", false)
	fork := testGitHubRepository("alice/kandev", "200", false)
	fork.Fork = true
	fork.ParentID = canonical.ID
	fork.ParentFullName = canonical.FullName
	client := &forkResolverFakeClient{
		user:  "alice",
		repos: map[string]*GitHubRepository{"kdlbs/kandev": canonical, "alice/kandev": fork},
	}

	_, err := resolveContributionFork(
		context.Background(), client, AuthPrincipal{Kind: AuthPrincipalHuman, Login: "alice"},
		"kdlbs", "kandev", true,
	)
	if !errors.Is(err, ErrContributionForkNotWritable) {
		t.Fatalf("error = %v, want ErrContributionForkNotWritable", err)
	}
}

func TestResolveContributionForkRejectsWrongParent(t *testing.T) {
	canonical := testGitHubRepository("kdlbs/kandev", "100", false)
	wrongParent := testGitHubRepository("alice/kandev", "200", true)
	wrongParent.Fork = true
	wrongParent.ParentID = 999
	wrongParent.ParentFullName = "other/project"
	client := &forkResolverFakeClient{
		user:  "alice",
		repos: map[string]*GitHubRepository{"kdlbs/kandev": canonical, "alice/kandev": wrongParent},
	}

	_, err := resolveContributionFork(
		context.Background(), client, AuthPrincipal{Kind: AuthPrincipalHuman, Login: "alice"},
		"kdlbs", "kandev", true,
	)
	if !errors.Is(err, ErrContributionForkConflict) {
		t.Fatalf("error = %v, want ErrContributionForkConflict", err)
	}
	if len(client.created) != 0 {
		t.Fatalf("created forks = %v, want none", client.created)
	}
}

func TestResolveContributionForkBlocksAppWithoutDirectWrite(t *testing.T) {
	canonical := testGitHubRepository("kdlbs/kandev", "100", false)
	client := &forkResolverFakeClient{
		user:  "kdlbs-app[bot]",
		repos: map[string]*GitHubRepository{"kdlbs/kandev": canonical},
	}

	_, err := resolveContributionFork(
		context.Background(), client, AuthPrincipal{Kind: AuthPrincipalApp, Login: "kdlbs-app[bot]"},
		"kdlbs", "kandev", true,
	)
	if !errors.Is(err, ErrContributionForkAppUnsupported) {
		t.Fatalf("error = %v, want ErrContributionForkAppUnsupported", err)
	}
	if len(client.created) != 0 {
		t.Fatalf("created forks = %v, want none", client.created)
	}
}

func TestResolveContributionForkReportsCreatableWithoutCreating(t *testing.T) {
	canonical := testGitHubRepository("kdlbs/kandev", "100", false)
	client := &forkResolverFakeClient{
		user:  "alice",
		repos: map[string]*GitHubRepository{"kdlbs/kandev": canonical},
		getErrors: map[string]error{
			"alice/kandev": &GitHubAPIError{StatusCode: 404, Endpoint: "/repos/alice/kandev"},
		},
	}

	result, err := resolveContributionFork(
		context.Background(), client, AuthPrincipal{Kind: AuthPrincipalHuman, Source: ConnectionSourceGHCLI, Login: "alice"},
		"kdlbs", "kandev", false,
	)
	if err != nil {
		t.Fatalf("resolveContributionFork: %v", err)
	}
	if result.Status != ContributionForkStatusCreatable {
		t.Fatalf("status = %q, want %q", result.Status, ContributionForkStatusCreatable)
	}
	if result.Destination != nil {
		t.Fatalf("creatable probe returned destination: %#v", result.Destination)
	}
	if len(client.created) != 0 {
		t.Fatalf("created forks = %v, want none", client.created)
	}
}

func TestResolveContributionForkCreatesAndVerifiesMissingHumanFork(t *testing.T) {
	canonical := testGitHubRepository("kdlbs/kandev", "100", false)
	fork := testGitHubRepository("alice/kandev", "200", true)
	fork.Fork = true
	fork.ParentID = canonical.ID
	fork.ParentFullName = canonical.FullName
	client := &forkResolverFakeClient{
		user:       "alice",
		repos:      map[string]*GitHubRepository{"kdlbs/kandev": canonical},
		createRepo: fork,
	}

	result, err := resolveContributionFork(
		context.Background(), client, AuthPrincipal{Kind: AuthPrincipalHuman, Source: ConnectionSourcePAT, Login: "alice"},
		"kdlbs", "kandev", true,
	)
	if err != nil {
		t.Fatalf("resolveContributionFork: %v", err)
	}
	if result.Status != ContributionForkStatusReady || result.Destination == nil {
		t.Fatalf("result = %#v, want ready destination", result)
	}
	if len(client.created) != 1 || client.created[0] != "kdlbs/kandev" {
		t.Fatalf("created forks = %v, want [kdlbs/kandev]", client.created)
	}
}

func TestResolveContributionForkUsesCreatedRepositoryIdentityAndWaitsForReadiness(t *testing.T) {
	canonical := testGitHubRepository("kdlbs/kandev", "100", false)
	fork := testGitHubRepository("alice/kandev-renamed", "200", false)
	fork.Fork = true
	fork.ParentID = canonical.ID
	fork.ParentFullName = canonical.FullName
	client := &forkResolverFakeClient{
		user:       "alice",
		repos:      map[string]*GitHubRepository{"kdlbs/kandev": canonical},
		createRepo: fork,
		readyAfter: 11,
	}

	result, err := resolveContributionFork(
		context.Background(), client, AuthPrincipal{Kind: AuthPrincipalHuman, Source: ConnectionSourcePAT, Login: "alice"},
		"kdlbs", "kandev", true,
	)
	if err != nil {
		t.Fatalf("resolveContributionFork: %v", err)
	}
	if result.Destination == nil || result.Destination.TargetRepository.Path != "alice/kandev-renamed" {
		t.Fatalf("destination = %#v, want created repository identity", result.Destination)
	}
	if client.getCalls["alice/kandev-renamed"] < client.readyAfter {
		t.Fatalf("fork readiness checks = %d, want at least %d", client.getCalls["alice/kandev-renamed"], client.readyAfter)
	}
}

func TestIsContributionRepositoryNotFoundRequiresProviderError(t *testing.T) {
	for _, err := range []error{
		errors.New("endpoint /repos/alice/404-user returned 500"),
		errors.New("repository not found in a wrapped provider failure"),
	} {
		if isContributionRepositoryNotFound(err) {
			t.Fatalf("isContributionRepositoryNotFound(%v) = true, want false", err)
		}
	}
}

func testGitHubRepository(fullName, id string, push bool) *GitHubRepository {
	var numericID int64
	if _, err := fmt.Sscan(id, &numericID); err != nil {
		panic(err)
	}
	parts := splitRepositoryFullName(fullName)
	return &GitHubRepository{
		ID:          numericID,
		FullName:    fullName,
		Owner:       parts[0],
		Name:        parts[1],
		CloneURL:    "https://github.com/" + fullName + ".git",
		Fork:        false,
		PushAccess:  push,
		AdminAccess: push,
	}
}

func splitRepositoryFullName(fullName string) []string {
	for index, character := range fullName {
		if character == '/' {
			return []string{fullName[:index], fullName[index+1:]}
		}
	}
	panic("invalid repository full name: " + fullName)
}
