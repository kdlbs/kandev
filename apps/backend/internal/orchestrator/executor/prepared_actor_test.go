package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
)

func TestRequirePreparedLaunchActor(t *testing.T) {
	withIdentity := func(identity authn.Identity) context.Context {
		return authn.WithIdentity(context.Background(), identity)
	}
	cases := []struct {
		name    string
		ctx     context.Context
		require bool
		wantErr bool
	}{
		{name: "not required without identity", ctx: context.Background()},
		{name: "required without identity", ctx: context.Background(), require: true, wantErr: true},
		{name: "required with empty user", ctx: withIdentity(authn.Identity{Role: authn.RoleMember}), require: true, wantErr: true},
		{name: "required with user", ctx: withIdentity(authn.Identity{UserID: "user-a", Role: authn.RoleMember}), require: true},
		{name: "required with synthetic identity", ctx: withIdentity(authn.Identity{Synthetic: true, Role: authn.RoleAdmin}), require: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := requirePreparedLaunchActor(tc.ctx, LaunchOptions{RequireExecutorActor: tc.require})
			if tc.wantErr != errors.Is(err, ErrPreparedLaunchActorUnavailable) {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
