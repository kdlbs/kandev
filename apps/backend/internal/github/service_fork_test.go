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
	getErrors  map[string]error
	created    []string
	createRepo *GitHubRepository
}

func (f *forkResolverFakeClient) GetAuthenticatedUser(context.Context) (string, error) {
	return f.user, nil
}

func (f *forkResolverFakeClient) GetRepository(_ context.Context, owner, repo string) (*GitHubRepository, error) {
	key := owner + "/" + repo
	if err := f.getErrors[key]; err != nil {
		return nil, err
	}
	repository, ok := f.repos[key]
	if !ok {
		return nil, &GitHubAPIError{StatusCode: 404, Endpoint: "/repos/" + key}
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
