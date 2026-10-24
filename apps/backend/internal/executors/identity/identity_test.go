package identity

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestExecutorCreatorIdentityPrecedence(t *testing.T) {
	user := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user"})
	synthetic := authn.WithIdentity(context.Background(), authn.Identity{UserID: "synthetic", Synthetic: true})
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"internal", context.Background(), ""},
		{"authenticated", user, "user"},
		{"bound actor", taskmodels.WithExecutorActor(user, "actor"), "actor"},
		{"explicit shared actor", taskmodels.WithExecutorActor(user, ""), ""},
		{"synthetic", synthetic, ""},
		{"synthetic ignores actor", taskmodels.WithExecutorActor(synthetic, "actor"), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CreatorUserID(tc.ctx); got != tc.want {
				t.Fatalf("creator = %q, want %q", got, tc.want)
			}
		})
	}
}
