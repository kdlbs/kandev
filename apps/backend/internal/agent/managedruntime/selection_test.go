package managedruntime

import (
	"context"
	"errors"
	"testing"
)

type memorySettings struct {
	values     map[string][]byte
	err        error
	getErrors  map[string]error
	saveErr    error
	deleteErr  error
	operations []string
}

func (s *memorySettings) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.operations = append(s.operations, "get:"+key)
	if s.err != nil {
		return nil, false, s.err
	}
	if err := s.getErrors[key]; err != nil {
		return nil, false, err
	}
	value, ok := s.values[key]
	return append([]byte(nil), value...), ok, nil
}

func (s *memorySettings) Save(_ context.Context, key string, value []byte) error {
	s.operations = append(s.operations, "save:"+key)
	if s.err != nil {
		return s.err
	}
	if s.saveErr != nil {
		return s.saveErr
	}
	if s.values == nil {
		s.values = make(map[string][]byte)
	}
	s.values[key] = append([]byte(nil), value...)
	return nil
}

func (s *memorySettings) Delete(_ context.Context, key string) error {
	s.operations = append(s.operations, "delete:"+key)
	if s.err != nil {
		return s.err
	}
	if s.deleteErr != nil {
		return s.deleteErr
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

// @covers AC-AGENTS-RUNTIME-UPDATES-002.1, AC-AGENTS-RUNTIME-UPDATES-002.2,
// AC-AGENTS-RUNTIME-UPDATES-002.7
func TestStoreReconcileDefaultsPreservesMatchingGeneration(t *testing.T) {
	settings := &memorySettings{values: map[string][]byte{
		selectionKey("agent-a"):         []byte(`{"package":"pkg-a","version":"0.9.0"}`),
		defaultGenerationKey("agent-a"): []byte(`{"package":"pkg-a","version":"1.0.0"}`),
	}}
	store := NewStore(settings)

	err := store.ReconcileDefaults(context.Background(), []DefaultGeneration{{
		AgentID: "agent-a",
		Package: "pkg-a",
		Version: "1.0.0",
	}})
	if err != nil {
		t.Fatalf("ReconcileDefaults: %v", err)
	}
	if got := settings.operations; len(got) != 1 || got[0] != "get:"+defaultGenerationKey("agent-a") {
		t.Fatalf("operations = %#v, want matching marker read only", got)
	}
	if _, found, err := store.Get(context.Background(), "agent-a", "pkg-a"); err != nil || !found {
		t.Fatalf("selection after matching reconciliation = found %v, err %v", found, err)
	}
}

// @covers AC-AGENTS-RUNTIME-UPDATES-002.1 and AC-AGENTS-RUNTIME-UPDATES-002.7
func TestStoreReconcileDefaultsResetsChangedAndLegacyGenerations(t *testing.T) {
	tests := []struct {
		name         string
		marker       []byte
		wantSequence []string
	}{
		{
			name:         "changed marker",
			marker:       []byte(`{"package":"pkg-a","version":"0.9.0"}`),
			wantSequence: []string{"get:" + defaultGenerationKey("agent-a"), "get:" + selectionKey("agent-a"), "delete:" + selectionKey("agent-a"), "save:" + defaultGenerationKey("agent-a")},
		},
		{
			name:         "legacy selection without marker",
			wantSequence: []string{"get:" + defaultGenerationKey("agent-a"), "get:" + selectionKey("agent-a"), "delete:" + selectionKey("agent-a"), "save:" + defaultGenerationKey("agent-a")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := map[string][]byte{
				selectionKey("agent-a"): []byte(`{"package":"pkg-a","version":"0.9.0"}`),
			}
			if tt.marker != nil {
				values[defaultGenerationKey("agent-a")] = tt.marker
			}
			settings := &memorySettings{values: values}
			store := NewStore(settings)

			err := store.ReconcileDefaults(context.Background(), []DefaultGeneration{{
				AgentID: "agent-a",
				Package: "pkg-a",
				Version: "1.0.0",
			}})
			if err != nil {
				t.Fatalf("ReconcileDefaults: %v", err)
			}
			if len(settings.operations) != len(tt.wantSequence) {
				t.Fatalf("operations = %#v, want %#v", settings.operations, tt.wantSequence)
			}
			for i, want := range tt.wantSequence {
				if settings.operations[i] != want {
					t.Fatalf("operation %d = %q, want %q; all operations %#v", i, settings.operations[i], want, settings.operations)
				}
			}
			if _, found, err := store.Get(context.Background(), "agent-a", "pkg-a"); err != nil || found {
				t.Fatalf("selection after reset = found %v, err %v; want absent", found, err)
			}
			if got := string(settings.values[defaultGenerationKey("agent-a")]); got != `{"package":"pkg-a","version":"1.0.0"}` {
				t.Fatalf("stored marker = %q, want current generation", got)
			}
		})
	}
}

// @covers AC-AGENTS-RUNTIME-UPDATES-002.1
func TestStoreReconcileDefaultsTreatsMalformedMarkerAsStale(t *testing.T) {
	settings := &memorySettings{values: map[string][]byte{
		selectionKey("agent-a"):         []byte(`{"package":"pkg-a","version":"0.9.0"}`),
		defaultGenerationKey("agent-a"): []byte("not-json"),
	}}
	store := NewStore(settings)

	if err := store.ReconcileDefaults(context.Background(), []DefaultGeneration{{
		AgentID: "agent-a",
		Package: "pkg-a",
		Version: "1.0.0",
	}}); err != nil {
		t.Fatalf("ReconcileDefaults: %v", err)
	}
	if _, found, err := store.Get(context.Background(), "agent-a", "pkg-a"); err != nil || found {
		t.Fatalf("selection after malformed marker = found %v, err %v; want absent", found, err)
	}
}

// @covers AC-AGENTS-RUNTIME-UPDATES-002.6
func TestStoreReconcileDefaultsStopsBeforeMarkerWriteWhenSelectionDeleteFails(t *testing.T) {
	wantErr := errors.New("selection delete failed")
	settings := &memorySettings{
		values: map[string][]byte{
			selectionKey("agent-a"): []byte(`{"package":"pkg-a","version":"0.9.0"}`),
		},
		deleteErr: wantErr,
	}
	store := NewStore(settings)
	generation := []DefaultGeneration{{AgentID: "agent-a", Package: "pkg-a", Version: "1.0.0"}}

	if err := store.ReconcileDefaults(context.Background(), generation); !errors.Is(err, wantErr) {
		t.Fatalf("delete error = %v, want %v", err, wantErr)
	}
	if _, found := settings.values[defaultGenerationKey("agent-a")]; found {
		t.Fatal("marker was written after selection deletion failed")
	}

	settings.deleteErr = nil
	if err := store.ReconcileDefaults(context.Background(), generation); err != nil {
		t.Fatalf("retry ReconcileDefaults: %v", err)
	}
}

// @covers AC-AGENTS-RUNTIME-UPDATES-002.6
func TestStoreReconcileDefaultsPropagatesReadAndMarkerWriteErrors(t *testing.T) {
	generation := []DefaultGeneration{{AgentID: "agent-a", Package: "pkg-a", Version: "1.0.0"}}
	readErr := errors.New("settings read failed")
	settings := &memorySettings{getErrors: map[string]error{
		defaultGenerationKey("agent-a"): readErr,
	}}
	store := NewStore(settings)
	if err := store.ReconcileDefaults(context.Background(), generation); !errors.Is(err, readErr) {
		t.Fatalf("marker read error = %v, want %v", err, readErr)
	}

	selectionReadErr := errors.New("selection read failed")
	settings = &memorySettings{
		values: map[string][]byte{
			defaultGenerationKey("agent-a"): []byte(`{"package":"pkg-a","version":"0.9.0"}`),
		},
		getErrors: map[string]error{
			selectionKey("agent-a"): selectionReadErr,
		},
	}
	store = NewStore(settings)
	if err := store.ReconcileDefaults(context.Background(), generation); !errors.Is(err, selectionReadErr) {
		t.Fatalf("selection read error = %v, want %v", err, selectionReadErr)
	}

	markerWriteErr := errors.New("marker write failed")
	settings = &memorySettings{
		values: map[string][]byte{
			selectionKey("agent-a"): []byte(`{"package":"pkg-a","version":"0.9.0"}`),
		},
		saveErr: markerWriteErr,
	}
	store = NewStore(settings)
	if err := store.ReconcileDefaults(context.Background(), generation); !errors.Is(err, markerWriteErr) {
		t.Fatalf("marker write error = %v, want %v", err, markerWriteErr)
	}
	if _, found := settings.values[selectionKey("agent-a")]; found {
		t.Fatal("selection remains after marker write failure")
	}

	settings.saveErr = nil
	if err := store.ReconcileDefaults(context.Background(), generation); err != nil {
		t.Fatalf("retry after marker write failure: %v", err)
	}
	if got := string(settings.values[defaultGenerationKey("agent-a")]); got != `{"package":"pkg-a","version":"1.0.0"}` {
		t.Fatalf("marker after retry = %q, want current generation", got)
	}
}

// @covers AC-AGENTS-RUNTIME-UPDATES-002.1
func TestStoreReconcileDefaultsProcessesAgentsInStableOrder(t *testing.T) {
	settings := &memorySettings{}
	store := NewStore(settings)
	generations := []DefaultGeneration{
		{AgentID: "agent-b", Package: "pkg-b", Version: "1.0.0"},
		{AgentID: "agent-a", Package: "pkg-a", Version: "1.0.0"},
	}

	if err := store.ReconcileDefaults(context.Background(), generations); err != nil {
		t.Fatalf("ReconcileDefaults: %v", err)
	}
	if got, want := settings.operations, []string{
		"get:" + defaultGenerationKey("agent-a"),
		"get:" + selectionKey("agent-a"),
		"save:" + defaultGenerationKey("agent-a"),
		"get:" + defaultGenerationKey("agent-b"),
		"get:" + selectionKey("agent-b"),
		"save:" + defaultGenerationKey("agent-b"),
	}; len(got) != len(want) {
		t.Fatalf("operations = %#v, want %#v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("operation %d = %q, want %q; all operations %#v", i, got[i], want[i], got)
			}
		}
	}
}
