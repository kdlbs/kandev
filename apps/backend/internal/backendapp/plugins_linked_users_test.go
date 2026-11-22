package backendapp

import (
	"context"
	"testing"

	authhttpmw "github.com/kandev/kandev/internal/auth/httpmw"
)

// With authentication disabled, the only linkable user is the synthetic local
// user. Any other user ID must resolve to false so no receipt is minted for it.
func TestPluginLinkedUserResolverDisabledAuthResolvesOnlyLocalUser(t *testing.T) {
	resolve := pluginLinkedUserResolver(nil, nil)
	local := authhttpmw.SyntheticIdentity()

	human, ok := resolve(context.Background(), local.UserID)
	if !ok {
		t.Fatal("local user did not resolve while authentication is disabled")
	}
	if human.Identity.UserID != local.UserID {
		t.Fatalf("identity user = %q, want %q", human.Identity.UserID, local.UserID)
	}
	if human.DisplayName != "" {
		t.Fatalf("display name = %q without a users service, want empty", human.DisplayName)
	}

	if _, ok := resolve(context.Background(), "someone-else"); ok {
		t.Fatal("a non-local user resolved while authentication is disabled")
	}
}
