package acp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type sessionCreationAgent struct {
	burstAgent
	conn  *acpsdk.AgentSideConnection
	mu    sync.Mutex
	calls int
	onNew func(context.Context, acpsdk.NewSessionRequest, int) (acpsdk.NewSessionResponse, error)
}

func (a *sessionCreationAgent) Initialize(
	_ context.Context,
	req acpsdk.InitializeRequest,
) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{ProtocolVersion: req.ProtocolVersion}, nil
}

func (a *sessionCreationAgent) NewSession(
	ctx context.Context,
	req acpsdk.NewSessionRequest,
) (acpsdk.NewSessionResponse, error) {
	a.mu.Lock()
	a.calls++
	call := a.calls
	onNew := a.onNew
	a.mu.Unlock()
	if onNew != nil {
		return onNew(ctx, req, call)
	}
	return acpsdk.NewSessionResponse{SessionId: acpsdk.SessionId(fmt.Sprintf("new-%d", call))}, nil
}

func (a *sessionCreationAgent) sendUsage(
	ctx context.Context,
	sessionID string,
	used int64,
	costUSD float64,
) error {
	return a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: acpsdk.SessionId(sessionID),
		Update: acpsdk.SessionUpdate{
			UsageUpdate: usageUpdateWithCost(200_000, used, costUSD, "USD"),
		},
	})
}

func setupSessionCreationAdapter(t *testing.T) (*Adapter, *sessionCreationAgent) {
	t.Helper()
	a := newTestAdapter()
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})
	if err := a.Connect(c2aW, a2cR); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	fake := &sessionCreationAgent{}
	fake.conn = acpsdk.NewAgentSideConnection(fake, a2cW, c2aR)
	if err := a.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	drainEvents(a)
	return a, fake
}

func TestNewSession_NoUsageNotificationLeavesNoTracker(t *testing.T) {
	a, _ := setupSessionCreationAdapter(t)
	a.convertUsageUpdate("stale-session", usageUpdate(200_000, 900))

	sessionID, err := a.NewSession(context.Background(), nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if sessionID != "new-1" {
		t.Fatalf("sessionID = %q, want new-1", sessionID)
	}
	a.mu.RLock()
	trackerCount := len(a.usageBySession)
	a.mu.RUnlock()
	if trackerCount != 0 {
		t.Fatalf("usage tracker count = %d, want 0", trackerCount)
	}
}

func TestNewSession_ConsumesCreationUsageAndCostAtBarrier(t *testing.T) {
	a, fake := setupSessionCreationAdapter(t)
	fake.onNew = func(ctx context.Context, _ acpsdk.NewSessionRequest, _ int) (acpsdk.NewSessionResponse, error) {
		if err := fake.sendUsage(ctx, "created-session", 100, 1.23); err != nil {
			return acpsdk.NewSessionResponse{}, err
		}
		return acpsdk.NewSessionResponse{SessionId: "created-session"}, nil
	}

	if _, err := a.NewSession(context.Background(), nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if delta, cost := a.consumeUsageDelta("created-session"); delta != 0 || cost != 0 {
		t.Fatalf("creation baseline = (%d, %d), want (0, 0)", delta, cost)
	}
	a.mu.RLock()
	maxSize := a.usageBySession["created-session"].maxSize
	a.mu.RUnlock()
	if maxSize != 200_000 {
		t.Fatalf("sticky max size = %d, want 200000", maxSize)
	}

	a.convertUsageUpdate("created-session", usageUpdateWithCost(200_000, 125, 1.25, "USD"))
	if delta, cost := a.consumeUsageDelta("created-session"); delta != 25 || cost != 200 {
		t.Fatalf("first-turn delta = (%d, %d), want (25, 200)", delta, cost)
	}

	var contextEventSeen bool
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeContextWindow && event.ContextWindowUsed == 100 {
			contextEventSeen = true
		}
	}
	if !contextEventSeen {
		t.Fatal("session-creation context window was not displayed")
	}
}

func TestNewSession_WaitsForBackedUpCreationUsage(t *testing.T) {
	a, fake := setupSessionCreationAdapter(t)
	fake.onNew = func(ctx context.Context, _ acpsdk.NewSessionRequest, _ int) (acpsdk.NewSessionResponse, error) {
		if err := fake.sendUsage(ctx, "blocked-session", 100, 1.23); err != nil {
			return acpsdk.NewSessionResponse{}, err
		}
		return acpsdk.NewSessionResponse{SessionId: "blocked-session"}, nil
	}

	workerBlocked := make(chan struct{})
	releaseWorker := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseWorker)
		}
	}()
	a.dialect.suppressNotification = func(n acpsdk.SessionNotification) bool {
		if n.Update.UsageUpdate != nil {
			close(workerBlocked)
			<-releaseWorker
		}
		return false
	}

	done := make(chan error, 1)
	go func() { _, err := a.NewSession(context.Background(), nil); done <- err }()
	select {
	case <-workerBlocked:
	case <-time.After(5 * time.Second):
		t.Fatal("creation usage did not reach blocked worker")
	}
	select {
	case err := <-done:
		t.Fatalf("NewSession returned before creation barrier drained: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseWorker)
	released = true
	if err := <-done; err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if delta, cost := a.consumeUsageDelta("blocked-session"); delta != 0 || cost != 0 {
		t.Fatalf("blocked creation baseline = (%d, %d), want (0, 0)", delta, cost)
	}
}

func TestNewSession_RPCFailureAndCancellationClearTrackers(t *testing.T) {
	tests := []struct {
		name string
		ctx  func() context.Context
	}{
		{
			name: "failure",
			ctx:  context.Background,
		},
		{
			name: "cancellation",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, fake := setupSessionCreationAdapter(t)
			if tt.name == "failure" {
				fake.onNew = func(ctx context.Context, _ acpsdk.NewSessionRequest, _ int) (acpsdk.NewSessionResponse, error) {
					if err := fake.sendUsage(ctx, "failed-session", 100, 1.23); err != nil {
						return acpsdk.NewSessionResponse{}, err
					}
					return acpsdk.NewSessionResponse{}, errors.New("creation failed")
				}
			} else {
				fake.onNew = func(context.Context, acpsdk.NewSessionRequest, int) (acpsdk.NewSessionResponse, error) {
					return acpsdk.NewSessionResponse{}, context.Canceled
				}
			}
			a.convertUsageUpdate("stale-session", usageUpdate(200_000, 900))

			if _, err := a.NewSession(tt.ctx(), nil); err == nil {
				t.Fatal("NewSession unexpectedly succeeded")
			}
			a.mu.RLock()
			trackerCount := len(a.usageBySession)
			a.mu.RUnlock()
			if trackerCount != 0 {
				t.Fatalf("usage tracker count after failure = %d, want 0", trackerCount)
			}
		})
	}
}

