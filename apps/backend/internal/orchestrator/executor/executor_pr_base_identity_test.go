package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestGitHubComparisonRepositoryFromRepositoryAcceptsCanonicalProviderHost(t *testing.T) {
	repo := &models.Repository{
		Provider: "github", ProviderHost: "https://github.com",
		ProviderOwner: "test-owner", ProviderName: "test-repo",
	}

	got, ok := githubComparisonRepositoryFromRepository(repo)
	if !ok {
		t.Fatal("githubComparisonRepositoryFromRepository() rejected canonical GitHub provider host")
	}
	if got.Host != "github.com" || got.Path != "test-owner/test-repo" ||
		got.RemoteURL != "https://github.com/test-owner/test-repo.git" {
		t.Fatalf("normalized GitHub repository = %#v", got)
	}
}

func TestNormalizeGitHubComparisonRepositoryRejectsOtherHosts(t *testing.T) {
	for _, host := range []string{"https://github.com.evil", "https://github.com:8443", "https://github.com/api"} {
		t.Run(host, func(t *testing.T) {
			if _, ok := normalizeGitHubComparisonRepository(models.ComparisonTargetRepository{
				Host: host, Path: "test-owner/test-repo",
			}); ok {
				t.Fatalf("normalizeGitHubComparisonRepository accepted host %q", host)
			}
		})
	}
}

func TestResolveTaskRepoInfo_PRBaseUsesTargetRepository(t *testing.T) {
	target := forkPRComparisonTarget()
	metadata := map[string]interface{}{"pr_number": 42}
	if err := models.PutComparisonTarget(metadata, target); err != nil {
		t.Fatalf("PutComparisonTarget() error: %v", err)
	}
	retargeted := *target
	retargeted.TargetBranch = "stable"
	resolver := &recordingPRBaseResolver{result: &models.PRBase{Target: retargeted, OID: "0123456789abcdef0123456789abcdef01234567"}}
	info := resolveForkPRTask(t, metadata, resolver)
	want := prBaseResolveCall{workspaceID: "workspace-1", owner: "upstream", repo: "widgets", number: 42}
	if len(resolver.calls) != 1 || resolver.calls[0] != want {
		t.Fatalf("resolver calls = %#v, want %#v", resolver.calls, want)
	}
	if info.BaseBranch != "stable" || info.QualifiedPRBase == nil || info.QualifiedPRBase.OID == "" {
		t.Fatalf("resolved PR base = %#v, branch = %q, want stable with observed OID", info.QualifiedPRBase, info.BaseBranch)
	}
}

func TestResolveTaskRepoInfo_PRBaseRejectsDifferentHeadForSameNumber(t *testing.T) {
	target := forkPRComparisonTarget()
	metadata := map[string]interface{}{"pr_number": 42}
	if err := models.PutComparisonTarget(metadata, target); err != nil {
		t.Fatalf("PutComparisonTarget() error: %v", err)
	}
	unrelated := *target
	unrelated.TargetBranch = "main"
	unrelated.HeadRepository = models.ComparisonTargetRepository{
		Host: "github.com", Path: "other-fork/widgets", RemoteURL: "https://github.com/other-fork/widgets.git",
	}
	resolver := &recordingPRBaseResolver{result: &models.PRBase{Target: unrelated}}
	info := resolveForkPRTask(t, metadata, resolver)
	if info.BaseBranch != target.TargetBranch || info.QualifiedPRBase == nil ||
		info.QualifiedPRBase.Target.TargetBranch != target.TargetBranch {
		t.Fatalf("mismatched PR result replaced stored target: %#v, branch=%q", info.QualifiedPRBase, info.BaseBranch)
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.15
func TestResolveTaskRepoInfo_LegacyForkPRQualifiesLiveLinkedBaseForMaterialization(t *testing.T) {
	target := forkPRComparisonTarget()
	target.TargetBranch = "stable"
	resolver := &recordingPRBaseResolver{result: &models.PRBase{
		Target: *target, OID: "0123456789abcdef0123456789abcdef01234567",
	}}
	info, err := resolveForkPRTaskWithError(t, map[string]interface{}{"pr_number": 42}, resolver)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo() error: %v", err)
	}
	if info.QualifiedPRBase == nil || info.ComparisonTarget == nil ||
		info.QualifiedPRBase.Target.TargetRepository.Path != "upstream/widgets" {
		t.Fatalf("resolved legacy fork PR base = %#v, comparison target = %#v", info.QualifiedPRBase, info.ComparisonTarget)
	}
	specs := buildRepoSpecs([]*repoInfo{info})
	if len(specs) != 1 || specs[0].QualifiedPRBase == nil ||
		specs[0].QualifiedPRBase.OID != resolver.result.OID || specs[0].ComparisonTarget == nil {
		t.Fatalf("materialization specs = %#v, want qualified upstream base from legacy PR metadata", specs)
	}
}

func TestResolveTaskRepoInfo_LegacyPRRejectsSameNumberAndBranchFromOtherHeadRepository(t *testing.T) {
	unrelated := forkPRComparisonTarget()
	unrelated.TargetBranch = "unrelated-base"
	unrelated.HeadRepository = models.ComparisonTargetRepository{
		Host: "github.com", Path: "other-fork/widgets", RemoteURL: "https://github.com/other-fork/widgets.git",
	}
	resolver := &recordingPRBaseResolver{result: &models.PRBase{Target: *unrelated}}
	info, err := resolveForkPRTaskWithError(t, map[string]interface{}{"pr_number": 42}, resolver)
	if err == nil {
		t.Fatalf("resolveTaskRepoInfo() accepted unrelated PR base %q: %#v", unrelated.TargetBranch, info)
	}
}

func TestResolveTaskRepoInfo_PreservesManualBaseOverrideForLegacyPR(t *testing.T) {
	target := forkPRComparisonTarget()
	resolver := &recordingPRBaseResolver{result: &models.PRBase{Target: *target}}
	info, err := resolveForkPRTaskWithError(t, map[string]interface{}{
		"pr_number": 42, "manual_base_branch_override": true,
	}, resolver)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo() error: %v", err)
	}
	if info.BaseBranch != "main" || info.QualifiedPRBase != nil || len(resolver.calls) != 0 {
		t.Fatalf("manual base override was replaced: base=%q qualified=%#v resolver calls=%#v",
			info.BaseBranch, info.QualifiedPRBase, resolver.calls)
	}
}

