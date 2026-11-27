package service

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/auth/authn"
	"testing"

	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type sessionSummaryFallbackRepository struct{ repository.SessionRepository }

func TestSessionSummaryListPreservesTaskAuthorization(t *testing.T) {
	for _, narrow := range []bool{true, false} {
		name := "narrow"
		if !narrow {
			name = "fallback"
		}
		t.Run(name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			seedScopedWorkspaces(t, repo)
			if _, err := repo.DB().ExecContext(t.Context(), "UPDATE workspaces SET org_id = ? WHERE id = ?", "owner-org", "ws-b"); err != nil {
				t.Fatal(err)
			}
			if !narrow {
				svc.sessions = sessionSummaryFallbackRepository{SessionRepository: repo}
			}
			for _, tc := range []struct {
				name   string
				ctx    context.Context
				taskID string
				denied bool
			}{
				{"foreign private workspace", ctxAs("user-a"), "task-b", true},
				{"owner from another organization", authn.WithIdentity(t.Context(), authn.Identity{UserID: "user-b", Role: authn.RoleMember, OrgID: "foreign-org"}), "task-b", true},
				{"owner in own organization", authn.WithIdentity(t.Context(), authn.Identity{UserID: "user-b", Role: authn.RoleMember, OrgID: "owner-org"}), "task-b", false},
				{"missing task with identity", ctxAs("user-b"), "missing-task", true},
				{"owner", ctxAs("user-b"), "task-b", false},
				{"internal", context.Background(), "task-b", false},
				{"synthetic", ctxSynthetic(), "task-b", false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					sessions, err := svc.ListTaskSessionSummaryObservations(tc.ctx, tc.taskID)
					if tc.denied {
						if !errors.Is(err, repoerrors.ErrTaskNotFound) || len(sessions) != 0 {
							t.Fatalf("unauthorized/missing task disclosed session rows: sessions=%+v err=%v", sessions, err)
						}
						return
					}
					if err != nil || len(sessions) != 1 || sessions[0].ID != "sess-b" {
						t.Fatalf("allowed session list: sessions=%+v err=%v", sessions, err)
					}
				})
			}
			sessions, err := svc.ListTaskSessionSummaryObservations(context.Background(), "missing-task")
			if err != nil || len(sessions) != 0 {
				t.Fatalf("unscoped missing task changed behavior: %+v %v", sessions, err)
			}
		})
	}
}
