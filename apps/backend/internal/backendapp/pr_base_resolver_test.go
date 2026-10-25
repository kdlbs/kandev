package backendapp

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	githubpkg "github.com/kandev/kandev/internal/github"
	executorpkg "github.com/kandev/kandev/internal/orchestrator/executor"
)

type githubPRBaseLookupServiceStub struct {
	prs       []*githubpkg.TaskPR
	pr        *githubpkg.PR
	getErr    error
	listErr   error
	getCalls  []string
	listCalls [][]string
}

func (s *githubPRBaseLookupServiceStub) GetPRForAutomation(
	_ context.Context, _ string, owner, repo string, number int,
) (*githubpkg.PR, error) {
	s.getCalls = append(s.getCalls, owner+"/"+repo+"#"+strconv.Itoa(number))
	return s.pr, s.getErr
}

func (s *githubPRBaseLookupServiceStub) ListTaskPRs(
	_ context.Context, taskIDs []string,
) (map[string][]*githubpkg.TaskPR, error) {
	s.listCalls = append(s.listCalls, append([]string(nil), taskIDs...))
	if s.listErr != nil {
		return nil, s.listErr
	}
	result := make(map[string][]*githubpkg.TaskPR, len(taskIDs))
	for _, taskID := range taskIDs {
		for _, pr := range s.prs {
			if pr != nil && pr.TaskID == taskID {
				result[taskID] = append(result[taskID], pr)
			}
		}
	}
	return result, nil
}

func TestPRBaseIdentityConversions(t *testing.T) {
	const oid = "0123456789abcdef0123456789abcdef01234567"
	base, err := githubPRBaseFromPR(&githubpkg.PR{
		Number: 42, RepoOwner: "upstream", RepoName: "widgets",
		HeadBranch: "feature", BaseBranch: "release", BaseSHA: oid,
		HeadRepoOwner: "fork-owner", HeadRepoName: "widgets",
		BaseRepoOwner: "upstream", BaseRepoName: "widgets",
	}, "upstream", "widgets", 42, "feature")
	if err != nil {
		t.Fatalf("githubPRBaseFromPR() error: %v", err)
	}
	if base.Target.TargetRepository.Path != "upstream/widgets" || base.Target.TargetBranch != "release" {
		t.Fatalf("target = %#v, want upstream/widgets:release", base.Target)
	}
	if base.Target.HeadRepository.Path != "fork-owner/widgets" || base.Target.HeadBranch != "feature" {
		t.Fatalf("head = %#v, want fork-owner/widgets:feature", base.Target)
	}
	if base.OID != oid {
		t.Fatalf("OID = %q, want %q", base.OID, oid)
	}
}

func TestPRBaseIdentityConversionRejectsMismatchedProviderIdentity(t *testing.T) {
	basePR := &githubpkg.PR{
		Number: 42, RepoOwner: "upstream", RepoName: "widgets",
		HeadBranch: "feature", BaseBranch: "release",
		HeadRepoOwner: "fork-owner", HeadRepoName: "widgets",
		BaseRepoOwner: "upstream", BaseRepoName: "widgets",
	}
	tests := []struct {
		name   string
		mutate func(*githubpkg.PR)
	}{
		{name: "number", mutate: func(pr *githubpkg.PR) { pr.Number++ }},
		{name: "head branch", mutate: func(pr *githubpkg.PR) { pr.HeadBranch = "other" }},
		{name: "base repository", mutate: func(pr *githubpkg.PR) { pr.BaseRepoOwner = "fork-owner" }},
		{name: "missing head repository", mutate: func(pr *githubpkg.PR) { pr.HeadRepoOwner = "" }},
		{name: "invalid base OID", mutate: func(pr *githubpkg.PR) { pr.BaseSHA = "not-an-oid" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := *basePR
			tt.mutate(&pr)
			if _, err := githubPRBaseFromPR(&pr, "upstream", "widgets", 42, "feature"); err == nil {
				t.Fatal("githubPRBaseFromPR() succeeded with mismatched provider identity")
			}
		})
	}
}

