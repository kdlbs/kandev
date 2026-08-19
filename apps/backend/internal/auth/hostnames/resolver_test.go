package hostnames

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type memoryCache struct {
	mu      sync.Mutex
	entries map[string]CacheEntry
	set     chan struct{}
}

func (c *memoryCache) GetMany(_ context.Context, ips []string) (map[string]CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]CacheEntry)
	for _, ip := range ips {
		if entry, ok := c.entries[ip]; ok {
			out[ip] = entry
		}
	}
	return out, nil
}

func (c *memoryCache) Get(_ context.Context, ip string) (CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[ip]
	if !ok {
		return CacheEntry{}, ErrNotFound
	}
	return entry, nil
}

func (c *memoryCache) Set(_ context.Context, ip, hostname string, resolvedAt time.Time) error {
	c.mu.Lock()
	c.entries[ip] = CacheEntry{Hostname: hostname, ResolvedAt: &resolvedAt}
	c.mu.Unlock()
	select {
	case c.set <- struct{}{}:
	default:
	}
	return nil
}

type trackingCache struct {
	*memoryCache
	gets chan struct{}
}

func (c *trackingCache) Get(ctx context.Context, ip string) (CacheEntry, error) {
	select {
	case c.gets <- struct{}{}:
	default:
	}
	return c.memoryCache.Get(ctx, ip)
}

func TestResolverDeduplicatesInterestsForTheSameRawIP(t *testing.T) {
	cache := &memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)}
	resolver := NewResolver(cache, func(context.Context, string) (bool, error) { return true, nil }, nil, nil, nil)

	resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10", "192.0.2.10"})

	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	job := resolver.pending["192.0.2.10"]
	if job == nil {
		t.Fatal("missing queued job")
		return
	}
	if len(job.interests) != 1 {
		t.Fatalf("interests = %#v, want one user/IP pair", job.interests)
	}
}

func TestResolverDeduplicatesLookupsAlreadyInFlight(t *testing.T) {
	cache := &trackingCache{
		memoryCache: &memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)},
		gets:        make(chan struct{}, 2),
	}
	lookupStarted := make(chan struct{}, 1)
	releaseLookup := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseLookup) }) }
	defer release()
	resolver := NewResolver(cache, func(context.Context, string) (bool, error) { return true, nil }, nil, nil, nil)
	resolver.lookupAddr = func(context.Context, string) ([]string, error) {
		lookupStarted <- struct{}{}
		<-releaseLookup
		return []string{"mail.example.test."}, nil
	}
	resolver.Start(context.Background())
	t.Cleanup(func() { _ = resolver.Close() })

	resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10"})
	select {
	case <-lookupStarted:
	case <-time.After(time.Second):
		t.Fatal("first lookup did not start")
	}
	select {
	case <-cache.gets:
	case <-time.After(time.Second):
		t.Fatal("first job did not read the cache")
	}
	resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10"})

	select {
	case <-cache.gets:
		t.Fatal("second job read the cache while the first lookup was still in flight")
	default:
	}
	release()
	select {
	case <-cache.set:
	case <-time.After(time.Second):
		t.Fatal("lookup did not persist")
	}
}

func TestResolverStartWaitsForCloseBeforeRestartingWorkers(t *testing.T) {
	cache := &memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)}
	firstLookupStarted := make(chan struct{})
	releaseFirstLookup := make(chan struct{})
	secondLookupStarted := make(chan struct{}, 1)
	var calls int
	var callsMu sync.Mutex
	resolver := NewResolver(cache, func(context.Context, string) (bool, error) { return true, nil }, nil, nil, nil)
	resolver.lookupAddr = func(context.Context, string) ([]string, error) {
		callsMu.Lock()
		calls++
		call := calls
		callsMu.Unlock()
		if call == 1 {
			close(firstLookupStarted)
			<-releaseFirstLookup
			return nil, context.Canceled
		}
		secondLookupStarted <- struct{}{}
		return nil, nil
	}
	resolver.Start(context.Background())
	t.Cleanup(func() {
		select {
		case <-releaseFirstLookup:
		default:
			close(releaseFirstLookup)
		}
		_ = resolver.Close()
	})
	resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10"})
	select {
	case <-firstLookupStarted:
	case <-time.After(time.Second):
		t.Fatal("first lookup did not start")
	}

	closed := make(chan struct{})
	go func() {
		_ = resolver.Close()
		close(closed)
	}()
	for deadline := time.After(time.Second); ; {
		resolver.mu.Lock()
		stopping := resolver.stopping != nil
		resolver.mu.Unlock()
		if stopping {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Close did not enter its stopping state")
		case <-time.After(time.Millisecond):
		}
	}
	started := make(chan struct{})
	go func() {
		resolver.Start(context.Background())
		close(started)
	}()
	select {
	case <-started:
		t.Fatal("Start returned before Close drained the old workers")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirstLookup)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not drain the first worker")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Start did not resume after Close")
	}
	resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10"})
	select {
	case <-secondLookupStarted:
	case <-time.After(time.Second):
		t.Fatal("replacement lookup did not start")
	}
}

