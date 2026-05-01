package secretadapter

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/secrets"
)

// fakeStore is a minimal in-memory secrets.SecretStore for adapter tests. We
// only implement the methods the adapter actually calls; everything else
// fails the test if invoked, which catches accidental dependency creep.
type fakeStore struct {
	rows    map[string]string
	getErr  error
	failOps map[string]error
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[string]string{}, failOps: map[string]error{}}
}

func (f *fakeStore) Create(_ context.Context, s *secrets.SecretWithValue) error {
	if err := f.failOps["create"]; err != nil {
		return err
	}
	if _, exists := f.rows[s.ID]; exists {
		return errors.New("constraint violation")
	}
	f.rows[s.ID] = s.Value
	return nil
}

func (f *fakeStore) Get(_ context.Context, id string) (*secrets.Secret, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if _, ok := f.rows[id]; !ok {
		return nil, errors.New("secret not found: " + id)
	}
	return &secrets.Secret{ID: id}, nil
}

func (f *fakeStore) Reveal(_ context.Context, id string) (string, error) {
	v, ok := f.rows[id]
	if !ok {
		return "", errors.New("secret not found: " + id)
	}
	return v, nil
}

func (f *fakeStore) Update(_ context.Context, id string, req *secrets.UpdateSecretRequest) error {
	if err := f.failOps["update"]; err != nil {
		return err
	}
	if _, ok := f.rows[id]; !ok {
		return errors.New("secret not found: " + id)
	}
	if req.Value != nil {
		f.rows[id] = *req.Value
	}
	return nil
}

func (f *fakeStore) Delete(_ context.Context, id string) error {
	delete(f.rows, id)
	return nil
}

func (f *fakeStore) List(_ context.Context) ([]*secrets.SecretListItem, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeStore) Close() error { return nil }

func TestAdapter_Set_CreatesWhenMissing(t *testing.T) {
	store := newFakeStore()
	a := New(store)

	if err := a.Set(context.Background(), "k1", "name", "v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := store.rows["k1"]; got != "v1" {
		t.Errorf("expected value v1, got %q", got)
	}
}

func TestAdapter_Set_UpdatesWhenPresent(t *testing.T) {
	store := newFakeStore()
	store.rows["k1"] = "old"
	a := New(store)

	if err := a.Set(context.Background(), "k1", "name", "new"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := store.rows["k1"]; got != "new" {
		t.Errorf("expected value new, got %q", got)
	}
}

func TestAdapter_Exists_AbsenceVsError(t *testing.T) {
	store := newFakeStore()
	a := New(store)

	got, err := a.Exists(context.Background(), "missing")
	if err != nil || got {
		t.Errorf("missing: want (false, nil), got (%v, %v)", got, err)
	}

	store.rows["here"] = "x"
	got, err = a.Exists(context.Background(), "here")
	if err != nil || !got {
		t.Errorf("present: want (true, nil), got (%v, %v)", got, err)
	}

	store.getErr = errors.New("db is down")
	got, err = a.Exists(context.Background(), "anything")
	if err == nil || got {
		t.Errorf("transient error: want (false, err), got (%v, %v)", got, err)
	}
}

func TestAdapter_Set_DoesNotMaskTransientError(t *testing.T) {
	store := newFakeStore()
	store.rows["k1"] = "v1"
	store.getErr = errors.New("db is down")
	a := New(store)

	// A transient Get error must surface, not silently fall through to Create.
	err := a.Set(context.Background(), "k1", "name", "new")
	if err == nil {
		t.Fatal("expected error from transient Get failure, got nil")
	}
}

func TestAdapter_Reveal_DelegatesToStore(t *testing.T) {
	store := newFakeStore()
	store.rows["k1"] = "secret"
	a := New(store)

	got, err := a.Reveal(context.Background(), "k1")
	if err != nil || got != "secret" {
		t.Errorf("got (%q, %v), want (\"secret\", nil)", got, err)
	}
}

func TestAdapter_Delete_DelegatesToStore(t *testing.T) {
	store := newFakeStore()
	store.rows["k1"] = "v"
	a := New(store)

	if err := a.Delete(context.Background(), "k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := store.rows["k1"]; ok {
		t.Error("Delete did not remove the row")
	}
}
