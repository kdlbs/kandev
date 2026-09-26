package updates

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/persistence"
	userstore "github.com/kandev/kandev/internal/user/store"
)

type capturingNotifier struct {
	calls []updateNotification
}

type updateNotification struct {
	version string
	url     string
}

type firstCallBlockingNotifier struct {
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (n *firstCallBlockingNotifier) HandleUpdateAvailable(ctx context.Context, _, _ string) {
	if n.calls.Add(1) != 1 {
		return
	}
	close(n.entered)
	select {
	case <-n.release:
	case <-ctx.Done():
	}
}

func (n *capturingNotifier) HandleUpdateAvailable(_ context.Context, version, releaseURL string) {
	n.calls = append(n.calls, updateNotification{version: version, url: releaseURL})
}

func TestService_ReplayCachedUpdate_NotifiesCachedNewerReleaseWithoutFetching(t *testing.T) {
	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default())
	notifier := &capturingNotifier{}
	svc.SetNotifier(notifier)
	svc.SetFetcher(func(context.Context) (string, string, error) {
		t.Fatal("ReplayCachedUpdate must not fetch GitHub")
		return "", "", nil
	})

	if err := persistence.WriteLatestVersion(svc.pool.Writer(), "v1.0.1", "https://example.test/v1.0.1", time.Now()); err != nil {
		t.Fatalf("write cached release: %v", err)
	}
	if err := svc.ReplayCachedUpdate(context.Background()); err != nil {
		t.Fatalf("ReplayCachedUpdate: %v", err)
	}

	if len(notifier.calls) != 1 {
		t.Fatalf("notifier calls = %d, want 1", len(notifier.calls))
	}
	if got := notifier.calls[0]; got.version != "v1.0.1" || got.url != "https://example.test/v1.0.1" {
		t.Errorf("notifier call = %+v, want cached release", got)
	}
}

func TestService_FetchAndPersist_NotifiesCanonicalServiceForNewerRelease(t *testing.T) {
	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default())
	notifier := &capturingNotifier{}
	svc.SetNotifier(notifier)
	svc.SetFetcher(func(context.Context) (string, string, error) {
		return "v1.0.1", "https://example.test/v1.0.1", nil
	})

	if _, err := svc.fetchAndPersist(context.Background()); err != nil {
		t.Fatalf("fetchAndPersist: %v", err)
	}

	if len(notifier.calls) != 1 {
		t.Fatalf("notifier calls = %d, want 1", len(notifier.calls))
	}
}

func TestService_ReplayNightlyNotifiesForUnequalAuthoritativeSHA(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	store := &memorySettingsStore{value: []byte(ChannelNightly), present: true}
	if err := persistence.WriteLatestNightlyVersion(
		pool.Writer(),
		"1.2.4-nightly.sha000000000000",
		"https://example.test/nightly",
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	svc := NewService(
		pool,
		"v1.2.4-nightly.shaffffffffffff",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithSettingsStore(store),
	)
	notifier := &capturingNotifier{}
	svc.SetNotifier(notifier)
	if err := svc.ReplayCachedUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("notifier calls=%d want 1", len(notifier.calls))
	}
	if got := notifier.calls[0]; got.version != "1.2.4-nightly.sha000000000000" || got.url != "https://example.test/nightly" {
		t.Errorf("notifier call = %+v, want cached nightly", got)
	}
}