func TestSelectLinkedTaskPRForBaseUsesExactAttachmentAndCheckout(t *testing.T) {
	lookup := executorpkg.PRBaseLookup{
		TaskID: "task-1", TaskRepositoryID: "task-repo-1", RepositoryID: "repo-1",
		Number: 42, CheckoutBranch: "feature",
	}
	prs := []*githubpkg.TaskPR{
		{RepositoryID: "repo-1", PRNumber: 42, HeadBranch: "other", Owner: "wrong", Repo: "branch"},
		{RepositoryID: "repo-2", PRNumber: 42, HeadBranch: "feature", Owner: "wrong", Repo: "attachment"},
		{RepositoryID: "repo-1", PRNumber: 41, HeadBranch: "feature", Owner: "wrong", Repo: "number"},
		{RepositoryID: "repo-1", PRNumber: 42, HeadBranch: "feature", Owner: "upstream", Repo: "widgets"},
	}
	got, err := selectLinkedTaskPRForBase(prs, lookup)
	if err != nil {
		t.Fatalf("selectLinkedTaskPRForBase() error: %v", err)
	}
	if got == nil || got.Owner != "upstream" || got.Repo != "widgets" {
		t.Fatalf("selected PR = %#v, want upstream/widgets#42 for repo-1:feature", got)
	}
}

func TestSelectLinkedTaskPRForBaseRejectsAmbiguousExactMatches(t *testing.T) {
	lookup := executorpkg.PRBaseLookup{TaskRepositoryID: "task-repo-1", RepositoryID: "repo-1", Number: 42, CheckoutBranch: "feature"}
	prs := []*githubpkg.TaskPR{
		{RepositoryID: "repo-1", PRNumber: 42, HeadBranch: "feature", Owner: "upstream", Repo: "one"},
		{RepositoryID: "repo-1", PRNumber: 42, HeadBranch: "feature", Owner: "other", Repo: "two"},
	}
	if _, err := selectLinkedTaskPRForBase(prs, lookup); err == nil {
		t.Fatal("selectLinkedTaskPRForBase() succeeded with multiple exact matches")
	}
}

func TestGitHubPRBaseResolverUsesLinkedPRForLegacyAttachment(t *testing.T) {
	service := &githubPRBaseLookupServiceStub{
		prs: []*githubpkg.TaskPR{
			{TaskID: "task-1", RepositoryID: "other-repo", PRNumber: 42, HeadBranch: "feature", Owner: "wrong", Repo: "attachment"},
			{TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42, HeadBranch: "other", Owner: "wrong", Repo: "branch"},
			{TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42, HeadBranch: "feature", Owner: "upstream", Repo: "widgets"},
		},
		pr: &githubpkg.PR{
			Number: 42, RepoOwner: "upstream", RepoName: "widgets", BaseRepoOwner: "upstream", BaseRepoName: "widgets",
			HeadRepoOwner: "fork-owner", HeadRepoName: "widgets", HeadBranch: "feature", BaseBranch: "release",
			BaseSHA: "0123456789abcdef0123456789abcdef01234567",
		},
	}
	resolver := githubPRBaseResolver{service: service}
	lookup := executorpkg.PRBaseLookup{
		TaskID: "task-1", TaskRepositoryID: "task-repo-1", RepositoryID: "repo-1",
		Number: 42, CheckoutBranch: "feature", AttachedOwner: "fork-owner", AttachedRepository: "widgets",
	}
	base, err := resolver.ResolvePRBase(context.Background(), "workspace-1", lookup)
	if err != nil {
		t.Fatalf("ResolvePRBase() error: %v", err)
	}
	if base.Target.TargetRepository.Path != "upstream/widgets" || base.Target.HeadRepository.Path != "fork-owner/widgets" ||
		base.Target.TargetBranch != "release" || base.OID != service.pr.BaseSHA {
		t.Fatalf("resolved base = %#v, want linked upstream PR identity and OID", base)
	}
	if !reflect.DeepEqual(service.getCalls, []string{"upstream/widgets#42"}) ||
		!reflect.DeepEqual(service.listCalls, [][]string{{"task-1"}}) {
		t.Fatalf("provider lookup calls = (%v, %v), want linked upstream PR query", service.getCalls, service.listCalls)
	}
}

