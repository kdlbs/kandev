package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/gitlab"
)

// A branch that is empty (or whitespace-only, which no git ref can be) must
// never reach AutoLinkMRForBranch. PATClient.FindMRByBranch interpolates it
// into `?source_branch=&state=opened&per_page=1`, a query with no effective
// source-branch filter, so GitLab answers with an arbitrary open merge
// request of the project and the wrong MR gets linked to the task.
// CheckSessionMR has always refused an empty branch; the push path must too.
func TestDetectPushAndAssociateMR_IgnoresBlankBranch(t *testing.T) {
	for _, branch := range []string{"", "   ", "\t\n"} {
		t.Run("branch="+branch, func(t *testing.T) {
			svc, fake := seedGitLabSessionWithRepo(t)
			fake.autoLinkFunc = func(_ context.Context, _, _, _, _, _, b string) (*gitlab.TaskMR, error) {
				t.Fatalf("AutoLinkMRForBranch called with blank branch %q", b)
				return nil, nil
			}

			svc.detectPushAndAssociateMR(context.Background(), "s1", "t1", "myproj", branch)

			if fake.autoLinkCalls != 0 {
				t.Fatalf("expected 0 auto-link calls for blank branch, got %d", fake.autoLinkCalls)
			}
		})
	}
}

// AssociateExistingMRByURL — the path behind both Kandev's own Create-MR
// action and manual URL linking — writes gitlab_task_mrs but no watch. Push
// detection must therefore create the missing watch rather than returning as
// soon as it sees an association, otherwise Poller.runMRMonitor has nothing
// to poll and the MR's review/pipeline status never populates on the task.
func TestDetectPushAndAssociateMR_EnsuresWatchForAlreadyLinkedMR(t *testing.T) {
	svc, fake := seedGitLabSessionWithRepo(t)
	fake.taskMRs = map[string][]*gitlab.TaskMR{
		"t1": {{
			TaskID: "t1", RepositoryID: "repo1",
			ProjectPath: "group/myproj", MRIID: 42, HeadBranch: "feat/a",
			State: gitlabMRStateOpen,
		}},
	}

	svc.detectPushAndAssociateMR(context.Background(), "s1", "t1", "myproj", "feat/a")

	if fake.autoLinkCalls != 0 {
		t.Errorf("already-linked MR must not be re-linked, got %d auto-link calls", fake.autoLinkCalls)
	}
	if fake.ensureWatchCalls != 1 {
		t.Fatalf("expected the missing watch to be ensured exactly once, got %d", fake.ensureWatchCalls)
	}
}
