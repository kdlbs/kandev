package process

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestPushPreflightHistoryClassification(t *testing.T) {
	repoDir, originDir, _, binding, _, providerTwo, localHead := setupDivergedContributionRepo(t)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	operator.setRemoteContribution(binding)

	result, err := operator.PushPreflight(context.Background(), PushOptions{})
	if err != nil {
		t.Fatalf("PushPreflight returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("PushPreflight = %+v, want history-only rejection", result)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal preflight result: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatalf("decode preflight result: %v", err)
	}
	if got := body["preflight_reason"]; got != "history_update_required" {
		t.Fatalf("preflight_reason = %v, want history_update_required", got)
	}

	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD")); got != localHead {
		t.Fatalf("local HEAD changed during preflight: %q != %q", got, localHead)
	}
	if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/feature/contribution")); got != providerTwo {
		t.Fatalf("remote HEAD changed during preflight: %q != %q", got, providerTwo)
	}
}

func TestClassifyPushPreflightHistoryUpdate(t *testing.T) {
	const destination = "refs/heads/feature/remote"
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "fetch first",
			output: "! HEAD:refs/heads/feature/remote refs/heads/feature/remote [rejected] (fetch first)",
			want:   true,
		},
		{
			name:   "non fast forward",
			output: "! HEAD:refs/heads/feature/remote refs/heads/feature/remote [rejected] (non-fast-forward)",
			want:   true,
		},
		{
			name:   "wrong destination",
			output: "! HEAD:refs/heads/other refs/heads/other [rejected] (fetch first)",
		},
		{
			name:   "remote rejected",
			output: "! HEAD:refs/heads/feature/remote refs/heads/feature/remote [rejected] (remote rejected)",
		},
		{
			name: "mixed failures",
			output: strings.Join([]string{
				"! HEAD:refs/heads/feature/remote refs/heads/feature/remote [rejected] (fetch first)",
				"! HEAD:refs/heads/other refs/heads/other [rejected] (remote rejected)",
			}, "\n"),
		},
		{
			name:   "malformed status",
			output: "! [rejected] (fetch first)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyPushPreflightHistoryUpdate(tt.output, destination); got != tt.want {
				t.Fatalf("classifyPushPreflightHistoryUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPushPreflightValidatesOriginWithoutTarget(t *testing.T) {
	_, originDir, _, operator := setupPushRemotesRepo(t)

	result, err := operator.PushPreflight(context.Background(), PushOptions{})
	if err != nil {
		t.Fatalf("PushPreflight() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("PushPreflight() = %+v, want success", result)
	}
	// It reports what it validated rather than a blanket success.
	if result.PushedRemote != "origin" || result.PushedBranch != "feature/work" {
		t.Errorf("validated (%q, %q), want (origin, feature/work)", result.PushedRemote, result.PushedBranch)
	}
	if got := remoteBranchSHA(t, originDir, "feature/work"); got != "" {
		t.Errorf("preflight published %q, want a dry run", got)
	}
}

func TestPushPreflightValidatesNamedRemote(t *testing.T) {
	_, originDir, backupDir, operator := setupPushRemotesRepo(t)

	result, err := operator.PushPreflight(context.Background(), PushOptions{
		Remote:         "backup",
		ExpectedBranch: "feature/work",
	})
	if err != nil {
		t.Fatalf("PushPreflight() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("PushPreflight() = %+v, want success", result)
	}
	if result.PushedRemote != "backup" || result.PushedBranch != "feature/work" {
		t.Errorf("validated (%q, %q), want (backup, feature/work)", result.PushedRemote, result.PushedBranch)
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != "" {
		t.Errorf("preflight published %q to backup, want a dry run", got)
	}
	if got := remoteBranchSHA(t, originDir, "feature/work"); got != "" {
		t.Errorf("preflight touched origin (%q)", got)
	}
}

func TestPushPreflightReportsValidatedRemoteAndBranch(t *testing.T) {
	_, _, _, operator := setupPushRemotesRepo(t)

	// Both with and without an explicit target, a non-routed preflight reports
	// the destination it checked.
	for _, opts := range []PushOptions{{}, {Remote: "backup"}} {
		result, err := operator.PushPreflight(context.Background(), opts)
		if err != nil {
			t.Fatalf("PushPreflight(%+v) error = %v", opts, err)
		}
		if !result.Success {
			t.Fatalf("PushPreflight(%+v) = %+v, want success", opts, result)
		}
		if result.PushedRemote == "" || result.PushedBranch == "" {
			t.Errorf("PushPreflight(%+v) reported (%q, %q), want both populated",
				opts, result.PushedRemote, result.PushedBranch)
		}
	}
}

func TestPushPreflightReportsNoRemoteConfigured(t *testing.T) {
	root := t.TempDir()
	isolateTestGitEnv(t)
	repoDir := filepath.Join(root, "repo")
	runGit(t, root, "init", "-b", "main", repoDir)
	runGit(t, repoDir, "config", "user.name", "Task User")
	runGit(t, repoDir, "config", "user.email", "task@example.com")
	writeFile(t, repoDir, "seed.txt", "seed\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "seed")
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)

	result, err := operator.PushPreflight(context.Background(), PushOptions{})
	if err != nil {
		t.Fatalf("PushPreflight() error = %v", err)
	}
	if result.Success {
		t.Fatalf("PushPreflight() = %+v, want a refusal", result)
	}
	if result.ErrorCode != pushNoRemoteConfiguredErrorCode {
		t.Errorf("ErrorCode = %q, want %q", result.ErrorCode, pushNoRemoteConfiguredErrorCode)
	}
}

func TestPushPreflightMutatesNothing(t *testing.T) {
	repoDir, originDir, backupDir, operator := setupPushRemotesRepo(t)
	headBefore := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	branchesBefore := runGit(t, repoDir, "branch", "--all")
	upstreamBefore := operator.getUpstreamRef(context.Background())
	originMain := remoteBranchSHA(t, originDir, "main")

	if _, err := operator.PushPreflight(context.Background(), PushOptions{Remote: "backup"}); err != nil {
		t.Fatalf("PushPreflight() error = %v", err)
	}

	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD")); got != headBefore {
		t.Errorf("HEAD moved from %q to %q", headBefore, got)
	}
	if got := runGit(t, repoDir, "branch", "--all"); got != branchesBefore {
		t.Errorf("local refs changed:\n%s\nwant:\n%s", got, branchesBefore)
	}
	if got := operator.getUpstreamRef(context.Background()); got != upstreamBefore {
		t.Errorf("upstream changed from %q to %q", upstreamBefore, got)
	}
	if got := remoteBranchSHA(t, originDir, "main"); got != originMain {
		t.Errorf("origin main moved from %q to %q", originMain, got)
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != "" {
		t.Errorf("backup gained %q", got)
	}
}

func TestPushPreflightReportsRemoteRefusalOutput(t *testing.T) {
	repoDir, _, backupDir, operator := setupPushRemotesRepo(t)
	// Give backup a history the local branch does not contain, so the write is
	// a non-fast-forward the remote refuses.
	runGit(t, repoDir, "checkout", "-b", "diverged")
	writeFile(t, repoDir, "diverged.txt", "diverged\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "diverged")
	runGit(t, repoDir, "push", "backup", "HEAD:refs/heads/feature/work")
	runGit(t, repoDir, "checkout", "feature/work")
	before := remoteBranchSHA(t, backupDir, "feature/work")

	result, err := operator.PushPreflight(context.Background(), PushOptions{Remote: "backup"})
	if err != nil {
		t.Fatalf("PushPreflight() returned a transport error rather than a result: %v", err)
	}
	if result.Success {
		t.Fatalf("PushPreflight() = %+v, want the refusal reported", result)
	}
	if result.Error == "" && result.Output == "" {
		t.Error("refusal reported neither an error nor the remote's output")
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != before {
		t.Errorf("backup moved from %q to %q", before, got)
	}
}

func TestPushPreflightRedactsCredentialsInRemoteOutput(t *testing.T) {
	repoDir, _, _, operator := setupPushRemotesRepo(t)
	const secret = "s3cr3t-token"
	runGit(t, repoDir, "remote", "add", "secured", "https://user:"+secret+"@example.invalid/repo.git")

	result, err := operator.PushPreflight(context.Background(), PushOptions{Remote: "secured"})
	if err != nil {
		t.Fatalf("PushPreflight() error = %v", err)
	}
	if result.Success {
		t.Fatal("preflight against an unreachable host succeeded")
	}
	if strings.Contains(result.Output, secret) || strings.Contains(result.Error, secret) {
		t.Errorf("credential leaked: output=%q error=%q", result.Output, result.Error)
	}
}

func TestPushPreflightAppliesSameRefusalOrdering(t *testing.T) {
	_, _, backupDir, operator := setupPushRemotesRepo(t)

	for _, tc := range []struct {
		name string
		opts PushOptions
		want string
	}{
		{name: "upstream unsupported", opts: PushOptions{Remote: "backup", SetUpstream: true}, want: pushRemoteUpstreamUnsupportedErrorCode},
		{name: "invalid expected branch", opts: PushOptions{ExpectedBranch: "bad..branch"}, want: pushBranchInvalidErrorCode},
		{name: "unknown remote name", opts: PushOptions{Remote: "missing"}, want: pushRemoteNotFoundErrorCode},
		{name: "unmatched remote url", opts: PushOptions{Remote: "https://example.test/nope.git"}, want: pushRemoteURLUnmatchedErrorCode},
		{name: "branch mismatch", opts: PushOptions{ExpectedBranch: "feature/other"}, want: pushBranchMismatchErrorCode},
		{name: "invalid branch outranks resolution", opts: PushOptions{Remote: "missing", ExpectedBranch: "bad..branch"}, want: pushBranchInvalidErrorCode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := operator.PushPreflight(context.Background(), tc.opts)
			if err != nil {
				t.Fatalf("PushPreflight() error = %v", err)
			}
			if result.ErrorCode != tc.want {
				t.Errorf("ErrorCode = %q, want %q", result.ErrorCode, tc.want)
			}
			// Preflight publishes no baseline, so this code is unreachable.
			if result.ErrorCode == pushBranchMismatchAfterBaselineErrorCode {
				t.Error("preflight reported a post-baseline mismatch")
			}
		})
	}
	if got := remoteBranchSHA(t, backupDir, "feature/work"); got != "" {
		t.Errorf("backup gained %q during refusals", got)
	}
}