func TestResolveTaskRepoInfo_ContributionValidatesPRHeadAgainstSourceRepository(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", SourceType: sourceTypeLocal, LocalPath: t.TempDir(),
		Provider: "github", ProviderOwner: "upstream", ProviderName: "widgets", DefaultBranch: "main",
	}
	metadata := map[string]interface{}{"pr_number": 42}
	binding := &models.RemoteContribution{
		Version: models.RemoteContributionVersion, Provider: models.RemoteContributionProviderGitHub,
		Kind: models.RemoteContributionKindPullRequest, CanonicalURL: "https://github.com/upstream/widgets/pull/42",
		Number: 42, State: models.RemoteContributionStateOpen, BaseBranch: "main", HeadBranch: "feature",
		HeadSHA: "0123456789abcdef0123456789abcdef01234567", CollaborationAllowed: true,
		SourceRepository: models.RemoteContributionRepository{
			Host: "github.com", Path: "fork-owner/widgets", RemoteURL: "https://github.com/fork-owner/widgets.git",
		},
	}
	if err := models.PutRemoteContribution(metadata, binding); err != nil {
		t.Fatalf("PutRemoteContribution() error: %v", err)
	}
	target := forkPRComparisonTarget()
	target.TargetBranch = "release"
	resolver := &recordingPRBaseResolver{result: &models.PRBase{Target: *target}}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetPRBaseResolver(resolver)
	info, err := exec.resolveTaskRepoInfo(context.Background(), &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main",
		CheckoutBranch: "feature", Metadata: metadata,
	})
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo() error: %v", err)
	}
	if info.BaseBranch != "release" || info.QualifiedPRBase != nil {
		t.Fatalf("valid contribution base = %q, qualified=%#v; want attached base repo branch and ordinary same-repo handling",
			info.BaseBranch, info.QualifiedPRBase)
	}
}

type knownCrossRepositoryPRBaseError struct{}

func (knownCrossRepositoryPRBaseError) Error() string { return "upstream PR base unavailable" }

func (knownCrossRepositoryPRBaseError) KnownCrossRepository() bool { return true }

func TestResolveTaskRepoInfo_KnownCrossRepositoryFailureCannotUseStoredBranch(t *testing.T) {
	resolver := &recordingPRBaseResolver{err: knownCrossRepositoryPRBaseError{}}
	info, err := resolveForkPRTaskWithError(t, map[string]interface{}{"pr_number": 42}, resolver)
	if err == nil {
		t.Fatalf("resolveTaskRepoInfo() returned branch-only base %q after known cross-repository resolution failed: %#v",
			info.BaseBranch, info)
	}
}

func forkPRComparisonTarget() *models.ComparisonTarget {
	return &models.ComparisonTarget{
		Version: models.ComparisonTargetVersion, Provider: models.ComparisonTargetProviderGitHub,
		Kind: models.ComparisonTargetKindPullRequest, Number: 42,
		HeadBranch: "feature", TargetBranch: "release",
		HeadRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "fork-owner/widgets", RemoteURL: "https://github.com/fork-owner/widgets.git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "upstream/widgets", RemoteURL: "https://github.com/upstream/widgets.git",
		},
	}
}

func resolveForkPRTask(t *testing.T, metadata map[string]interface{}, resolver *recordingPRBaseResolver) *repoInfo {
	t.Helper()
	info, err := resolveForkPRTaskWithError(t, metadata, resolver)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo() error: %v", err)
	}
	return info
}

func resolveForkPRTaskWithError(t *testing.T, metadata map[string]interface{}, resolver *recordingPRBaseResolver) (*repoInfo, error) {
	t.Helper()
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		WorkspaceID:   "workspace-1",
		SourceType:    sourceTypeLocal,
		LocalPath:     t.TempDir(),
		Provider:      "github",
		ProviderOwner: "fork-owner",
		ProviderName:  "widgets",
		DefaultBranch: "main",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetPRBaseResolver(resolver)

	return exec.resolveTaskRepoInfo(context.Background(), &models.TaskRepository{
		ID:             "task-repo-1",
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		BaseBranch:     "main",
		CheckoutBranch: "feature",
		Metadata:       metadata,
	})
}
