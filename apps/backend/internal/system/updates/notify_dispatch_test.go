package updates

import (
	"context"
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
	notifier := notificationservice.NewService(repo, nil, hub, logger.Default())
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
