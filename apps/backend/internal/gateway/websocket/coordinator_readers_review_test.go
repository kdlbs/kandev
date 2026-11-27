package websocket

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
)

func TestCoordinatorNotificationUsesReadableWorkspaceRecipientsAndFailsClosed(t *testing.T) {
	for _, mode := range []string{"shared", "empty", "read_failed"} {
		t.Run(mode, func(t *testing.T) {
			hub, eventBus, _ := newCoordinatorBroadcastHub(t, true)
			hub.setAuthPolicy(AuthPolicy{
				Enforced:       func() bool { return true },
				WorkspaceOwner: func(context.Context, string) (string, error) { return "owner", nil },
				WorkspaceReaders: func(context.Context, string) ([]string, error) {
					if mode == "read_failed" {
						return nil, errors.New("membership read failed")
					}
					if mode == "empty" {
						return nil, nil
					}
					return []string{"owner", "member", "org-reader"}, nil
				},
			})
			clients := map[string]*Client{}
			for _, id := range []string{"owner", "member", "org-reader", "foreign"} {
				clients[id] = registerAccessClient(t, hub, id, authn.Identity{UserID: id, Role: authn.RoleMember})
			}
			publishRealCoordinatorUpdated(t, eventBus)
			if mode == "shared" {
				for _, id := range []string{"owner", "member", "org-reader"} {
					waitForMessage(t, clients[id])
				}
			}
			for id, client := range clients {
				if mode == "shared" && id != "foreign" {
					continue
				}
				if got := receivedActions(client); len(got) != 0 {
					t.Fatalf("%s routed coordinator data to %s: %v", mode, id, got)
				}
			}
		})
	}
}