func TestResolverCoalescesRepeatedSessionTriggers(t *testing.T) {
	cache := &memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)}
	sessionPasses := make(chan struct{}, 2)
	releasePass := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releasePass) }) }
	defer release()
	resolver := NewResolver(cache, func(context.Context, string) (bool, error) { return true, nil }, func(context.Context, string) ([]string, error) {
		sessionPasses <- struct{}{}
		<-releasePass
		return nil, nil
	}, nil, nil)
	resolver.Start(context.Background())
	t.Cleanup(func() { _ = resolver.Close() })

	resolver.TriggerUserSessionsResolved("user-1")
	select {
	case <-sessionPasses:
	case <-time.After(time.Second):
		t.Fatal("triggered session pass did not start")
	}
	resolver.TriggerUserSessionsResolved("user-1")
	release()
	select {
	case <-sessionPasses:
	case <-time.After(time.Second):
		t.Fatal("repeated trigger did not start a follow-up session pass")
	}
}

func TestResolverSkipsQueuedLookupAfterOptOut(t *testing.T) {
	cache := &trackingCache{
		memoryCache: &memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)},
		gets:        make(chan struct{}, 1),
	}
	enabled := true
	lookupStarted := make(chan struct{}, 1)
	resolver := NewResolver(cache, func(context.Context, string) (bool, error) { return enabled, nil }, nil, nil, nil)
	resolver.lookupAddr = func(context.Context, string) ([]string, error) {
		lookupStarted <- struct{}{}
		return nil, nil
	}

	resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10"})
	enabled = false
	resolver.Start(context.Background())
	t.Cleanup(func() { _ = resolver.Close() })
	select {
	case <-cache.gets:
	case <-time.After(time.Second):
		t.Fatal("queued job did not re-read the cache")
	}
	select {
	case <-lookupStarted:
		t.Fatal("disabled user DNS lookup started")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestResolverRetainsLateEnabledInterestAfterDisabledGate(t *testing.T) {
	cache := &memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)}
	initialGate := make(chan struct{})
	releaseInitialGate := make(chan struct{})
	lookupStarted := make(chan struct{}, 1)
	disabledChecks := 0
	var initialGateOnce sync.Once
	resolver := NewResolver(cache, func(_ context.Context, userID string) (bool, error) {
		if userID != "user-disabled" {
			return userID == "user-enabled", nil
		}
		disabledChecks++
		if disabledChecks == 1 {
			return true, nil
		}
		initialGateOnce.Do(func() {
			close(initialGate)
			<-releaseInitialGate
		})
		return false, nil
	}, nil, nil, nil)
	resolver.lookupAddr = func(context.Context, string) ([]string, error) {
		lookupStarted <- struct{}{}
		return nil, nil
	}
	resolver.Start(context.Background())
	t.Cleanup(func() { _ = resolver.Close() })

	resolver.HostnamesForSessionIPs(context.Background(), "user-disabled", []string{"192.0.2.10"})
	select {
	case <-initialGate:
	case <-time.After(time.Second):
		t.Fatal("initial disabled interest did not reach the settings gate")
	}
	resolver.HostnamesForSessionIPs(context.Background(), "user-enabled", []string{"192.0.2.10"})
	close(releaseInitialGate)

	select {
	case <-lookupStarted:
	case <-time.After(time.Second):
		t.Fatal("late enabled interest did not trigger a lookup")
	}
}