func TestGitHubPRBaseResolverUsesAttachedTargetWhenAssociationIsMissing(t *testing.T) {
	service := &githubPRBaseLookupServiceStub{pr: &githubpkg.PR{
		Number: 42, RepoOwner: "upstream", RepoName: "widgets",
		BaseRepoOwner: "upstream", BaseRepoName: "widgets",
		HeadRepoOwner: "fork-owner", HeadRepoName: "widgets", HeadBranch: "feature",
		BaseBranch: "main", BaseSHA: "0123456789abcdef0123456789abcdef01234567",
	}}
	resolver := githubPRBaseResolver{service: service}
	lookup := executorpkg.PRBaseLookup{
		TaskID: "task-1", TaskRepositoryID: "task-repo-1", RepositoryID: "repo-1",
		Number: 42, CheckoutBranch: "feature", AttachedOwner: "upstream", AttachedRepository: "widgets",
	}
	base, err := resolver.ResolvePRBase(context.Background(), "workspace-1", lookup)
	if err != nil {
		t.Fatalf("ResolvePRBase() error: %v", err)
	}
	if base.Target.TargetRepository.Path != "upstream/widgets" ||
		base.Target.HeadRepository.Path != "fork-owner/widgets" || base.Target.HeadBranch != "feature" {
		t.Fatalf("resolved target-attached PR = %#v, want upstream target and validated fork head", base.Target)
	}
	if !reflect.DeepEqual(service.getCalls, []string{"upstream/widgets#42"}) ||
		!reflect.DeepEqual(service.listCalls, [][]string{{"task-1"}}) {
		t.Fatalf("provider lookups = (%v, %v), want empty association check then attached namespace lookup",
			service.getCalls, service.listCalls)
	}
}

func TestGitHubPRBaseResolverClassifiesKnownCrossRepositoryFailure(t *testing.T) {
	service := &githubPRBaseLookupServiceStub{
		prs: []*githubpkg.TaskPR{{
			TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42, HeadBranch: "feature", Owner: "upstream", Repo: "widgets",
		}},
		getErr: errors.New("GitHub unavailable"),
	}
	resolver := githubPRBaseResolver{service: service}
	_, err := resolver.ResolvePRBase(context.Background(), "workspace-1", executorpkg.PRBaseLookup{
		TaskID: "task-1", TaskRepositoryID: "task-repo-1", RepositoryID: "repo-1",
		Number: 42, CheckoutBranch: "feature", AttachedOwner: "fork-owner", AttachedRepository: "widgets",
	})
	var classified interface{ KnownCrossRepository() bool }
	if !errors.As(err, &classified) || !classified.KnownCrossRepository() {
		t.Fatalf("ResolvePRBase() error = %v, want a known cross-repository failure", err)
	}
}

func TestGitHubPRBaseResolverClassifiesAmbiguousLegacyAssociation(t *testing.T) {
	service := &githubPRBaseLookupServiceStub{prs: []*githubpkg.TaskPR{
		{TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42, HeadBranch: "feature", Owner: "upstream", Repo: "widgets"},
		{TaskID: "task-1", RepositoryID: "repo-1", PRNumber: 42, HeadBranch: "feature", Owner: "other", Repo: "widgets"},
	}}
	resolver := githubPRBaseResolver{service: service}
	_, err := resolver.ResolvePRBase(context.Background(), "workspace-1", executorpkg.PRBaseLookup{
		TaskID: "task-1", TaskRepositoryID: "task-repo-1", RepositoryID: "repo-1",
		Number: 42, CheckoutBranch: "feature", AttachedOwner: "fork-owner", AttachedRepository: "widgets",
	})
	var classified interface{ InvalidAssociation() bool }
	if !errors.As(err, &classified) || !classified.InvalidAssociation() {
		t.Fatalf("ResolvePRBase() error = %v, want an invalid/ambiguous association", err)
	}
	if len(service.getCalls) != 0 {
		t.Fatalf("GetPRForAutomation calls = %v, want none for ambiguous association", service.getCalls)
	}
}
