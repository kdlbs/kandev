package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type pushDiscoveryLookup struct {
	pr  *github.PR
	err error
}

type pushDiscoveryCall struct {
	WorkspaceID string
	Owner       string
	Repo        string
	Branch      string
}

type pushDiagnosticsGitHubService struct {
	*mockGitHubService
	lookups []pushDiscoveryLookup
	calls   []pushDiscoveryCall
}

func (s *pushDiagnosticsGitHubService) FindPRByBranchForWorkspace(
	_ context.Context, workspaceID, owner, repo, branch string,
) (*github.PR, error) {
	s.calls = append(s.calls, pushDiscoveryCall{
		WorkspaceID: workspaceID,
		Owner:       owner,
		Repo:        repo,
		Branch:      branch,
	})
	if len(s.lookups) == 0 {
		return nil, nil
	}
	lookup := s.lookups[0]
	s.lookups = s.lookups[1:]
	return lookup.pr, lookup.err
}

func newPushDiagnosticsService(
	t *testing.T, repositoryID, repositoryName, owner, repoName string, watch *github.PRWatch,
) (*Service, *pushDiagnosticsGitHubService, *observer.ObservedLogs) {
	t.Helper()
	ctx := context.Background()
	repository := setupTestRepo(t)
	seedSession(t, repository, "t1", "s1", "step1")
	now := time.Now().UTC()
	if err := repository.CreateRepository(ctx, &models.Repository{
		ID:            repositoryID,
		WorkspaceID:   "ws1",
		Name:          repositoryName,
		SourceType:    "provider",
		Provider:      "github",
		ProviderOwner: owner,
		ProviderName:  repoName,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repository.CreateTaskRepository(ctx, &models.TaskRepository{
		ID:           "task-repository-1",
		TaskID:       "t1",
		RepositoryID: repositoryID,
		Position:     0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	session, err := repository.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.RepositoryID = repositoryID
	if err := repository.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	log, logs := newPushDiagnosticsLogger(t)
	svc := createTestService(repository, newMockStepGetter(), newMockTaskRepo())
	svc.logger = log
	gh := &pushDiagnosticsGitHubService{
		mockGitHubService: &mockGitHubService{prWatch: watch},
	}
	svc.SetGitHubService(gh)
	return svc, gh, logs
}

func newPushDiagnosticsLogger(t *testing.T) (*commonlogger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	return log, logs
}

func installPushDiagnosticsWait(svc *Service, waits *[]time.Duration) {
	svc.prDiscoveryWait = func(ctx context.Context, delay time.Duration) bool {
		*waits = append(*waits, delay)
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}
}

func assertPushDiagnosticLogsContainNoRawError(t *testing.T, logs *observer.ObservedLogs, raw string) {
	t.Helper()
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, raw) || strings.Contains(fmt.Sprint(entry.ContextMap()), raw) {
			t.Fatalf("log exposed raw provider error %q: %#v", raw, entry)
		}
	}
}

func TestPushDiscoveryDiagnostics(t *testing.T) {
	providerErr := errors.New("GET https://token:push-secret@example.invalid/repos/myorg/myrepo: 503 service unavailable")
	providerDeadlineErr := fmt.Errorf("provider request: %w", context.DeadlineExceeded)
	foundPR := &github.PR{
		Number:     17,
		RepoOwner:  "myorg",
		RepoName:   "myrepo",
		HeadBranch: "feature-branch",
	}

	tests := []struct {
		name          string
		lookups       []pushDiscoveryLookup
		wantCalls     int
		wantWaits     []time.Duration
		wantWarns     int
		wantSummary   bool
		wantLastState string
		wantAssoc     int
	}{
		{
			name:          "all empty",
			lookups:       []pushDiscoveryLookup{{}, {}, {}},
			wantCalls:     3,
			wantWaits:     []time.Duration{30 * time.Second, 60 * time.Second},
			wantSummary:   true,
			wantLastState: "empty",
		},
		{
			name:          "all errors",
			lookups:       []pushDiscoveryLookup{{err: providerErr}, {err: providerErr}, {err: providerErr}},
			wantCalls:     3,
			wantWaits:     []time.Duration{30 * time.Second, 60 * time.Second},
			wantWarns:     3,
			wantSummary:   true,
			wantLastState: "failed",
		},
		{
			name:          "wrapped provider deadline remains retryable",
			lookups:       []pushDiscoveryLookup{{err: providerDeadlineErr}, {err: providerDeadlineErr}, {err: providerDeadlineErr}},
			wantCalls:     3,
			wantWaits:     []time.Duration{30 * time.Second, 60 * time.Second},
			wantWarns:     3,
			wantSummary:   true,
			wantLastState: "failed",
		},
		{
			name:          "error then empty",
			lookups:       []pushDiscoveryLookup{{err: providerErr}, {}, {}},
			wantCalls:     3,
			wantWaits:     []time.Duration{30 * time.Second, 60 * time.Second},
			wantWarns:     1,
			wantSummary:   true,
			wantLastState: "empty",
		},
		{
			name:          "empty then error",
			lookups:       []pushDiscoveryLookup{{}, {err: providerErr}, {}},
			wantCalls:     3,
			wantWaits:     []time.Duration{30 * time.Second, 60 * time.Second},
			wantWarns:     1,
			wantSummary:   true,
			wantLastState: "empty",
		},
		{
			name:      "error then found",
			lookups:   []pushDiscoveryLookup{{err: providerErr}, {pr: foundPR}},
			wantCalls: 2,
			wantWaits: []time.Duration{30 * time.Second},
			wantWarns: 1,
			wantAssoc: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, gh, logs := newPushDiagnosticsService(t, "repo1", "myrepo", "myorg", "myrepo", nil)
			gh.lookups = append([]pushDiscoveryLookup(nil), tt.lookups...)
			var waits []time.Duration
			installPushDiagnosticsWait(svc, &waits)

			svc.detectPushAndAssociatePR(context.Background(), "s1", "t1", "", "feature-branch")

			if len(gh.calls) != tt.wantCalls {
				t.Fatalf("lookup calls = %d, want %d", len(gh.calls), tt.wantCalls)
			}
			if len(waits) != len(tt.wantWaits) {
				t.Fatalf("waits = %v, want %v", waits, tt.wantWaits)
			}
			for i := range waits {
				if waits[i] != tt.wantWaits[i] {
					t.Fatalf("waits = %v, want %v", waits, tt.wantWaits)
				}
			}
			if gh.associateCalls != tt.wantAssoc {
				t.Fatalf("association calls = %d, want %d", gh.associateCalls, tt.wantAssoc)
			}
			failed := logs.FilterMessage("PR discovery lookup failed after push").All()
			if len(failed) != tt.wantWarns {
				t.Fatalf("failed lookup warnings = %d, want %d; logs = %v", len(failed), tt.wantWarns, logs.All())
			}
			for _, entry := range failed {
				fields := entry.ContextMap()
				if fields["workspace_id"] != "ws1" || fields["task_id"] != "t1" ||
					fields["repository_id"] != "repo1" || fields["owner"] != "myorg" ||
					fields["repo"] != "myrepo" || fields["branch"] != "feature-branch" ||
					fields["category"] != string(github.PRDiscoveryHealthUnavailable) {
					t.Fatalf("failed lookup fields = %v", fields)
				}
			}
			summary := logs.FilterMessage("exhausted all retries, no PR found after push").All()
			if len(summary) != boolToInt(tt.wantSummary) {
				t.Fatalf("retry summaries = %d, want %d; logs = %v", len(summary), boolToInt(tt.wantSummary), logs.All())
			}
			if tt.wantSummary {
				if summary[0].Level != zap.DebugLevel {
					t.Fatalf("retry summary level = %s, want debug", summary[0].Level)
				}
				fields := summary[0].ContextMap()
				if fields["attempt_count"] != int64(3) || fields["error_count"] != int64(countLookupErrors(tt.lookups)) ||
					fields["empty_count"] != int64(3-countLookupErrors(tt.lookups)) || fields["last_outcome"] != tt.wantLastState {
					t.Fatalf("retry summary fields = %v", fields)
				}
			}
			assertPushDiagnosticLogsContainNoRawError(t, logs, "push-secret")
		})
	}

	t.Run("cancellation before lookup", func(t *testing.T) {
		svc, gh, logs := newPushDiagnosticsService(t, "repo1", "myrepo", "myorg", "myrepo", nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "feature-branch")
		if len(gh.calls) != 0 {
			t.Fatalf("lookup calls = %d, want 0", len(gh.calls))
		}
		if logs.FilterMessage("PR discovery lookup failed after push").Len() != 0 ||
			logs.FilterMessage("exhausted all retries, no PR found after push").Len() != 0 {
			t.Fatalf("cancellation emitted failure logs: %v", logs.All())
		}
	})

	t.Run("cancellation during delay", func(t *testing.T) {
		svc, gh, logs := newPushDiagnosticsService(t, "repo1", "myrepo", "myorg", "myrepo", nil)
		gh.lookups = []pushDiscoveryLookup{{err: providerErr}}
		ctx, cancel := context.WithCancel(context.Background())
		var waits []time.Duration
		svc.prDiscoveryWait = func(_ context.Context, delay time.Duration) bool {
			waits = append(waits, delay)
			cancel()
			return false
		}
		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "feature-branch")
		if len(gh.calls) != 1 || len(waits) != 1 || waits[0] != 30*time.Second {
			t.Fatalf("calls/waits = %d/%v, want 1/[30s]", len(gh.calls), waits)
		}
		if logs.FilterMessage("exhausted all retries, no PR found after push").Len() != 0 {
			t.Fatalf("cancellation emitted exhaustion warning: %v", logs.All())
		}
		assertPushDiagnosticLogsContainNoRawError(t, logs, "push-secret")
	})
}

