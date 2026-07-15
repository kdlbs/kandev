package telemetry

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return log
}

// fakeStore is an in-memory SettingsStore.
type fakeStore struct {
	mu     sync.Mutex
	values map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{values: map[string]string{}}
}

func (f *fakeStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[key]
	return []byte(v), ok, nil
}

func (f *fakeStore) Save(_ context.Context, key string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[key] = string(value)
	return nil
}

// fakeSink records every batch it receives.
type fakeSink struct {
	mu      sync.Mutex
	batches [][]Event
	ids     []string
}

func (f *fakeSink) Send(_ context.Context, distinctID string, events []Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, events)
	f.ids = append(f.ids, distinctID)
	return nil
}

func (f *fakeSink) sent() []Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []Event
	for _, b := range f.batches {
		all = append(all, b...)
	}
	return all
}

func (f *fakeSink) distinctIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ids...)
}

// newTestService builds a Service with a fake store/sink and no bus
// unless one is supplied.
func newTestService(t *testing.T, eventBus bus.EventBus, opts Options) (*Service, *fakeStore, *fakeSink) {
	t.Helper()
	store := newFakeStore()
	sink := &fakeSink{}
	if opts.Sink == nil {
		opts.Sink = sink
	}
	if opts.Version == "" {
		opts.Version = "1.2.3-test"
	}
	if opts.DeployMode == "" {
		opts.DeployMode = "local"
	}
	svc := NewService(store, eventBus, newTestLogger(), opts)
	return svc, store, sink
}

func grantConsent(t *testing.T, svc *Service) {
	t.Helper()
	if _, err := svc.SetConsent(context.Background(), ConsentGranted); err != nil {
		t.Fatalf("SetConsent(granted): %v", err)
	}
}
