package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/notifications/models"
)

// seedOwnedProvider stores one enabled provider for userID and returns it.
func seedOwnedProvider(t *testing.T, repo *sqliteRepository, userID, name string) *models.Provider {
	t.Helper()
	provider := &models.Provider{
		UserID:  userID,
		Name:    name,
		Type:    models.ProviderTypeApprise,
		Config:  map[string]interface{}{"urls": "slack://" + name},
		Enabled: true,
	}
	if err := repo.CreateProvider(context.Background(), provider); err != nil {
		t.Fatalf("create provider for %s: %v", userID, err)
	}
	return provider
}

func newOwnershipTestRepo(t *testing.T) *sqliteRepository {
	t.Helper()
	database := openNotificationTestDB(t)
	repo, err := newSQLiteRepositoryWithDB(context.Background(), database, database)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	return repo
}

func TestGetProviderIsScopedToItsOwner(t *testing.T) {
	ctx := context.Background()
	repo := newOwnershipTestRepo(t)
	owned := seedOwnedProvider(t, repo, "user-a", "a-webhook")

	got, err := repo.GetProvider(ctx, "user-a", owned.ID)
	if err != nil {
		t.Fatalf("owner read: %v", err)
	}
	if got.ID != owned.ID {
		t.Fatalf("owner read = %#v, want provider %s", got, owned.ID)
	}

	foreign, foreignErr := repo.GetProvider(ctx, "user-b", owned.ID)
	missing, missingErr := repo.GetProvider(ctx, "user-b", "does-not-exist")
	if !errors.Is(foreignErr, ErrProviderNotFound) {
		t.Fatalf("foreign read error = %v, want ErrProviderNotFound", foreignErr)
	}
	if !errors.Is(missingErr, ErrProviderNotFound) {
		t.Fatalf("missing read error = %v, want ErrProviderNotFound", missingErr)
	}
	// A guessed ID belonging to someone else must be indistinguishable from
	// an ID that does not exist at all.
	if foreignErr.Error() != missingErr.Error() || foreign != nil || missing != nil {
		t.Fatalf("foreign read (%#v, %v) differs from missing read (%#v, %v)", foreign, foreignErr, missing, missingErr)
	}
}

func TestUpdateProviderRejectsAForeignOwnerWithoutMutating(t *testing.T) {
	ctx := context.Background()
	repo := newOwnershipTestRepo(t)
	owned := seedOwnedProvider(t, repo, "user-a", "a-webhook")

	hijack := &models.Provider{
		ID:      owned.ID,
		UserID:  "user-b",
		Name:    "stolen",
		Type:    models.ProviderTypeApprise,
		Config:  map[string]interface{}{"urls": "slack://attacker"},
		Enabled: false,
	}
	if err := repo.UpdateProvider(ctx, hijack); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("foreign update error = %v, want ErrProviderNotFound", err)
	}

	after, err := repo.GetProvider(ctx, "user-a", owned.ID)
	if err != nil {
		t.Fatalf("reload after rejected update: %v", err)
	}
	if after.Name != "a-webhook" || !after.Enabled || after.Config["urls"] != "slack://a-webhook" {
		t.Fatalf("provider mutated by a foreign update: %#v", after)
	}
}

func TestDeleteProviderRejectsAForeignOwnerWithoutMutating(t *testing.T) {
	ctx := context.Background()
	repo := newOwnershipTestRepo(t)
	owned := seedOwnedProvider(t, repo, "user-a", "a-webhook")

	if err := repo.DeleteProvider(ctx, "user-b", owned.ID); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("foreign delete error = %v, want ErrProviderNotFound", err)
	}
	if _, err := repo.GetProvider(ctx, "user-a", owned.ID); err != nil {
		t.Fatalf("provider deleted by a foreign caller: %v", err)
	}

	if err := repo.DeleteProvider(ctx, "user-b", "does-not-exist"); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("missing delete error = %v, want ErrProviderNotFound", err)
	}
	if err := repo.DeleteProvider(ctx, "user-a", owned.ID); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	if _, err := repo.GetProvider(ctx, "user-a", owned.ID); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("owner delete left the row behind: %v", err)
	}
}

func TestListProvidersByUserSeesOnlyItsOwnRows(t *testing.T) {
	ctx := context.Background()
	repo := newOwnershipTestRepo(t)
	seedOwnedProvider(t, repo, "user-a", "a-webhook")
	seedOwnedProvider(t, repo, "user-b", "b-webhook")

	for userID, want := range map[string]string{"user-a": "a-webhook", "user-b": "b-webhook"} {
		providers, err := repo.ListProvidersByUser(ctx, userID)
		if err != nil {
			t.Fatalf("list for %s: %v", userID, err)
		}
		if len(providers) != 1 || providers[0].Name != want {
			t.Fatalf("%s sees %#v, want only %s", userID, providers, want)
		}
	}
}

func TestListProviderUserIDsReturnsEachOwnerOnce(t *testing.T) {
	ctx := context.Background()
	repo := newOwnershipTestRepo(t)
	seedOwnedProvider(t, repo, "user-a", "a-webhook")
	seedOwnedProvider(t, repo, "user-a", "a-second")
	seedOwnedProvider(t, repo, "user-b", "b-webhook")

	owners, err := repo.ListProviderUserIDs(ctx)
	if err != nil {
		t.Fatalf("list provider owners: %v", err)
	}
	if len(owners) != 2 || owners[0] != "user-a" || owners[1] != "user-b" {
		t.Fatalf("provider owners = %#v, want [user-a user-b]", owners)
	}
}
