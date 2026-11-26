package plugins

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
)

func themeAdmin(id string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: id, Role: authn.RoleAdmin})
}

func TestThemeReviewCommitIdentityAndReplay(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := themeAdmin("one")
	review, err := svc.PrepareTheme(ctx, []byte(`{"name":"Sample","type":"dark","colors":{"editor.background":"#272822"}}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(svc.ThemeInventory().Themes) != 0 {
		t.Fatal("prepare installed theme")
	}
	if _, err = svc.CommitTheme(themeAdmin("two"), review.ID); err == nil {
		t.Fatal("different principal committed")
	}
	rec, err := svc.CommitTheme(ctx, review.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.CommitTheme(ctx, review.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.InstallationID != again.InstallationID || len(svc.ThemeInventory().Themes) != 1 {
		t.Fatal("commit duplicated install")
	}
	inventory := svc.ThemeInventory()
	if err = svc.Disable(rec.ID); err != nil {
		t.Fatal(err)
	}
	if len(svc.ThemeInventory().Themes) != 0 || svc.ThemeInventory().Revision <= inventory.Revision {
		t.Fatal("disable did not revoke inventory")
	}
	if err = svc.Enable(rec.ID); err != nil {
		t.Fatal(err)
	}
}

func TestThemeReviewExpiry(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := themeAdmin("one")
	review, err := svc.PrepareTheme(ctx, []byte(`{"type":"dark"}`), "")
	if err != nil {
		t.Fatal(err)
	}
	svc.themeReviews.entries[review.ID].expires = time.Now().Add(-time.Second)
	if _, err = svc.CommitTheme(ctx, review.ID); err == nil {
		t.Fatal("expired review committed")
	}
}

func TestThemeReviewRejectsChangedPredecessor(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := themeAdmin("one")
	data := []byte(`{"type":"dark"}`)
	first, err := svc.PrepareTheme(ctx, data, "")
	if err != nil {
		t.Fatal(err)
	}
	installed, err := svc.CommitTheme(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.PrepareTheme(ctx, data, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Disable(installed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.CommitTheme(ctx, second.ID); !errors.Is(err, ErrThemeReviewConflict) {
		t.Fatalf("stale commit: %v", err)
	}
}

func TestThemeReviewLimitAndDuplicateImport(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := themeAdmin("one")
	data := []byte(`{"type":"dark"}`)
	first, err := svc.PrepareTheme(ctx, data, "")
	if err != nil {
		t.Fatal(err)
	}
	installed, err := svc.CommitTheme(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.PrepareTheme(ctx, data, "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.CommitTheme(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if installed.InstallationID != again.InstallationID {
		t.Fatal("duplicate source got new installation")
	}
	for i := 0; i < 6; i++ {
		if _, err = svc.PrepareTheme(ctx, data, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = svc.PrepareTheme(ctx, data, ""); err == nil {
		t.Fatal("review quota not enforced")
	}
}
