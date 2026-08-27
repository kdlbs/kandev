package managedruntime

import (
	"context"
	"errors"
	"testing"
)

type memorySettings struct {
	values map[string][]byte
	err    error
}

func (s *memorySettings) Get(_ context.Context, key string) ([]byte, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	value, ok := s.values[key]
	return append([]byte(nil), value...), ok, nil
}

func (s *memorySettings) Save(_ context.Context, key string, value []byte) error {
	if s.err != nil {
		return s.err
	}
	if s.values == nil {
		s.values = make(map[string][]byte)
	}
	s.values[key] = append([]byte(nil), value...)
	return nil
}

func (s *memorySettings) Delete(_ context.Context, key string) error {
	if s.err != nil {
		return s.err
	}
	delete(s.values, key)
	return nil
}

func TestStoreRoundTripAndPerAgentIsolation(t *testing.T) {
	settings := &memorySettings{}
	store := NewStore(settings)
	ctx := context.Background()

	if _, found, err := store.Get(ctx, "opencode-acp", "opencode-ai"); err != nil || found {
		t.Fatalf("missing selection = found %v, err %v", found, err)
	}
	if err := store.Save(ctx, "opencode-acp", "opencode-ai", "1.18.5"); err != nil {
		t.Fatalf("save opencode: %v", err)
	}
	if err := store.Save(ctx, "claude-acp", "@example/claude", "2.0.0"); err != nil {
		t.Fatalf("save claude: %v", err)
	}

	got, found, err := store.Get(ctx, "opencode-acp", "opencode-ai")
	if err != nil || !found || got.Package != "opencode-ai" || got.Version != "1.18.5" {
		t.Fatalf("opencode selection = %#v, found %v, err %v", got, found, err)
	}
	got, found, err = store.Get(ctx, "claude-acp", "@example/claude")
	if err != nil || !found || got.Version != "2.0.0" {
		t.Fatalf("claude selection = %#v, found %v, err %v", got, found, err)
	}
}

func TestStoreRejectsInvalidStoredValueAndPackageMismatch(t *testing.T) {
	settings := &memorySettings{values: map[string][]byte{
		selectionKey("opencode-acp"): []byte(`{"package":"opencode-ai","version":"latest"}`),
		selectionKey("claude-acp"):   []byte(`{"package":"old-claude","version":"1.0.0"}`),
	}}
	store := NewStore(settings)

	if _, _, err := store.Get(context.Background(), "opencode-acp", "opencode-ai"); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("invalid stored selection error = %v, want %v", err, ErrInvalidSelection)
	}
	if _, found, err := store.Get(context.Background(), "claude-acp", "@example/claude"); err != nil || found {
		t.Fatalf("package mismatch = found %v, err %v; want no selection", found, err)
	}
}

func TestStorePropagatesSettingsErrors(t *testing.T) {
	wantErr := errors.New("settings unavailable")
	store := NewStore(&memorySettings{err: wantErr})
	if _, _, err := store.Get(context.Background(), "opencode-acp", "opencode-ai"); !errors.Is(err, wantErr) {
		t.Fatalf("get error = %v, want %v", err, wantErr)
	}
	if err := store.Save(context.Background(), "opencode-acp", "opencode-ai", "1.18.5"); !errors.Is(err, wantErr) {
		t.Fatalf("save error = %v, want %v", err, wantErr)
	}
}

func TestStoreDeleteRemovesSelection(t *testing.T) {
	settings := &memorySettings{}
	store := NewStore(settings)
	ctx := context.Background()
	if err := store.Save(ctx, "opencode-acp", "opencode-ai", "1.18.5"); err != nil {
		t.Fatalf("save selection: %v", err)
	}
	if err := store.Delete(ctx, "opencode-acp", "opencode-ai"); err != nil {
		t.Fatalf("delete selection: %v", err)
	}
	if _, found, err := store.Get(ctx, "opencode-acp", "opencode-ai"); err != nil || found {
		t.Fatalf("selection after delete = found %v, err %v; want absent", found, err)
	}
}