func TestResolverRetainsRepeatedInterestAfterSettingsReenabled(t *testing.T) {
	cache := &memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)}
	settingsGate := make(chan struct{})
	releaseSettingsGate := make(chan struct{})
	lookupStarted := make(chan struct{}, 1)
	var checks atomic.Int32
	resolver := NewResolver(cache, func(context.Context, string) (bool, error) {
		if checks.Add(1) == 2 {
			close(settingsGate)
			<-releaseSettingsGate
			return false, nil
		}
		return true, nil
	}, nil, nil, nil)
	resolver.lookupAddr = func(context.Context, string) ([]string, error) {
		lookupStarted <- struct{}{}
		return nil, nil
	}
	resolver.Start(context.Background())
	t.Cleanup(func() { _ = resolver.Close() })

	resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10"})
	select {
	case <-settingsGate:
	case <-time.After(time.Second):
		t.Fatal("worker did not reach the disabled settings gate")
	}
	resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10"})
	close(releaseSettingsGate)

	select {
	case <-lookupStarted:
	case <-time.After(time.Second):
		t.Fatal("re-enabled repeated interest did not trigger a lookup")
	}
}

func TestResolverWakeQueueMatchesLookupConcurrency(t *testing.T) {
	resolver := NewResolver(nil, nil, nil, nil, nil)

	if got := cap(resolver.wake); got != lookupWorkers {
		t.Fatalf("initial wake queue capacity = %d, want %d workers", got, lookupWorkers)
	}
	resolver.Start(context.Background())
	if err := resolver.Close(); err != nil {
		t.Fatalf("close resolver: %v", err)
	}
	if got := cap(resolver.wake); got != lookupWorkers {
		t.Fatalf("restart wake queue capacity = %d, want %d workers", got, lookupWorkers)
	}
}

func TestResolverChecksEachUserOncePerResolve(t *testing.T) {
	cache := &memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)}
	var settingsReads atomic.Int32
	resolver := NewResolver(cache, func(context.Context, string) (bool, error) {
		settingsReads.Add(1)
		return true, nil
	}, nil, nil, nil)
	resolver.lookupAddr = func(context.Context, string) ([]string, error) {
		return []string{"mail.example.test."}, nil
	}
	resolver.Start(context.Background())
	t.Cleanup(func() { _ = resolver.Close() })

	resolver.HostnamesForSessionIPs(
		context.Background(),
		"user-1",
		[]string{"192.0.2.10", "::ffff:192.0.2.10"},
	)
	select {
	case <-cache.set:
	case <-time.After(time.Second):
		t.Fatal("hostname lookup did not finish")
	}
	if got := settingsReads.Load(); got != 2 {
		t.Fatalf("settings reads = %d, want one admission read and one resolve read", got)
	}
}

func TestResolverLogsSettingsGateErrorsWithUserID(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	resolver := NewResolver(nil, func(context.Context, string) (bool, error) {
		return false, errors.New("settings unavailable")
	}, nil, nil, log)

	if got := resolver.enabledInterests([]interest{{userID: "user-1"}}, make(map[string]bool)); len(got) != 0 {
		t.Fatalf("enabled interests = %#v, want none after settings error", got)
	}
	entries := logs.FilterMessage("hostname settings gate").All()
	if len(entries) != 1 {
		t.Fatalf("settings gate log count = %d, want 1", len(entries))
	}
	if got := entries[0].ContextMap()["user_id"]; got != "user-1" {
		t.Fatalf("settings gate user_id = %#v, want user-1", got)
	}
}

func TestResolverHandlesEnabledUserSettingsEvent(t *testing.T) {
	type settingsUpdateHandler interface {
		HandleUserSettingsUpdated(context.Context, *bus.Event) error
	}
	requested := make(chan string, 1)
	resolver := NewResolver(
		&memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)},
		func(context.Context, string) (bool, error) { return true, nil },
		func(_ context.Context, userID string) ([]string, error) {
			requested <- userID
			return nil, nil
		},
		nil,
		nil,
	)
	handler, ok := any(resolver).(settingsUpdateHandler)
	if !ok {
		t.Fatal("resolver does not handle persisted user settings events")
	}
	resolver.Start(context.Background())
	t.Cleanup(func() { _ = resolver.Close() })

	if err := handler.HandleUserSettingsUpdated(context.Background(), bus.NewEvent(
		events.UserSettingsUpdated,
		"test",
		map[string]any{"user_id": "user-1", "resolve_session_hostnames": false},
	)); err != nil {
		t.Fatalf("handle disabled event: %v", err)
	}
	select {
	case userID := <-requested:
		t.Fatalf("disabled event requested sessions for %s", userID)
	default:
	}

	if err := handler.HandleUserSettingsUpdated(context.Background(), bus.NewEvent(
		events.UserSettingsUpdated,
		"test",
		map[string]any{"user_id": "user-1", "resolve_session_hostnames": true},
	)); err != nil {
		t.Fatalf("handle enabled event: %v", err)
	}
	select {
	case userID := <-requested:
		if userID != "user-1" {
			t.Fatalf("requested user = %q, want user-1", userID)
		}
	case <-time.After(time.Second):
		t.Fatal("enabled event did not request sessions")
	}
}

