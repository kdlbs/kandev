package metrics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestServiceSubscribeLifecycle(t *testing.T) {
	svc := newTestService(t)
	published := make(chan struct{}, 4)
	svc.SetBroadcaster(func(_ *ws.Message) {
		published <- struct{}{}
	})

	svc.MetricsSubscribe("client-1")
	waitForPublish(t, published)

	svc.mu.Lock()
	if len(svc.interested) != 1 {
		t.Fatalf("interested=%d, want 1", len(svc.interested))
	}
	if svc.cancel == nil {
		t.Fatal("expected sampler to start on first subscriber")
	}
	svc.mu.Unlock()

	svc.MetricsSubscribe("client-1")
	svc.mu.Lock()
	if len(svc.interested) != 1 {
		t.Fatalf("interested after duplicate subscribe=%d, want 1", len(svc.interested))
	}
	svc.mu.Unlock()

	svc.MetricsSubscribe("client-2")
	svc.MetricsUnsubscribe("client-1")
	svc.mu.Lock()
	if svc.cancel == nil {
		t.Fatal("expected sampler to keep running while another subscriber remains")
	}
	svc.mu.Unlock()

	svc.MetricsUnsubscribe("client-2")
	svc.mu.Lock()
	if len(svc.interested) != 0 {
		t.Fatalf("interested after unsubscribe=%d, want 0", len(svc.interested))
	}
	if svc.cancel != nil {
		t.Fatal("expected sampler cancel to clear after last unsubscribe")
	}
	svc.mu.Unlock()
}

func TestServiceCollectExecutionsUsesContainerRootDiskPath(t *testing.T) {
	svc := newTestService(t)
	client := &recordingMetricsClient{}
	svc.SetExecutionProvider(staticExecutionProvider{sources: []ExecutionSource{{
		ID:     "exec-1",
		Label:  "Executor",
		Client: client,
	}}})

	settings := DefaultSettings()
	settings.BackendDiskPath = "/mnt/backend-data"
	settings.CollectExecution = true

	sources := svc.collectExecutions(context.Background(), settings)
	if len(sources) != 1 {
		t.Fatalf("sources=%d, want 1", len(sources))
	}
	if client.diskPath != "/" {
		t.Fatalf("disk path=%q, want /", client.diskPath)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store, err := NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := store.SaveSettings(context.Background(), GlobalSettings{
		Metrics:         []string{MetricCPUPercent},
		IntervalSeconds: 1,
		BackendDiskPath: "/",
	}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	return NewService(store, NewCollector())
}

func waitForPublish(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for metrics publish")
	}
}

type staticExecutionProvider struct {
	sources []ExecutionSource
}

func (p staticExecutionProvider) MetricExecutions() []ExecutionSource {
	return p.sources
}

type recordingMetricsClient struct {
	mu       sync.Mutex
	diskPath string
}

func (c *recordingMetricsClient) SystemMetrics(_ context.Context, _ []string, diskPath string) (*SourceSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diskPath = diskPath
	return &SourceSnapshot{Metrics: []MetricSample{}}, nil
}
