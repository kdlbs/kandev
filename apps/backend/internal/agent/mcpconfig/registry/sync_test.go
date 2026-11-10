package registry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakeCacheStore struct {
	mu          sync.Mutex
	entries     []Entry
	state       SyncState
	stateErr    error
	replaceCall int
	upsertCall  int
}

func (s *fakeCacheStore) ListMCPRegistryEntries(_ context.Context, query string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Entry, 0)
	for _, entry := range s.entries {
		if query == "" || strings.Contains(strings.ToLower(entry.Name), strings.ToLower(query)) {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (s *fakeCacheStore) GetMCPRegistryEntry(_ context.Context, identity string) (*Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.entries {
		if s.entries[index].Identity() == identity {
			entry := s.entries[index]
			return &entry, nil
		}
	}
	return nil, ErrRegistryEntryNotFound
}

func (s *fakeCacheStore) ReplaceMCPRegistryEntries(_ context.Context, entries []Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replaceCall++
	s.entries = append([]Entry(nil), entries...)
	return nil
}

func (s *fakeCacheStore) UpsertMCPRegistryEntries(_ context.Context, entries []Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upsertCall++
	for _, entry := range entries {
		found := false
		for index := range s.entries {
			if s.entries[index].Identity() == entry.Identity() {
				s.entries[index] = entry
				found = true
				break
			}
		}
		if !found {
			s.entries = append(s.entries, entry)
		}
	}
	return nil
}

func (s *fakeCacheStore) GetMCPRegistrySyncState(context.Context) (SyncState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stateErr != nil {
		return SyncState{}, s.stateErr
	}
	return s.state, nil
}

func (s *fakeCacheStore) SaveMCPRegistrySyncState(_ context.Context, state SyncState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	return nil
}

func TestSyncPreservesLastGoodCacheOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	cache := &fakeCacheStore{entries: []Entry{{Name: "com.example/cached", Version: "1.0.0"}}}
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	syncer := NewSyncService(client, cache)
	result, err := syncer.Refresh(context.Background(), false)
	if err == nil || !result.Degraded || !result.Stale {
		t.Fatalf("refresh result/error = %#v/%v", result, err)
	}
	if len(cache.entries) != 1 || cache.entries[0].Name != "com.example/cached" {
		t.Fatalf("cache after failed refresh = %#v", cache.entries)
	}
	if !cache.state.Degraded || cache.state.LastError == "" {
		t.Fatalf("sync state = %#v", cache.state)
	}
}

func TestSyncRefreshesOnlyOnceForConcurrentCallers(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"servers":[{"name":"com.example/one","description":"One","version":"1.0.0"}]}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	cache := &fakeCacheStore{}
	syncer := NewSyncService(client, cache)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, _ = syncer.Refresh(context.Background(), false)
		}()
	}
	group.Wait()
	if requests != 1 {
		t.Fatalf("registry requests = %d, want one", requests)
	}
}

var _ = errors.Is
