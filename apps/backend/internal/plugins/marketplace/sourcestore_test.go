package marketplace

import (
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
)

func newTestSourceStore(t *testing.T) *SourceStore {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := NewSourceStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new source store: %v", err)
	}
	return store
}

func TestEnsureBuiltinIsIdempotentAndListedFirst(t *testing.T) {
	s := newTestSourceStore(t)
	if err := s.EnsureBuiltin(OfficialSourceName, OfficialSourceURL); err != nil {
		t.Fatalf("ensure builtin: %v", err)
	}
	// A second call (e.g. next boot) must not duplicate the row.
	if err := s.EnsureBuiltin(OfficialSourceName, OfficialSourceURL); err != nil {
		t.Fatalf("ensure builtin again: %v", err)
	}
	if _, err := s.Add("Acme", "https://acme.example/index.json"); err != nil {
		t.Fatalf("add: %v", err)
	}

	sources, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("want 2 sources, got %d", len(sources))
	}
	if !sources[0].Builtin || sources[0].Name != OfficialSourceName {
		t.Fatalf("builtin source should be listed first, got %+v", sources[0])
	}
}

func TestEnsureBuiltinRepointsURLButKeepsDisabled(t *testing.T) {
	s := newTestSourceStore(t)
	if err := s.EnsureBuiltin(OfficialSourceName, "https://old.example/index.json"); err != nil {
		t.Fatalf("ensure builtin: %v", err)
	}
	sources, _ := s.List()
	disabled := false
	if _, err := s.Update(sources[0].ID, nil, &disabled); err != nil {
		t.Fatalf("disable builtin: %v", err)
	}
	// Re-seed with a new URL (constant changed): url updates, disabled sticks.
	if err := s.EnsureBuiltin(OfficialSourceName, "https://new.example/index.json"); err != nil {
		t.Fatalf("re-ensure builtin: %v", err)
	}
	got, _ := s.List()
	if got[0].URL != "https://new.example/index.json" {
		t.Fatalf("url not repointed: %q", got[0].URL)
	}
	if got[0].Enabled {
		t.Fatalf("operator-disabled builtin was re-enabled by EnsureBuiltin")
	}
}

func TestAddRejectsBadAndDuplicateURL(t *testing.T) {
	s := newTestSourceStore(t)
	if _, err := s.Add("bad", "ftp://nope"); err == nil {
		t.Fatalf("expected non-http url to be rejected")
	}
	if _, err := s.Add("empty", ""); err == nil {
		t.Fatalf("expected empty url to be rejected")
	}
	if _, err := s.Add("first", "https://dup.example/index.json"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := s.Add("second", "https://dup.example/index.json"); err == nil {
		t.Fatalf("expected duplicate url to be rejected")
	}
}

func TestDeleteBuiltinIsRefused(t *testing.T) {
	s := newTestSourceStore(t)
	_ = s.EnsureBuiltin(OfficialSourceName, OfficialSourceURL)
	sources, _ := s.List()
	if err := s.Delete(sources[0].ID); !errors.Is(err, ErrBuiltinImmutable) {
		t.Fatalf("want ErrBuiltinImmutable, got %v", err)
	}
}

func TestDeleteAndGetUnknownSource(t *testing.T) {
	s := newTestSourceStore(t)
	rec, err := s.Add("Acme", "https://acme.example/index.json")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s.Delete(rec.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(rec.ID); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("want ErrSourceNotFound, got %v", err)
	}
	if err := s.Delete("does-not-exist"); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("want ErrSourceNotFound for unknown delete, got %v", err)
	}
}

func TestUpdateRenamesAndToggles(t *testing.T) {
	s := newTestSourceStore(t)
	rec, _ := s.Add("Acme", "https://acme.example/index.json")
	newName := "Acme Internal"
	disabled := false
	updated, err := s.Update(rec.ID, &newName, &disabled)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != newName || updated.Enabled {
		t.Fatalf("update not applied: %+v", updated)
	}
	got, _ := s.Get(rec.ID)
	if got.Name != newName || got.Enabled {
		t.Fatalf("update not persisted: %+v", got)
	}
}