func TestService_ReplayCachedUpdateSerializesWithChannelSelection(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	store := &memorySettingsStore{value: []byte(ChannelStable), present: true}
	if err := persistence.WriteLatestVersion(
		pool.Writer(),
		"v1.2.4",
		"https://example.test/stable",
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	svc := NewService(
		pool,
		"v1.2.3",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithSettingsStore(store),
	)
	notifier := &firstCallBlockingNotifier{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	releaseReplay := sync.OnceFunc(func() { close(notifier.release) })
	t.Cleanup(releaseReplay)
	svc.SetNotifier(notifier)
	nightlyFetchEntered := make(chan struct{})
	nightlyFetchRelease := make(chan struct{})
	releaseNightlyFetch := sync.OnceFunc(func() { close(nightlyFetchRelease) })
	t.Cleanup(releaseNightlyFetch)
	svc.SetNightlyFetcher(func(ctx context.Context) (string, string, error) {
		close(nightlyFetchEntered)
		select {
		case <-nightlyFetchRelease:
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
		return "v1.2.5-nightly.shaabcdef123456", "https://example.test/nightly", nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	replayDone := make(chan error, 1)
	go func() { replayDone <- svc.ReplayCachedUpdate(ctx) }()
	select {
	case <-notifier.entered:
	case err := <-replayDone:
		t.Fatalf("cached replay returned before notifier: %v", err)
	case <-ctx.Done():
		t.Fatal("cached replay did not reach notifier")
	}
	if svc.updateMu.TryLock() {
		svc.updateMu.Unlock()
		t.Fatal("update lock was not held while cached replay notified")
	}

	selectDone := make(chan error, 1)
	selectStarted := make(chan struct{})
	go func() {
		close(selectStarted)
		_, err := svc.SelectChannel(ctx, string(ChannelNightly))
		selectDone <- err
	}()
	select {
	case <-selectStarted:
	case <-ctx.Done():
		t.Fatal("channel selection did not start")
	}

	releaseReplay()
	select {
	case err := <-replayDone:
		if err != nil {
			t.Fatalf("cached replay: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("cached replay did not finish")
	}
	select {
	case <-nightlyFetchEntered:
	case err := <-selectDone:
		t.Fatalf("channel selection returned before resolving Nightly: %v", err)
	case <-ctx.Done():
		t.Fatal("channel selection did not resolve Nightly")
	}
	releaseNightlyFetch()
	select {
	case err := <-selectDone:
		if err != nil {
			t.Fatalf("channel selection: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("channel selection did not finish")
	}
}

func TestService_ReturnToOlderStableIsAvailableWithoutUpgradeNotification(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	store := &memorySettingsStore{value: []byte(ChannelStable), present: true}
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.2.3", "https://example.test/stable", time.Now()); err != nil {
		t.Fatal(err)
	}
	svc := NewService(
		pool,
		"v1.2.4-nightly.shaabc123def456",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithSettingsStore(store),
	)
	notifier := &capturingNotifier{}
	svc.SetNotifier(notifier)

	resp, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !resp.UpdateAvailable {
		t.Fatal("explicit stable return should be available")
	}
	if err := svc.ReplayCachedUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("downgrade-like stable return sent %d notification(s)", len(notifier.calls))
	}
}

func TestService_PollBeforeLocalSubscription_ReplaysCachedUpdateExactlyOnce(t *testing.T) {
	ctx := context.Background()
	t.Setenv("KANDEV_DESKTOP_NATIVE_NOTIFICATIONS", "true")
	pool := newTestPool(t)
	repo, closeRepo, err := notificationstore.Provide(ctx, pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatalf("provide notification repository: %v", err)
	}
	t.Cleanup(func() { _ = closeRepo() })

	hub := gateways.NewHub(nil, logger.Default())
	notifier := notificationservice.NewService(repo, nil, hub, logger.Default(), nil)
	svc := NewService(pool, "v1.0.0", nil, logger.Default())
	svc.SetNotifier(notifier)
	fetches := 0
	svc.SetFetcher(func(context.Context) (string, string, error) {
		fetches++
		return "v1.0.1", "https://example.test/v1.0.1", nil
	})
	hub.AddUserSubscriptionListener(func(userID string) {
		if userID != userstore.DefaultUserID {
			return
		}
		if err := svc.ReplayCachedUpdate(ctx); err != nil {
			t.Errorf("replay cached update: %v", err)
		}
	})

	if _, err := svc.fetchAndPersist(ctx); err != nil {
		t.Fatalf("initial poll: %v", err)
	}
	if got := localUpdateDeliveryCount(t, pool); got != 0 {
		t.Fatalf("Local delivery claims after poll without subscriber = %d, want 0", got)
	}

	first := gateways.NewClient("first", authn.Identity{}, nil, hub, logger.Default())
	hub.SubscribeToUser(first, userstore.DefaultUserID)
	if got := localUpdateDeliveryCount(t, pool); got != 1 {
		t.Fatalf("Local delivery claims after first subscription = %d, want 1", got)
	}
	if fetches != 1 {
		t.Fatalf("GitHub fetches after cached replay = %d, want 1", fetches)
	}

	second := gateways.NewClient("second", authn.Identity{}, nil, hub, logger.Default())
	hub.SubscribeToUser(second, userstore.DefaultUserID)
	if got := localUpdateDeliveryCount(t, pool); got != 1 {
		t.Fatalf("Local delivery claims after second subscription = %d, want 1", got)
	}
	if fetches != 1 {
		t.Fatalf("GitHub fetches after second replay = %d, want 1", fetches)
	}
}

func localUpdateDeliveryCount(t *testing.T, pool *db.Pool) int {
	t.Helper()
	var count int
	if err := pool.Reader().Get(&count, `
		SELECT COUNT(*)
		FROM notification_deliveries deliveries
		JOIN notification_providers providers ON providers.id = deliveries.provider_id
		WHERE providers.type = 'local'
			AND deliveries.event_type = 'system.update_available'
			AND deliveries.occurrence_id = 'v1.0.1'
	`); err != nil {
		t.Fatalf("count Local update deliveries: %v", err)
	}
	return count
}