func TestResetSession_ConsumesEachCreationBaseline(t *testing.T) {
	a, fake := setupSessionCreationAdapter(t)
	fake.onNew = func(ctx context.Context, _ acpsdk.NewSessionRequest, call int) (acpsdk.NewSessionResponse, error) {
		sessionID := fmt.Sprintf("reset-%d", call)
		if err := fake.sendUsage(ctx, sessionID, int64(call*100), float64(call)+0.25); err != nil {
			return acpsdk.NewSessionResponse{}, err
		}
		return acpsdk.NewSessionResponse{SessionId: acpsdk.SessionId(sessionID)}, nil
	}

	if _, err := a.NewSession(context.Background(), nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := a.ResetSession(context.Background(), nil); err != nil {
		t.Fatalf("ResetSession: %v", err)
	}
	if delta, cost := a.consumeUsageDelta("reset-2"); delta != 0 || cost != 0 {
		t.Fatalf("reset baseline = (%d, %d), want (0, 0)", delta, cost)
	}
	a.mu.RLock()
	_, oldTracker := a.usageBySession["reset-1"]
	a.mu.RUnlock()
	if oldTracker {
		t.Fatal("ResetSession retained the previous session tracker")
	}
	a.convertUsageUpdate("reset-2", usageUpdateWithCost(200_000, 230, 2.50, "USD"))
	if delta, cost := a.consumeUsageDelta("reset-2"); delta != 30 || cost != 2500 {
		t.Fatalf("first post-reset delta = (%d, %d), want (30, 2500)", delta, cost)
	}
}

func TestNewSession_BarrierShutdownClearsProvisionalTracker(t *testing.T) {
	a, fake := setupSessionCreationAdapter(t)
	fake.onNew = func(ctx context.Context, _ acpsdk.NewSessionRequest, _ int) (acpsdk.NewSessionResponse, error) {
		if err := fake.sendUsage(ctx, "shutdown-session", 100, 1.23); err != nil {
			return acpsdk.NewSessionResponse{}, err
		}
		return acpsdk.NewSessionResponse{SessionId: "shutdown-session"}, nil
	}

	workerBlocked := make(chan struct{})
	releaseWorker := make(chan struct{})
	a.dialect.suppressNotification = func(n acpsdk.SessionNotification) bool {
		if n.Update.UsageUpdate != nil {
			close(workerBlocked)
			<-releaseWorker
		}
		return false
	}
	done := make(chan error, 1)
	go func() { _, err := a.NewSession(context.Background(), nil); done <- err }()
	select {
	case <-workerBlocked:
	case <-time.After(5 * time.Second):
		t.Fatal("creation usage did not reach blocked worker")
	}
	waitForQueuedBarrier(t, a)
	closeDone := make(chan struct{})
	go func() { _ = a.Close(); close(closeDone) }()

	err := <-done
	if err == nil || !strings.Contains(err.Error(), "synchronize new session notifications") {
		t.Fatalf("NewSession error = %v, want barrier synchronization error", err)
	}
	close(releaseWorker)
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not finish after releasing worker")
	}
	a.mu.RLock()
	trackerCount := len(a.usageBySession)
	a.mu.RUnlock()
	if trackerCount != 0 {
		t.Fatalf("usage tracker count after barrier shutdown = %d, want 0", trackerCount)
	}
}

func waitForQueuedBarrier(t *testing.T, a *Adapter) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if len(a.notifQueue) > 0 {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("session/new barrier was not queued behind blocked notification")
		case <-ticker.C:
		}
	}
}
