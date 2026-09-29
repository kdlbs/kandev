package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestMaterializeRepository_QualifiedPRBaseUsesUpstreamAndKeepsPRHead(t *testing.T) {
	locator, qualifiedBase, prHeadOID, upstreamBaseOID, forkMainOID, forkTargetOID := qualifiedMaterializationFixture(t)
	destination := filepath.Join(t.TempDir(), "fork-feature-work")
	reused, err := materializeRepositoryWithQualifiedPRBase(
		context.Background(), locator, destination, "release/next", "feature/work", 42,
		&qualifiedBase, nil, nil, nil,
	)
	if err != nil || reused {
		t.Fatalf("materialize qualified PR repository = reused:%t err:%v", reused, err)
	}
	if got := strings.TrimSpace(materializeGitOutputForTest(t, destination, "rev-parse", "HEAD")); got != prHeadOID {
		t.Fatalf("HEAD = %q, want PR head %q", got, prHeadOID)
	}
	if got := strings.TrimSpace(materializeGitOutputForTest(t, destination, "rev-parse", qualifiedBase.Target.ComparisonRef()+"^{commit}")); got != upstreamBaseOID {
		t.Fatalf("qualified target = %q, want upstream base %q", got, upstreamBaseOID)
	}
	if got := strings.TrimSpace(materializeGitOutputForTest(t, destination, "rev-parse", "refs/remotes/origin/main")); got != forkMainOID {
		t.Fatalf("origin/main = %q, want fork commit %q", got, forkMainOID)
	}
	if got := strings.TrimSpace(materializeGitOutputForTest(t, destination, "rev-parse", "refs/remotes/origin/release/next")); got != forkTargetOID {
		t.Fatalf("origin/release/next = %q, want fork commit %q", got, forkTargetOID)
	}
	forkHeadOID := strings.TrimSpace(materializeGitOutputForTest(t, destination, "rev-parse", "refs/remotes/origin/feature/work"))
	if forkHeadOID == prHeadOID {
		t.Fatalf("fixture fork branch unexpectedly matches the PR head: %q", forkHeadOID)
	}
	if got := strings.TrimSpace(materializeGitOutputForTest(t, destination, "config", "--get", "remote.origin.url")); got != locator {
		t.Fatalf("origin URL = %q, want %q", got, locator)
	}
	if got := strings.TrimSpace(materializeGitOutputForTest(t, destination, "config", "--get", "remote."+qualifiedBase.Target.ComparisonRemoteName()+".pushurl")); got != "DISABLED" {
		t.Fatalf("comparison push URL = %q, want disabled", got)
	}

	reused, err = materializeRepositoryWithQualifiedPRBase(
		context.Background(), locator, destination, "release/next", "feature/work", 42,
		&qualifiedBase, nil, nil, nil,
	)
	if err != nil || !reused {
		t.Fatalf("reuse qualified PR checkout = reused:%t err:%v, want verified reuse", reused, err)
	}
	staleBase := qualifiedBase
	staleBase.OID = strings.Repeat("f", 40)
	if _, err := materializeRepositoryWithQualifiedPRBase(
		context.Background(), locator, destination, "release/next", "feature/work", 42,
		&staleBase, nil, nil, nil,
	); err == nil || !strings.Contains(err.Error(), "OID changed") {
		t.Fatalf("reuse with stale qualified OID error = %v, want OID mismatch", err)
	}
}

