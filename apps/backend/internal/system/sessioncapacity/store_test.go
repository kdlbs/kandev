package sessioncapacity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestStoreSaveLoadAndRejectsInvalidPersistedRecords(t *testing.T) {
	raw := &memoryRawStore{}
	store := NewStore(raw)
	ctx := context.Background()

	settings, err := store.Load(ctx)
	if err != nil || settings != nil {
		t.Fatalf("load absent = %+v, %v; want nil, nil", settings, err)
	}

	if err := store.Save(ctx, Settings{Enabled: true, MaxSessions: 8}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	settings, err = store.Load(ctx)
	if err != nil || settings == nil || *settings != (Settings{Enabled: true, MaxSessions: 8}) {
		t.Fatalf("load saved = %+v, %v", settings, err)
	}

	for _, persisted := range []string{
		`{"enabled":true,"max_sessions":0}`,
		`{"enabled":false,"max_sessions":2147483648}`,
		`not-json`,
	} {
		raw.set([]byte(persisted), true)
		_, err := store.Load(ctx)
		if !errors.Is(err, ErrInvalidPersisted) {
			t.Fatalf("load %s error = %v, want ErrInvalidPersisted", persisted, err)
		}
	}
}

func TestStoreUpdateWithoutCompareAndSwapUsesFallback(t *testing.T) {
	raw := &memoryRawStore{}
	store := NewStore(raw)

	updated, err := store.Update(context.Background(), func(current *Settings) (Settings, error) {
		if current != nil {
			t.Fatalf("current = %+v, want absent", current)
		}
		return Settings{Enabled: true, MaxSessions: 6}, nil
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated != (Settings{Enabled: true, MaxSessions: 6}) {
		t.Fatalf("updated = %+v", updated)
	}
}

func TestStoreUpdateUsesCASAndPreservesConcurrentPartialChanges(t *testing.T) {
	raw := &compareAndSwapRawStore{}
	if err := raw.Save(context.Background(), SettingsKey, []byte(`{"enabled":false,"max_sessions":5}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	store := NewStore(raw)

	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := store.Update(context.Background(), func(current *Settings) (Settings, error) {
			if current == nil {
				return Settings{}, fmt.Errorf("expected saved settings")
			}
			current.Enabled = true
			return *current, nil
		})
		results <- err
	}()
	go func() {
		<-start
		_, err := store.Update(context.Background(), func(current *Settings) (Settings, error) {
			if current == nil {
				return Settings{}, fmt.Errorf("expected saved settings")
			}
			current.MaxSessions = 8
			return *current, nil
		})
		results <- err
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent update: %v", err)
		}
	}

	settings, err := store.Load(context.Background())
	if err != nil || settings == nil {
		t.Fatalf("load concurrent result = %+v, %v", settings, err)
	}
	if *settings != (Settings{Enabled: true, MaxSessions: 8}) {
		t.Fatalf("concurrent result = %+v, want both fields preserved", *settings)
	}
}

func TestStoreUpdateReturnsInvalidPersistedWithoutTreatingItAsAbsent(t *testing.T) {
	raw := &memoryRawStore{}
	raw.set([]byte(`{"enabled":true,"max_sessions":0}`), true)
	_, err := NewStore(raw).Update(context.Background(), func(*Settings) (Settings, error) {
		t.Fatal("update callback called for invalid persisted record")
		return Settings{}, nil
	})
	if !errors.Is(err, ErrInvalidPersisted) {
		t.Fatalf("update error = %v, want ErrInvalidPersisted", err)
	}
}

type memoryRawStore struct {
	mu        sync.Mutex
	raw       []byte
	found     bool
	getErr    error
	saveErr   error
	saveCalls int
}

func (s *memoryRawStore) Get(context.Context, string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return nil, false, s.getErr
	}
	return append([]byte(nil), s.raw...), s.found, nil
}

func (s *memoryRawStore) Save(_ context.Context, _ string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.raw = append([]byte(nil), value...)
	s.found = true
	return nil
}

func (s *memoryRawStore) set(value []byte, found bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw = append([]byte(nil), value...)
	s.found = found
}

type compareAndSwapRawStore struct {
	mu    sync.Mutex
	raw   []byte
	found bool
}

func (s *compareAndSwapRawStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return s.GetConsistent(ctx, key)
}

func (s *compareAndSwapRawStore) GetConsistent(context.Context, string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.raw...), s.found, nil
}

func (s *compareAndSwapRawStore) Save(_ context.Context, _ string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw = append([]byte(nil), value...)
	s.found = true
	return nil
}

func (s *compareAndSwapRawStore) CompareAndSwap(
	_ context.Context,
	_ string,
	expected, value []byte,
) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expected == nil {
		if s.found {
			return false, nil
		}
	} else if !s.found || !bytes.Equal(s.raw, expected) {
		return false, nil
	}
	s.raw = append([]byte(nil), value...)
	s.found = true
	return true, nil
}
