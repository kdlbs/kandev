package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	userstore "github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

// brokenTasks fails the lookups the recipient resolution depends on, standing
// in for a deleted task, a workspace-deletion race, or a transient database
// error.
type brokenTasks struct {
	task         *taskmodels.Task
	taskErr      error
	workspace    *taskmodels.Workspace
	workspaceErr error
}

func (b brokenTasks) GetTask(context.Context, string) (*taskmodels.Task, error) {
	return b.task, b.taskErr
}

func (b brokenTasks) GetWorkspace(context.Context, string) (*taskmodels.Workspace, error) {
	return b.workspace, b.workspaceErr
}

// seedAdminAndMember gives the default administrator a provider alongside a
// regular member's. A recipient resolution that falls open lands on the
// administrator, so this is the account the assertions watch.
func seedAdminAndMember(repo *multiUserRepository) {
	repo.seedProvider(userstore.DefaultUserID, "slack://admin", EventTaskSessionClarificationAsked, EventOfficeInboxItem)
	repo.seedProvider("user-a", "slack://user-a", EventTaskSessionClarificationAsked, EventOfficeInboxItem)
}

func TestUnresolvableOwnerDeliversNothingUnderEnforcedAuth(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tasks TaskContextReader
	}{
		{"workspace lookup fails", brokenTasks{
			task:         &taskmodels.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			workspaceErr: errors.New("database unavailable"),
		}},
		{"workspace vanished", brokenTasks{
			task: &taskmodels.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		}},
		{"task lookup fails", brokenTasks{taskErr: errors.New("database unavailable")}},
		{"task vanished", brokenTasks{}},
		{"task carries no workspace", brokenTasks{task: &taskmodels.Task{ID: "task-1"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMultiUserRepository()
			seedAdminAndMember(repo)
			svc, capture := newOwnershipTestService(t, repo, tc.tasks, true)

			svc.HandleClarificationRequested(context.Background(), "task-1", "session-1", "pending-1")

			if len(capture.messages) != 0 {
				t.Fatalf("delivered %#v, want nothing: an unresolvable owner must not fall back to the administrator", webhooksOf(capture.messages))
			}
			if len(repo.deliveries) != 0 {
				t.Fatalf("recorded deliveries %#v, want none", repo.deliveries)
			}
		})
	}
}

func TestNotificationWithoutATaskIsDroppedUnderEnforcedAuth(t *testing.T) {
	repo := newMultiUserRepository()
	seedAdminAndMember(repo)
	svc, capture := newOwnershipTestService(t, repo, ownedWorkspaceTasks{workspaceID: "workspace-a", ownerID: "user-a"}, true)

	// An event whose payload carried no task_id names no workspace, so there
	// is no owner to deliver it to.
	svc.HandleClarificationRequested(context.Background(), "", "session-1", "pending-1")

	if len(capture.messages) != 0 {
		t.Fatalf("delivered %#v, want nothing", webhooksOf(capture.messages))
	}
}

func TestInboxItemWithoutAWorkspaceIsDroppedUnderEnforcedAuth(t *testing.T) {
	repo := newMultiUserRepository()
	seedAdminAndMember(repo)
	svc, capture := newOwnershipTestService(t, repo, ownedWorkspaceTasks{workspaceID: "workspace-a", ownerID: "user-a"}, true)

	svc.HandleInboxItem(context.Background(), "", "approval", "Deploy to production")

	if len(capture.messages) != 0 {
		t.Fatalf("delivered %#v, want nothing", webhooksOf(capture.messages))
	}
}

func TestUnownedWorkspaceStillResolvesToTheDefaultUser(t *testing.T) {
	repo := newMultiUserRepository()
	seedAdminAndMember(repo)
	// The workspace loads successfully and is explicitly unowned: a pre-auth
	// row the setup wizard has not claimed yet, not a failed lookup.
	svc, capture := newOwnershipTestService(t, repo, brokenTasks{
		task:      &taskmodels.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		workspace: &taskmodels.Workspace{ID: "workspace-1"},
	}, true)

	svc.HandleClarificationRequested(context.Background(), "task-1", "session-1", "pending-1")

	got := webhooksOf(capture.messages)
	if len(got) != 1 || got[0] != "slack://admin" {
		t.Fatalf("delivered to %#v, want the default user's provider", got)
	}
}

func TestUnresolvableOwnerStillFallsBackWhenAuthIsDisabled(t *testing.T) {
	repo := newMultiUserRepository()
	seedAdminAndMember(repo)
	// Same failure as the enforced case above, but with one account on the
	// instance there is nobody else it could wrongly reach.
	svc, capture := newOwnershipTestService(t, repo, brokenTasks{
		taskErr: errors.New("database unavailable"),
	}, false)

	svc.HandleClarificationRequested(context.Background(), "task-1", "session-1", "pending-1")

	got := webhooksOf(capture.messages)
	if len(got) != 1 || got[0] != "slack://admin" {
		t.Fatalf("delivered to %#v, want the single-user default to be preserved", got)
	}
}

func TestNilAuthEnforcementBehavesAsAuthDisabled(t *testing.T) {
	repo := newMultiUserRepository()
	seedAdminAndMember(repo)
	t.Setenv(desktopNativeNotificationsEnv, "true")
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	// A nil predicate is the pre-auth single-user install.
	svc := NewService(repo, brokenTasks{taskErr: errors.New("database unavailable")}, nil, log, nil)
	capture := &captureProvider{}
	svc.providers[models.ProviderTypeLocal] = capture

	svc.HandleClarificationRequested(context.Background(), "task-1", "session-1", "pending-1")

	if got := webhooksOf(capture.messages); len(got) != 1 || got[0] != "slack://admin" {
		t.Fatalf("delivered to %#v, want the single-user default", got)
	}
}

func TestTestNotificationIsAddressedToTheCaller(t *testing.T) {
	repo := newMultiUserRepository()
	owned := repo.seedProvider("user-a", "slack://user-a", EventTaskSessionClarificationAsked)
	svc, capture := newOwnershipTestService(t, repo, ownedWorkspaceTasks{workspaceID: "workspace-a", ownerID: "user-a"}, true)

	if err := svc.TestProvider(context.Background(), "user-a", owned.ID); err != nil {
		t.Fatalf("test provider: %v", err)
	}
	if len(capture.messages) != 1 {
		t.Fatalf("messages = %#v, want one test notification", capture.messages)
	}
	// The local provider broadcasts to Message.UserID; an empty one never
	// reaches the tab that asked for the test.
	if capture.messages[0].UserID != "user-a" {
		t.Fatalf("test notification addressed to %q, want user-a", capture.messages[0].UserID)
	}
}