func TestMaterializeRepository_QualifiedPRBaseRejectsOIDDrift(t *testing.T) {
	locator, qualifiedBase, _, _, _, _ := qualifiedMaterializationFixture(t)
	qualifiedBase.OID = strings.Repeat("f", 40)
	destination := filepath.Join(t.TempDir(), "fork-feature-work")
	_, err := materializeRepositoryWithQualifiedPRBase(
		context.Background(), locator, destination, "release/next", "feature/work", 42,
		&qualifiedBase, nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "OID changed") {
		t.Fatalf("materialization error = %v, want required OID mismatch", err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("OID mismatch published destination: stat error = %v", statErr)
	}
}

func TestMaterializeRepository_QualifiedPRBaseRejectsRequestMismatch(t *testing.T) {
	locator, qualifiedBase, _, _, _, _ := qualifiedMaterializationFixture(t)
	destination := filepath.Join(t.TempDir(), "fork-feature-work")
	_, err := materializeRepositoryWithQualifiedPRBase(
		context.Background(), locator, destination, "main", "feature/work", 42,
		&qualifiedBase, nil, nil, nil,
	)
	if err == nil {
		t.Fatal("materialization accepted branch-only base that conflicts with qualified target")
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("invalid request published destination: stat error = %v", statErr)
	}
}

func TestWorkspaceMaterializeRepository_RejectsQualifiedPRBaseMismatch(t *testing.T) {
	s := newMaterializeTestServer(t, t.TempDir())
	_, base, _, _, _, _ := qualifiedMaterializationFixture(t)
	request := MaterializeRepositoryRequest{
		RepositoryURL: "https://github.com/fork/widget.git", Destination: "repo",
		BaseBranch: "main", CheckoutBranch: "feature/work", PRNumber: 42, QualifiedPRBase: &base,
	}
	w := workspaceMaterializeRequest(t, s, request)
	if w.Code != 400 {
		t.Fatalf("status = %d, body = %s, want invalid-qualified-target 400", w.Code, w.Body.String())
	}
}

func TestWorkspaceMaterializeRepository_RejectsContributionFromDifferentHeadRepository(t *testing.T) {
	s := newMaterializeTestServer(t, t.TempDir())
	_, base, _, _, _, _ := qualifiedMaterializationFixture(t)
	binding := &models.RemoteContribution{
		Version: models.RemoteContributionVersion, Provider: models.RemoteContributionProviderGitHub,
		Kind: models.RemoteContributionKindPullRequest, CanonicalURL: "https://github.com/upstream/widget/pull/42",
		Number: 42, State: models.RemoteContributionStateOpen, BaseBranch: "release/next",
		HeadBranch: "feature/work", HeadSHA: strings.Repeat("a", 40), CollaborationAllowed: true,
		SourceRepository: models.RemoteContributionRepository{
			Host: "github.com", Path: "other-fork/widget", RemoteURL: "https://github.com/other-fork/widget.git",
		},
	}
	request := MaterializeRepositoryRequest{
		RepositoryURL: "https://github.com/fork/widget.git", Destination: "repo",
		BaseBranch: "release/next", CheckoutBranch: "feature/work", PRNumber: 42,
		QualifiedPRBase: &base, RemoteContribution: binding,
	}
	response := workspaceMaterializeRequest(t, s, request)
	if response.Code != 400 {
		t.Fatalf("status = %d, body = %s, want mismatched contribution identity 400", response.Code, response.Body.String())
	}
}

func workspaceMaterializeRequest(t *testing.T, s *Server, request MaterializeRepositoryRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/v1/workspace/materialize-repository", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

func qualifiedMaterializationFixture(t *testing.T) (string, models.PRBase, string, string, string, string) {
	t.Helper()
	root := t.TempDir()
	upstream := filepath.Join(root, "upstream.git")
	fork := filepath.Join(root, "fork.git")
	upstreamSeed := filepath.Join(root, "upstream-seed")
	forkSeed := filepath.Join(root, "fork-seed")

	materializationTestGit(t, root, "init", "--bare", "--initial-branch=main", upstream)
	materializationTestGit(t, root, "init", "--initial-branch=main", upstreamSeed)
	configureMaterializationCommitter(t, upstreamSeed)
	writeMaterializationTestFile(t, upstreamSeed, "base.txt", "upstream main\n")
	materializationTestGit(t, upstreamSeed, "add", "base.txt")
	materializationTestGit(t, upstreamSeed, "commit", "-m", "upstream main")
	materializationTestGit(t, upstreamSeed, "remote", "add", "origin", upstream)
	materializationTestGit(t, upstreamSeed, "push", "origin", "main")
	materializationTestGit(t, upstreamSeed, "checkout", "-b", "release/next")
	writeMaterializationTestFile(t, upstreamSeed, "target.txt", "upstream release target\n")
	materializationTestGit(t, upstreamSeed, "add", "target.txt")
	materializationTestGit(t, upstreamSeed, "commit", "-m", "upstream release target")
	materializationTestGit(t, upstreamSeed, "push", "origin", "release/next")
	upstreamBaseOID := strings.TrimSpace(materializeGitOutputForTest(t, upstreamSeed, "rev-parse", "HEAD"))
	materializationTestGit(t, upstreamSeed, "checkout", "-b", "feature/work")
	writeMaterializationTestFile(t, upstreamSeed, "pr.txt", "PR head\n")
	materializationTestGit(t, upstreamSeed, "add", "pr.txt")
	materializationTestGit(t, upstreamSeed, "commit", "-m", "PR head")
	prHeadOID := strings.TrimSpace(materializeGitOutputForTest(t, upstreamSeed, "rev-parse", "HEAD"))
	materializationTestGit(t, upstreamSeed, "push", "origin", "HEAD:refs/pull/42/head")

	materializationTestGit(t, root, "init", "--bare", "--initial-branch=main", fork)
	materializationTestGit(t, root, "init", "--initial-branch=main", forkSeed)
	configureMaterializationCommitter(t, forkSeed)
	writeMaterializationTestFile(t, forkSeed, "base.txt", "fork main\n")
	materializationTestGit(t, forkSeed, "add", "base.txt")
	materializationTestGit(t, forkSeed, "commit", "-m", "fork main")
	materializationTestGit(t, forkSeed, "remote", "add", "origin", fork)
	materializationTestGit(t, forkSeed, "push", "origin", "main")
	forkMainOID := strings.TrimSpace(materializeGitOutputForTest(t, forkSeed, "rev-parse", "HEAD"))
	materializationTestGit(t, forkSeed, "checkout", "-b", "release/next")
	writeMaterializationTestFile(t, forkSeed, "target.txt", "fork release\n")
	materializationTestGit(t, forkSeed, "add", "target.txt")
	materializationTestGit(t, forkSeed, "commit", "-m", "fork release")
	materializationTestGit(t, forkSeed, "push", "origin", "release/next")
	forkTargetOID := strings.TrimSpace(materializeGitOutputForTest(t, forkSeed, "rev-parse", "HEAD"))
	materializationTestGit(t, forkSeed, "checkout", "-b", "feature/work")
	writeMaterializationTestFile(t, forkSeed, "fork-pr.txt", "fork branch with the same name\n")
	materializationTestGit(t, forkSeed, "add", "fork-pr.txt")
	materializationTestGit(t, forkSeed, "commit", "-m", "fork feature branch")
	materializationTestGit(t, forkSeed, "push", "origin", "feature/work")

	globalConfig := filepath.Join(root, "gitconfig")
	config := fmt.Sprintf(
		"[url %q]\n\tinsteadOf = https://github.com/upstream/widget.git\n[url %q]\n\tinsteadOf = https://github.com/fork/widget.git\n",
		"file://"+filepath.ToSlash(upstream), "file://"+filepath.ToSlash(fork),
	)
	if err := os.WriteFile(globalConfig, []byte(config), 0o600); err != nil {
		t.Fatalf("write isolated Git config: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	target := models.ComparisonTarget{
		Version: models.ComparisonTargetVersion, Provider: models.ComparisonTargetProviderGitHub,
		Kind: models.ComparisonTargetKindPullRequest, Number: 42,
		HeadBranch: "feature/work", TargetBranch: "release/next",
		HeadRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "fork/widget", ProviderID: "fork-id", RemoteURL: "https://github.com/fork/widget.git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "upstream/widget", ProviderID: "upstream-id", RemoteURL: "https://github.com/upstream/widget.git",
		},
	}
	if err := target.Validate(); err != nil {
		t.Fatalf("test target invalid: %v", err)
	}
	return "https://github.com/fork/widget.git", models.PRBase{Target: target, OID: upstreamBaseOID},
		prHeadOID, upstreamBaseOID, forkMainOID, forkTargetOID
}

func configureMaterializationCommitter(t *testing.T, directory string) {
	t.Helper()
	materializationTestGit(t, directory, "config", "user.email", "test@example.com")
	materializationTestGit(t, directory, "config", "user.name", "Test User")
	materializationTestGit(t, directory, "config", "commit.gpgsign", "false")
}

func writeMaterializationTestFile(t *testing.T, directory, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func materializationTestGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
