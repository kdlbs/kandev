package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

type stubAccounts struct{ users []*models.User }

func (s stubAccounts) ListUsers(context.Context) ([]*models.User, error) { return s.users, nil }

func (s stubAccounts) GetUser(_ context.Context, id string) (*models.User, error) {
	for _, u := range s.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

// The member picker is the one place a person's name is shown to someone who
// has no other relationship with them. Listing it unscoped makes every account
// on the instance enumerable from any tenant, which is the cross-tenant leak
// this filter exists to close.
func TestDirectoryIsScopedToTheCallersOrg(t *testing.T) {
	adapter := &userDirectoryAdapter{accounts: stubAccounts{users: []*models.User{
		{ID: "ada", DisplayName: "Ada", OrgID: "org-a", Status: "active"},
		{ID: "grace", DisplayName: "Grace", OrgID: "org-a", Status: "active"},
		{ID: "eve", DisplayName: "Eve", OrgID: "org-b", Status: "active"},
		{ID: "gone", DisplayName: "Gone", OrgID: "org-a", Status: "disabled"},
	}}}

	got, err := adapter.ListDirectory(context.Background(), "org-a")
	if err != nil {
		t.Fatalf("list directory: %v", err)
	}
	ids := map[string]bool{}
	for _, u := range got {
		ids[u.ID] = true
	}
	if !ids["ada"] || !ids["grace"] {
		t.Fatalf("own organization missing from the directory: %+v", got)
	}
	if ids["eve"] {
		t.Fatal("another organization's account is enumerable through the directory")
	}
	if ids["gone"] {
		t.Fatal("a disabled account is pickable")
	}
}

// With organizations off every account carries an empty org, so the same code
// path must return the whole instance rather than nothing.
func TestDirectoryReturnsEveryoneWhenOrgsAreOff(t *testing.T) {
	adapter := &userDirectoryAdapter{accounts: stubAccounts{users: []*models.User{
		{ID: "ada", DisplayName: "Ada", Status: "active"},
		{ID: "grace", DisplayName: "Grace", Status: "active"},
	}}}

	got, err := adapter.ListDirectory(context.Background(), "")
	if err != nil {
		t.Fatalf("list directory: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("directory = %+v, want both accounts on a single-org instance", got)
	}
}
