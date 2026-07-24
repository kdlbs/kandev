package updates

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// capturingBroadcaster records every message handed to it so tests can
// assert whether (and what) the service broadcast.
type capturingBroadcaster struct {
	messages []*ws.Message
}

func (c *capturingBroadcaster) broadcast(msg *ws.Message) {
	c.messages = append(c.messages, msg)
}

// newNotifyTestService wires a Service against a stub GitHub release server
// plus a fresh NotifyStore (defaults: enabled, both channels) and a
// capturing broadcaster, for tests that assert on the notify-on-new-update
// side effect of Check()/fetchAndPersist.
func newNotifyTestService(t *testing.T, tag, url string) (svc *Service, bc *capturingBroadcaster, notifyStore *NotifyStore) {
	t.Helper()
	pool := newTestPool(t)
	srv, _ := newStubGitHub(t, tag, url)
	notifyStore = newTestNotifyStore(t)
	svc = NewService(pool, "v1.0.0", srv.Client(), logger.Default(), WithNotifyStore(notifyStore))
	svc.SetReleaseURL(srv.URL)
	bc = &capturingBroadcaster{}
	svc.SetBroadcaster(bc.broadcast)
	return svc, bc, notifyStore
}

func TestService_Check_BroadcastsUpdateAvailable_WhenEnabled(t *testing.T) {
	svc, bc, _ := newNotifyTestService(t, "v1.0.1", "https://example/v1.0.1")

	if _, err := svc.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}

	if len(bc.messages) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(bc.messages))
	}
	if bc.messages[0].Action != ws.ActionUpdateAvailable {
		t.Errorf("action = %q, want %q", bc.messages[0].Action, ws.ActionUpdateAvailable)
	}
}

func TestService_Check_NoBroadcast_WhenNotifyDisabled(t *testing.T) {
	svc, bc, notifyStore := newNotifyTestService(t, "v1.0.1", "https://example/v1.0.1")
	if _, err := notifyStore.SaveSettings(context.Background(), NotifySettings{Enabled: false, Channel: NotifyChannelBoth}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	if _, err := svc.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}

	if len(bc.messages) != 0 {
		t.Fatalf("expected no broadcast while disabled, got %d", len(bc.messages))
	}
}

func TestService_Check_NoBroadcast_WhenNoUpdateAvailable(t *testing.T) {
	// current == latest -> not an update.
	svc, bc, _ := newNotifyTestService(t, "v1.0.0", "https://example/v1.0.0")

	if _, err := svc.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(bc.messages) != 0 {
		t.Fatalf("expected no broadcast when already up to date, got %d", len(bc.messages))
	}
}

func TestService_Check_DedupesRepeatedBroadcastForSameVersion(t *testing.T) {
	svc, bc, _ := newNotifyTestService(t, "v1.0.1", "https://example/v1.0.1")

	// Simulate two separate poll ticks of the same release (fetchAndPersist
	// is what both Check() and the poller call; using it directly avoids the
	// unrelated 30s manual-check rate limiter tested elsewhere).
	if _, err := svc.fetchAndPersist(context.Background()); err != nil {
		t.Fatalf("fetchAndPersist 1: %v", err)
	}
	if _, err := svc.fetchAndPersist(context.Background()); err != nil {
		t.Fatalf("fetchAndPersist 2: %v", err)
	}

	if len(bc.messages) != 1 {
		t.Fatalf("expected exactly 1 broadcast across repeated polls of the same version, got %d", len(bc.messages))
	}
}

func TestService_Check_NilBroadcaster_DoesNotPanic(t *testing.T) {
	pool := newTestPool(t)
	srv, _ := newStubGitHub(t, "v1.0.1", "https://example/v1.0.1")
	svc := NewService(pool, "v1.0.0", srv.Client(), logger.Default())
	svc.SetReleaseURL(srv.URL)

	if _, err := svc.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestService_Check_ConcurrentChecks_DoNotDoubleBroadcast(t *testing.T) {
	// Regression test for a race between fetchAndPersist calls that land
	// concurrently (e.g. a manual Check racing the poller's tick): only one
	// of them should win the notified-version dedup and broadcast.
	svc, bc, _ := newNotifyTestService(t, "v1.0.1", "https://example/v1.0.1")

	const n = 8
	done := make(chan struct{}, n)
	for range n {
		go func() {
			defer func() { done <- struct{}{} }()
			svc.notifyIfNewUpdate(context.Background(), "v1.0.1", "https://example/v1.0.1")
		}()
	}
	for range n {
		<-done
	}

	if got := len(bc.messages); got != 1 {
		t.Fatalf("expected exactly 1 broadcast across %d concurrent notifyIfNewUpdate calls, got %d", n, got)
	}
}