func TestResolverStreamsUncachedTransientLookupFailure(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	received := make(chan *bus.Event, 1)
	if _, err := eventBus.Subscribe(events.AuthSessionHostnameResolved, func(_ context.Context, event *bus.Event) error {
		received <- event
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	cache := &memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)}
	resolver := NewResolver(cache, func(context.Context, string) (bool, error) { return true, nil }, nil, eventBus, log)
	resolver.lookupAddr = func(context.Context, string) ([]string, error) {
		return nil, &net.DNSError{IsTemporary: true}
	}
	resolver.Start(context.Background())
	t.Cleanup(func() { _ = resolver.Close() })

	resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10"})
	select {
	case event := <-received:
		data := event.Data.(map[string]any)
		if data["hostname"] != "" || data["resolved_at"] != nil {
			t.Fatalf("transient failure event = %#v, want empty unresolved hostname", data)
		}
	case <-time.After(time.Second):
		t.Fatal("uncached transient lookup failure was not streamed")
	}
}

func TestResolverCloseWaitsForTriggeredSessionPass(t *testing.T) {
	cache := &memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)}
	triggerStarted := make(chan struct{})
	releaseTrigger := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseTrigger) }) }
	defer release()
	resolver := NewResolver(cache, func(context.Context, string) (bool, error) { return true, nil }, func(context.Context, string) ([]string, error) {
		close(triggerStarted)
		<-releaseTrigger
		return nil, nil
	}, nil, nil)
	resolver.Start(context.Background())
	t.Cleanup(func() { _ = resolver.Close() })
	resolver.TriggerUserSessionsResolved("user-1")
	select {
	case <-triggerStarted:
	case <-time.After(time.Second):
		t.Fatal("triggered session pass did not start")
	}

	closed := make(chan struct{})
	go func() {
		_ = resolver.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("Close returned before the triggered session pass ended")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not wait for the triggered session pass")
	}
}

func TestHostnamesForSessionIPsGatesAndResolves(t *testing.T) {
	cache := &memoryCache{entries: map[string]CacheEntry{}, set: make(chan struct{}, 1)}
	resolver := NewResolver(cache, func(context.Context, string) (bool, error) { return true, nil }, nil, nil, nil)
	resolver.lookupAddr = func(_ context.Context, ip string) ([]string, error) {
		if ip != "192.0.2.10" {
			t.Fatalf("lookup IP = %q", ip)
		}
		return []string{"mail.example.test."}, nil
	}
	resolver.Start(context.Background())
	t.Cleanup(func() { _ = resolver.Close() })

	initial := resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10"})
	if got := initial["192.0.2.10"]; got.Hostname != "" || got.ResolvedAt != nil {
		t.Fatalf("initial cache entry = %#v, want unknown", got)
	}
	select {
	case <-cache.set:
	case <-time.After(time.Second):
		t.Fatal("lookup did not persist")
	}
	resolved := resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10"})
	if got := resolved["192.0.2.10"].Hostname; got != "mail.example.test" {
		t.Fatalf("hostname = %q, want mail.example.test", got)
	}
}

func TestHostnamesForSessionIPsDoesNotLeakOrLookupWhenDisabled(t *testing.T) {
	now := time.Now().UTC()
	cache := &memoryCache{entries: map[string]CacheEntry{"192.0.2.10": {Hostname: "cached.example.test", ResolvedAt: &now}}, set: make(chan struct{}, 1)}
	resolver := NewResolver(cache, func(context.Context, string) (bool, error) { return false, nil }, nil, nil, nil)
	resolver.lookupAddr = func(context.Context, string) ([]string, error) { t.Fatal("unexpected lookup"); return nil, nil }
	resolver.Start(context.Background())
	t.Cleanup(func() { _ = resolver.Close() })

	if got := resolver.HostnamesForSessionIPs(context.Background(), "user-1", []string{"192.0.2.10"}); len(got) != 0 {
		t.Fatalf("disabled map = %#v, want empty", got)
	}
}
