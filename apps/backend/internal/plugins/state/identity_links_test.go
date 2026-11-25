package state

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
)

func newIdentityLinkTestStore(t *testing.T) *CommandStore {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := NewCommandStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	return store
}

func TestIdentityLinkReplacesSameSubjectAndSameUser(t *testing.T) {
	store := newIdentityLinkTestStore(t)
	ctx := context.Background()
	first, err := store.LinkIdentity(ctx, IdentityLink{InstallationID: "inst-1", Provider: "discord", Subject: "d-1", UserID: "alice"})
	if err != nil {
		t.Fatalf("link alice: %v", err)
	}
	if _, err := store.LinkIdentity(ctx, IdentityLink{InstallationID: "inst-2", Provider: "discord", Subject: "d-1", UserID: "bob"}); err != nil {
		t.Fatalf("link in another installation: %v", err)
	}
	// The same Discord account relinked to bob replaces alice's link.
	relinked, err := store.LinkIdentity(ctx, IdentityLink{InstallationID: "inst-1", Provider: "discord", Subject: "d-1", UserID: "bob"})
	if err != nil {
		t.Fatalf("relink subject: %v", err)
	}
	if _, err := store.GetIdentityLink(ctx, "inst-1", first.ID); !errors.Is(err, ErrIdentityLinkNotFound) {
		t.Fatalf("replaced link lookup err = %v, want not found", err)
	}
	// bob linking a second Discord account keeps one link per user.
	if _, err := store.LinkIdentity(ctx, IdentityLink{InstallationID: "inst-1", Provider: "discord", Subject: "d-2", UserID: "bob"}); err != nil {
		t.Fatalf("link second subject: %v", err)
	}
	if _, err := store.FindIdentityLink(ctx, "inst-1", "discord", relinked.Subject); !errors.Is(err, ErrIdentityLinkNotFound) {
		t.Fatalf("old subject lookup err = %v, want not found", err)
	}
	links, err := store.ListIdentityLinks(ctx, "inst-1", "")
	if err != nil || len(links) != 1 || links[0].Subject != "d-2" || links[0].UserID != "bob" {
		t.Fatalf("installation links = %+v err=%v, want only bob's d-2", links, err)
	}
	other, err := store.ListIdentityLinks(ctx, "inst-2", "")
	if err != nil || len(other) != 1 {
		t.Fatalf("other installation links = %+v err=%v, want untouched", other, err)
	}

	if err := store.DeleteInstallationIdentityLinks(ctx, "inst-1"); err != nil {
		t.Fatalf("delete installation links: %v", err)
	}
	if links, _ := store.ListIdentityLinks(ctx, "inst-1", ""); len(links) != 0 {
		t.Fatalf("links after uninstall = %+v, want none", links)
	}
}

func TestDelegatedResponseConsumesOnce(t *testing.T) {
	store := newIdentityLinkTestStore(t)
	ctx := context.Background()
	record := DelegatedResponse{
		ReceiptID: "receipt-1", InstallationID: "inst-1", WorkspaceID: "ws-1", LinkID: "link-1", UserID: "alice",
		UserDisplayName: "Alice", Provider: "discord", Subject: "d-1", Kind: "permission", TargetID: "interaction-1",
		TargetVersion: "v1", PayloadDigest: "sha256:x", IssuedAt: "2026-11-22T10:00:00Z", ExpiresAt: "2026-11-22T10:05:00Z",
	}
	if err := store.RecordDelegatedResponse(ctx, record); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := store.ConsumedDelegatedResponse(ctx, "interaction-1"); !errors.Is(err, ErrDelegatedResponseNotFound) {
		t.Fatalf("unconsumed attribution err = %v, want not found", err)
	}
	if err := store.MarkDelegatedResponseConsumed(ctx, "receipt-1"); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if err := store.MarkDelegatedResponseConsumed(ctx, "receipt-1"); !errors.Is(err, ErrDelegatedResponseNotFound) {
		t.Fatalf("second consume err = %v, want not found", err)
	}
	got, err := store.ConsumedDelegatedResponse(ctx, "interaction-1")
	if err != nil || got.UserID != "alice" || got.Provider != "discord" || got.ConsumedAt == "" {
		t.Fatalf("attribution = %+v err=%v", got, err)
	}
}
