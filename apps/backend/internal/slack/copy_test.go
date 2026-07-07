package slack

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// fakeSecrets is an in-memory SecretStore for copy tests.
type fakeSecrets struct {
	mu     sync.Mutex
	values map[string]string
}

func newFakeSecrets() *fakeSecrets { return &fakeSecrets{values: map[string]string{}} }

func (f *fakeSecrets) Reveal(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.values[id], nil
}

func (f *fakeSecrets) Set(_ context.Context, id, _, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[id] = value
	return nil
}

func (f *fakeSecrets) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.values, id)
	return nil
}

func (f *fakeSecrets) Exists(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.values[id]
	return ok, nil
}

func (f *fakeSecrets) get(id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.values[id]
}

func newCopyTestService(t *testing.T, secrets SecretStore) *Service {
	t.Helper()
	store := newTestStore(t)
	// A fake client keeps the async health probe from doing real IO.
	factory := func(_ *SlackConfig, _, _ string) Client { return &fakeClient{} }
	return NewService(store, secrets, nil, factory, logger.Default())
}

func TestCopyConfigToWorkspace_CopiesConfigAndSecrets(t *testing.T) {
	ctx := context.Background()
	secrets := newFakeSecrets()
	svc := newCopyTestService(t, secrets)
	// Block on the async probe so the test doesn't race the background write.
	done := make(chan struct{}, 4)
	svc.SetProbeHook(func() { done <- struct{}{} })

	const src, dst = "ws-src", "ws-dst"
	if _, err := svc.SetConfigForWorkspace(ctx, src, &SetConfigRequest{
		AuthMethod:          AuthMethodCookie,
		CommandPrefix:       "!go",
		UtilityAgentID:      "ua-1",
		PollIntervalSeconds: 45,
		Token:               "xoxc-token",
		Cookie:              "d-cookie",
	}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	<-done

	got, err := svc.CopyConfigToWorkspace(ctx, src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	<-done

	if got.UtilityAgentID != "ua-1" || got.CommandPrefix != "!go" || got.PollIntervalSeconds != 45 {
		t.Errorf("copied config mismatch: %+v", got)
	}
	if v := secrets.get(SecretKeyForToken(dst)); v != "xoxc-token" {
		t.Errorf("token not copied: %q", v)
	}
	if v := secrets.get(SecretKeyForCookie(dst)); v != "d-cookie" {
		t.Errorf("cookie not copied: %q", v)
	}
}

func TestCopyConfigToWorkspace_SameWorkspace(t *testing.T) {
	svc := newCopyTestService(t, newFakeSecrets())
	if _, err := svc.CopyConfigToWorkspace(context.Background(), "ws-1", "ws-1"); !errors.Is(err, ErrSameWorkspace) {
		t.Fatalf("expected ErrSameWorkspace, got %v", err)
	}
}

func TestCopyConfigToWorkspace_NothingToCopy(t *testing.T) {
	svc := newCopyTestService(t, newFakeSecrets())
	if _, err := svc.CopyConfigToWorkspace(context.Background(), "ws-empty", "ws-dst"); !errors.Is(err, ErrNothingToCopy) {
		t.Fatalf("expected ErrNothingToCopy, got %v", err)
	}
}