func TestExistingWatchDiscoveryDiagnostics(t *testing.T) {
	providerErr := errors.New("GET https://token:watch-secret@example.invalid/repos/acme/secondary: 503 service unavailable")
	providerDeadlineErr := fmt.Errorf("provider request: %w", context.DeadlineExceeded)
	foundPR := &github.PR{
		Number:     23,
		RepoOwner:  "acme",
		RepoName:   "secondary",
		HeadBranch: "feature-branch",
	}

	tests := []struct {
		name      string
		lookup    pushDiscoveryLookup
		wantWarn  bool
		wantAssoc int
	}{
		{name: "provider error", lookup: pushDiscoveryLookup{err: providerErr}, wantWarn: true},
		{name: "wrapped provider deadline", lookup: pushDiscoveryLookup{err: providerDeadlineErr}, wantWarn: true},
		{name: "empty result", lookup: pushDiscoveryLookup{}, wantAssoc: 0},
		{name: "found result", lookup: pushDiscoveryLookup{pr: foundPR}, wantAssoc: 1},
		{name: "provider cancellation", lookup: pushDiscoveryLookup{err: context.Canceled}, wantWarn: true, wantAssoc: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			watch := &github.PRWatch{
				ID:           "watch-secondary",
				SessionID:    "s1",
				TaskID:       "t1",
				RepositoryID: "repo-secondary",
				Owner:        "acme",
				Repo:         "secondary",
				PRNumber:     0,
				Branch:       "feature-branch",
			}
			svc, gh, logs := newPushDiagnosticsService(t, "repo-secondary", "secondary", "acme", "secondary", watch)
			gh.lookups = []pushDiscoveryLookup{tt.lookup}

			svc.detectPushAndAssociatePR(context.Background(), "s1", "t1", "", "feature-branch")

			if len(gh.calls) != 1 || gh.calls[0].WorkspaceID != "ws1" || gh.calls[0].Owner != "acme" ||
				gh.calls[0].Repo != "secondary" || gh.calls[0].Branch != "feature-branch" {
				t.Fatalf("lookup identity = %#v, want ws1/acme/secondary/feature-branch", gh.calls)
			}
			if gh.associateCalls != tt.wantAssoc {
				t.Fatalf("association calls = %d, want %d", gh.associateCalls, tt.wantAssoc)
			}
			failed := logs.FilterMessage("PR discovery lookup failed for existing watch").All()
			if len(failed) != boolToInt(tt.wantWarn) {
				t.Fatalf("existing-watch failure warnings = %d, want %d; logs = %v", len(failed), boolToInt(tt.wantWarn), logs.All())
			}
			if tt.wantWarn {
				fields := failed[0].ContextMap()
				if fields["workspace_id"] != "ws1" || fields["task_id"] != "t1" ||
					fields["repository_id"] != "repo-secondary" || fields["owner"] != "acme" ||
					fields["repo"] != "secondary" || fields["category"] != string(github.PRDiscoveryHealthUnavailable) {
					t.Fatalf("existing-watch failure fields = %v", fields)
				}
			}
			if logs.FilterMessage("no PR found for existing watch").Len() > 0 && tt.lookup.err != nil {
				t.Fatalf("failed existing-watch lookup was logged as empty: %v", logs.All())
			}
			if tt.lookup.err == nil && tt.lookup.pr == nil && logs.FilterMessage("no PR found for existing watch").Len() != 1 {
				t.Fatalf("empty existing-watch result did not emit debug log: %v", logs.All())
			}
			assertPushDiagnosticLogsContainNoRawError(t, logs, "watch-secret")
		})
	}
}

func countLookupErrors(lookups []pushDiscoveryLookup) int {
	count := 0
	for _, lookup := range lookups {
		if lookup.err != nil {
			count++
		}
	}
	return count
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
