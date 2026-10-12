package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

// TestRemoteDockerMutationsRequireAdmin is finding 4 from branch review.
//
// Gating only the bespoke test and build routes left the generic executor and
// profile mutations open, so a member could still create a remote Docker
// profile — which grants effective root on the named host.
func TestRemoteDockerMutationsRequireAdmin(t *testing.T) {
	member := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "user-1", Role: authn.RoleMember,
	})
	admin := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "admin-1", Role: authn.RoleAdmin,
	})

	t.Run("member is refused", func(t *testing.T) {
		if err := requireExecutorTypeAdmin(member, models.ExecutorTypeRemoteDocker); !errors.Is(err, ErrRemoteDockerAdminRequired) {
			t.Fatalf("member gate = %v, want ErrRemoteDockerAdminRequired", err)
		}
	})

	t.Run("anonymous is refused", func(t *testing.T) {
		if err := requireExecutorTypeAdmin(context.Background(), models.ExecutorTypeRemoteDocker); !errors.Is(err, ErrRemoteDockerAdminRequired) {
			t.Fatalf("anonymous gate = %v, want ErrRemoteDockerAdminRequired", err)
		}
	})

	t.Run("admin is allowed", func(t *testing.T) {
		if err := requireExecutorTypeAdmin(admin, models.ExecutorTypeRemoteDocker); err != nil {
			t.Fatalf("admin gate = %v, want nil", err)
		}
	})

	t.Run("a type change into remote docker is gated", func(t *testing.T) {
		// Update passes both the current and target type, so switching an
		// ordinary executor into a remote Docker one is refused as well.
		if err := requireExecutorTypeAdmin(member, models.ExecutorTypeLocalDocker, models.ExecutorTypeRemoteDocker); !errors.Is(err, ErrRemoteDockerAdminRequired) {
			t.Fatalf("type-change gate = %v, want ErrRemoteDockerAdminRequired", err)
		}
	})

	t.Run("unrelated types stay open to members", func(t *testing.T) {
		for _, executorType := range []models.ExecutorType{
			models.ExecutorTypeLocal, models.ExecutorTypeWorktree, models.ExecutorTypeLocalDocker,
		} {
			if err := requireExecutorTypeAdmin(member, executorType); err != nil {
				t.Fatalf("%s gate = %v, want nil; this change must not restrict other executors", executorType, err)
			}
		}
	})

	t.Run("kubernetes keeps its own gate", func(t *testing.T) {
		if err := requireExecutorTypeAdmin(member, models.ExecutorTypeKubernetes); !errors.Is(err, ErrKubernetesAdminRequired) {
			t.Fatalf("kubernetes gate = %v, want ErrKubernetesAdminRequired", err)
		}
	})
}
