package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestCoordinatorPromptAdmissionAndReplayRequireManager(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, true)
	ctx := context.Background()
	for _, origin := range []string{"", models.TaskOriginCoordinator} {
		id := "prompt-review-" + origin
		if err := repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "ws-team", Title: id,
			Origin: origin, State: v1.TaskStateCreated, Priority: "medium"}); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: id + "-session", TaskID: id,
			State: models.TaskSessionStateCreated}); err != nil {
			t.Fatal(err)
		}
		owner := ctxAsRole("user-ana", authn.RoleMember)
		collaborator := ctxAsRole("user-bruno", authn.RoleMember)
		if err := svc.AuthorizeTaskSessionPromptAccess(owner, id, id+"-session"); err != nil {
			t.Fatalf("manager admission: %v", err)
		}
		if err := svc.AuthorizeTaskSessionPromptAccess(owner, id, "session-missing"); err == nil {
			t.Fatal("missing session admitted")
		}
		msg, err := svc.CreateMessageIdempotent(owner, id+"-message", &CreateMessageRequest{
			TaskID: id, TaskSessionID: id + "-session", Content: "manager prompt", AuthorType: "user",
		})
		if err != nil {
			t.Fatal(err)
		}
		admissionErr := svc.AuthorizeTaskSessionPromptAccess(collaborator, id, id+"-session")
		_, found, replayErr := svc.findStoredMessage(collaborator, msg.ID)
		if origin == models.TaskOriginCoordinator {
			if !IsForbidden(admissionErr) || !IsForbidden(replayErr) || found {
				t.Errorf("coordinator collaborator admission=%v replay=%v found=%v", admissionErr, replayErr, found)
			}
		} else if admissionErr != nil || replayErr != nil || !found {
			t.Errorf("ordinary collaborator admission=%v replay=%v found=%v", admissionErr, replayErr, found)
		}
	}
}

func TestOrphanCoordinatorPromptFailsClosedWithoutChangingOrdinaryCleanup(t *testing.T) {
	for _, workspace := range []string{"", "missing-workspace"} {
		t.Run(workspace, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			for _, origin := range []string{"", models.TaskOriginCoordinator} {
				id := "orphan-" + origin
				if err := repo.CreateTask(t.Context(), &models.Task{ID: id, WorkspaceID: workspace,
					Title: id, Origin: origin, State: v1.TaskStateCreated, Priority: "medium"}); err != nil {
					t.Fatal(err)
				}
				err := svc.AuthorizeTaskPromptScope(ctxAs("caller"), id)
				if origin == models.TaskOriginCoordinator && !errors.Is(err, repoerrors.ErrTaskNotFound) {
					t.Errorf("orphan coordinator authorized: %v", err)
				}
				if origin == "" && err != nil {
					t.Errorf("ordinary orphan cleanup changed: %v", err)
				}
				for _, internal := range []context.Context{context.Background(), ctxSynthetic()} {
					if err := svc.AuthorizeTaskPromptScope(internal, id); err != nil {
						t.Errorf("internal or auth-disabled caller changed: %v", err)
					}
				}
			}
		})
	}
}
