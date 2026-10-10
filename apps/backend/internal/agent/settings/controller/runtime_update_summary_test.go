package controller

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type runtimeSummaryCapture struct {
	mu           sync.Mutex
	individual   []agents.RuntimeUpdateNotice
	batches      [][]agents.RuntimeUpdateNotice
	batchStart   chan struct{}
	releaseBatch <-chan struct{}
}

func (n *runtimeSummaryCapture) HandleAgentRuntimeUpdate(_ context.Context, notice agents.RuntimeUpdateNotice) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.individual = append(n.individual, notice)
}

func (n *runtimeSummaryCapture) HandleAgentRuntimeUpdates(ctx context.Context, notices []agents.RuntimeUpdateNotice) {
	n.mu.Lock()
	n.batches = append(n.batches, append([]agents.RuntimeUpdateNotice(nil), notices...))
	n.mu.Unlock()
	if n.batchStart != nil {
		n.batchStart <- struct{}{}
		select {
		case <-n.releaseBatch:
		case <-ctx.Done():
		}
	}
}

func (n *runtimeSummaryCapture) snapshot() ([]agents.RuntimeUpdateNotice, [][]agents.RuntimeUpdateNotice) {
	n.mu.Lock()
	defer n.mu.Unlock()
	individual := append([]agents.RuntimeUpdateNotice(nil), n.individual...)
	batches := append([][]agents.RuntimeUpdateNotice(nil), n.batches...)
	return individual, batches
}

func TestRuntimeAvailabilityRemovalKeepsFirstWindowDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		startedAt := time.Now()
		key := "gemini\x00npm:@google/gemini-cli"
		batch := runtimeAvailabilityBatch{
			pending: map[string]agents.RuntimeUpdateNotice{
				key: {AgentID: "gemini", RuntimeID: "npm:@google/gemini-cli", Version: "2.0.0"},
			},
		}
		batch.startTimer(startedAt)
		firstDeadline := startedAt.Add(runtimeUpdateAvailabilityWindow)

		batch.applyObservation(runtimeAvailabilityObservation{
			agentID: "gemini", runtimeID: "npm:@google/gemini-cli", observedAt: startedAt.Add(time.Second),
		})
		if !batch.deadline.Equal(firstDeadline) {
			t.Fatalf("empty pending set reset the collection deadline: got %v, want %v", batch.deadline, firstDeadline)
		}

		batch.applyObservation(runtimeAvailabilityObservation{
			agentID: "gemini", runtimeID: "npm:@google/gemini-cli", observedAt: startedAt.Add(20 * time.Second),
			notice: &agents.RuntimeUpdateNotice{AgentID: "gemini", RuntimeID: "npm:@google/gemini-cli", Version: "3.0.0"},
		})
		if !batch.deadline.Equal(firstDeadline) {
			t.Fatalf("replacement availability moved the first deadline: got %v, want %v", batch.deadline, firstDeadline)
		}
		batch.stopTimer()
	})
}

func TestRuntimeUpdateDeliveryQueueCoalescesAndCancelsPendingNotices(t *testing.T) {
	queue := newRuntimeUpdateDeliveryQueue()
	if !queue.admit(context.Background(), []agents.RuntimeUpdateNotice{
		{AgentID: "gemini", RuntimeID: "npm:@google/gemini-cli", Version: "2.0.0", OccurrenceID: "old"},
		{AgentID: "gemini", RuntimeID: "npm:@google/gemini-cli", Version: "3.0.0", OccurrenceID: "current"},
		{AgentID: "claude-acp", RuntimeID: "npm:@agentclientprotocol/claude-agent-acp", Version: "4.0.0", OccurrenceID: "claude"},
	}) {
		t.Fatal("delivery queue rejected active summary work")
	}
	if len(queue.latest) != 2 || len(queue.wake) != 1 {
		t.Fatalf("delivery queue was not bounded by runtime identity: pending=%d wake=%d", len(queue.latest), len(queue.wake))
	}
	if notice := queue.latest["gemini\x00npm:@google/gemini-cli"]; notice.Version != "3.0.0" || notice.OccurrenceID != "current" {
		t.Fatalf("delivery queue did not retain the latest runtime occurrence: %+v", notice)
	}

	queue.close()
	if len(queue.latest) != 0 {
		t.Fatalf("shutdown retained pending notices: %+v", queue.latest)
	}
	if queue.admit(context.Background(), []agents.RuntimeUpdateNotice{{AgentID: "gemini", RuntimeID: "npm:@google/gemini-cli"}}) {
		t.Fatal("delivery queue accepted work after shutdown")
	}
}

func TestRuntimeUpdateSummaryShutdownCancelsBlockedProviderSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gemini := agents.NewGemini()
		c := newTestController(map[string]agents.Agent{gemini.ID(): gemini})
		c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { return "9.0.0", nil })
		batchStart := make(chan struct{}, 1)
		notices := &runtimeSummaryCapture{batchStart: batchStart}
		c.SetRuntimeUpdateNotifier(notices)
		stop := c.StartRuntimeUpdateBackground(context.Background())
		synctest.Wait()

		time.Sleep(runtimeUpdateAvailabilityWindow)
		synctest.Wait()
		select {
		case <-batchStart:
		default:
			t.Fatal("summary delivery did not reach the provider barrier")
		}

		stopped := make(chan struct{})
		go func() {
			stop()
			close(stopped)
		}()
		synctest.Wait()
		select {
		case <-stopped:
		default:
			t.Fatal("shutdown did not cancel the blocked provider send")
		}

		time.Sleep(runtimeUpdateAvailabilityWindow)
		synctest.Wait()
		_, batches := notices.snapshot()
		if len(batches) != 1 {
			t.Fatalf("shutdown delivered another pending summary: %+v", batches)
		}
	})
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-003.7
func TestRuntimeUpdateOutcomeDispatchDoesNotWaitForSummaryDelivery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gemini := agents.NewGemini()
		claude := agents.NewClaudeACP()
		claudePackage := claude.ManagedNPMRuntime().Package
		var claudeAvailable atomic.Bool
		c := newTestController(map[string]agents.Agent{gemini.ID(): gemini, claude.ID(): claude})
		c.SetRuntimeUpdateStatusResolver(func(_ context.Context, packageName string) (string, error) {
			if packageName == claudePackage && !claudeAvailable.Load() {
				return "", errors.New("Claude runtime lookup held for test")
			}
			return "9.0.0", nil
		})
		batchStart := make(chan struct{}, 1)
		releaseBatch := make(chan struct{})
		notices := &runtimeSummaryCapture{batchStart: batchStart, releaseBatch: releaseBatch}
		c.SetRuntimeUpdateNotifier(notices)
		stop := c.StartRuntimeUpdateBackground(context.Background())
		defer stop()
		synctest.Wait()

		time.Sleep(runtimeUpdateAvailabilityWindow)
		synctest.Wait()
		select {
		case <-batchStart:
		default:
			t.Fatal("summary notifier did not reach its delivery barrier")
		}
		_, batches := notices.snapshot()
		if len(batches) != 1 || len(batches[0]) != 1 || batches[0][0].AgentID != gemini.ID() {
			t.Fatalf("first summary did not contain only the available runtime: %+v", batches)
		}

		c.publishRuntimeStatus(context.Background(), dto.AgentUpdateStatusDTO{
			AgentName: gemini.ID(), DisplayName: "Gemini", RuntimeID: agents.RuntimeUpdateCapabilities(gemini).RuntimeID,
			Available: true, Enabled: true, CheckState: dto.AgentUpdateCheckStateUpdateAvailable,
			EffectiveVersion: "1.0.0", LatestVersion: "9.0.0",
			LastOutcome: &managedruntime.UpdateOutcome{ID: "attempt", Status: "failed", PreviousVersion: "1.0.0", TargetVersion: "9.0.0"},
		})
		synctest.Wait()
		individual, _ := notices.snapshot()
		if len(individual) != 1 || individual[0].Status != managedruntime.UpdateOutcomeFailed {
			t.Fatalf("terminal outcome was not delivered while summary was blocked: %+v", individual)
		}

		claudeAvailable.Store(true)
		c.InvalidateRuntimeUpdateStatus(claudePackage)
		response, err := c.ListAgentUpdateStatuses(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var claudeStatus *dto.AgentUpdateStatusDTO
		for i := range response.Statuses {
			if response.Statuses[i].AgentName == claude.ID() {
				claudeStatus = &response.Statuses[i]
				break
			}
		}
		if claudeStatus == nil || !claudeStatus.Available {
			t.Fatalf("test runtime did not become available: %+v", response.Statuses)
		}
		c.publishRuntimeStatus(context.Background(), *claudeStatus)
		synctest.Wait()
		time.Sleep(runtimeUpdateAvailabilityWindow)
		synctest.Wait()

		c.runtimeBackgroundMu.Lock()
		worker := c.runtimeBackground
		c.runtimeBackgroundMu.Unlock()
		worker.deliveries.mu.Lock()
		_, queuedClaude := worker.deliveries.latest[claude.ID()+"\x00"+claudeStatus.RuntimeID]
		worker.deliveries.mu.Unlock()
		if !queuedClaude {
			t.Fatal("collector did not queue the next summary while the previous provider send was blocked")
		}

		close(releaseBatch)
		synctest.Wait()
		select {
		case <-batchStart:
		default:
			t.Fatal("queued next summary did not reach the delivery worker after release")
		}
		_, batches = notices.snapshot()
		if len(batches) != 2 || len(batches[1]) != 2 {
			t.Fatalf("terminal outcome or later availability was lost from the next summary: %+v", batches)
		}
		if batches[1][0].AgentID != claude.ID() || batches[1][1].AgentID != gemini.ID() {
			t.Fatalf("queued summary members were not stably ordered: %+v", batches[1])
		}
	})
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-003.6
func TestRuntimeUpdateSummaryUsesRevalidatedEffectiveVersion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gemini := agents.NewGemini()
		packageName := gemini.ManagedNPMRuntime().Package
		selectionKey := gemini.ID() + "\x00" + packageName
		selections := &statusSelectionStore{selection: map[string]managedruntime.Selection{
			selectionKey: {Package: packageName, Version: "1.0.0"},
		}}
		c := newTestController(map[string]agents.Agent{gemini.ID(): gemini})
		c.SetManagedRuntimeSelectionStore(selections)
		c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { return "3.0.0", nil })
		notices := &runtimeSummaryCapture{}
		c.SetRuntimeUpdateNotifier(notices)
		stop := c.StartRuntimeUpdateBackground(context.Background())
		defer stop()
		synctest.Wait()

		selections.set(selectionKey, managedruntime.Selection{Package: packageName, Version: "2.0.0"})
		time.Sleep(runtimeUpdateAvailabilityWindow)
		synctest.Wait()
		_, batches := notices.snapshot()
		if len(batches) != 1 || len(batches[0]) != 1 {
			t.Fatalf("runtime availability was not delivered once: %+v", batches)
		}
		wantOccurrence := runtimeNoticeKey(gemini.ID(), agents.RuntimeUpdateCapabilities(gemini).RuntimeID, "3.0.0", "available")
		got := batches[0][0]
		if got.PreviousVersion != "2.0.0" || got.Version != "3.0.0" || got.OccurrenceID != wantOccurrence || got.DisplayName != "Gemini" {
			t.Fatalf("summary member used stale availability metadata: got %+v", got)
		}
	})
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-003.1, AC-AGENTS-RUNTIME-NOTIFY-003.2
func TestRuntimeUpdateSummaryWaitsForFixedWindowAndBatchesStartupNotices(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gemini := agents.NewGemini()
		claude := agents.NewClaudeACP()
		c := newTestController(map[string]agents.Agent{gemini.ID(): gemini, claude.ID(): claude})
		c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { return "9.0.0", nil })
		notices := &runtimeSummaryCapture{}
		c.SetRuntimeUpdateNotifier(notices)
		stop := c.StartRuntimeUpdateBackground(context.Background())
		defer stop()
		synctest.Wait()

		individual, batches := notices.snapshot()
		if len(individual) != 0 || len(batches) != 0 {
			t.Fatalf("availability delivered during collection: individual=%d batches=%d", len(individual), len(batches))
		}
		if err := c.ReplayRuntimeUpdateNotices(context.Background()); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()

		time.Sleep(20 * time.Second)
		response, err := c.ListAgentUpdateStatuses(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, status := range response.Statuses {
			if status.AgentName == gemini.ID() {
				c.publishRuntimeStatus(context.Background(), status)
				break
			}
		}
		synctest.Wait()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		individual, batches = notices.snapshot()
		if len(individual) != 0 || len(batches) != 1 || len(batches[0]) != 2 {
			t.Fatalf("startup availability was not grouped once: individual=%+v batches=%+v", individual, batches)
		}
		if batches[0][0].AgentID != claude.ID() || batches[0][1].AgentID != gemini.ID() || batches[0][1].PreviousVersion == "" || batches[0][1].Version != "9.0.0" {
			t.Fatalf("summary members lost trusted runtime details: %+v", batches[0])
		}
	})
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-003.6
func TestRuntimeUpdateSummaryDropsMembersNoLongerAvailable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gemini := agents.NewGemini()
		c := newTestController(map[string]agents.Agent{gemini.ID(): gemini})
		latest := "9.0.0"
		c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { return latest, nil })
		notices := &runtimeSummaryCapture{}
		c.SetRuntimeUpdateNotifier(notices)
		stop := c.StartRuntimeUpdateBackground(context.Background())
		defer stop()
		synctest.Wait()

		response, err := c.ListAgentUpdateStatuses(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var effectiveVersion, packageName string
		for _, status := range response.Statuses {
			if status.AgentName == gemini.ID() {
				effectiveVersion, packageName = status.EffectiveVersion, status.Package
			}
		}
		if effectiveVersion == "" || packageName == "" {
			t.Fatal("runtime status did not provide revalidation identity")
		}
		latest = effectiveVersion
		c.InvalidateRuntimeUpdateStatus(packageName)
		time.Sleep(runtimeUpdateAvailabilityWindow)
		synctest.Wait()
		individual, batches := notices.snapshot()
		if len(individual) != 0 || len(batches) != 0 {
			t.Fatalf("obsolete availability was delivered: individual=%+v batches=%+v", individual, batches)
		}
	})
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-003.7, AC-AGENTS-RUNTIME-NOTIFY-003.9
func TestRuntimeUpdateOutcomeBypassesWindowAndShutdownCancelsAvailability(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gemini := agents.NewGemini()
		c := newTestController(map[string]agents.Agent{gemini.ID(): gemini})
		c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { return "9.0.0", nil })
		notices := &runtimeSummaryCapture{}
		c.SetRuntimeUpdateNotifier(notices)
		stop := c.StartRuntimeUpdateBackground(context.Background())
		synctest.Wait()

		c.publishRuntimeStatus(context.Background(), dto.AgentUpdateStatusDTO{
			AgentName: gemini.ID(), DisplayName: "Gemini", RuntimeID: agents.RuntimeUpdateCapabilities(gemini).RuntimeID,
			Available: true, Enabled: true, CheckState: dto.AgentUpdateCheckStateUpdateAvailable,
			EffectiveVersion: "1.0.0", LatestVersion: "9.0.0",
			LastOutcome: &managedruntime.UpdateOutcome{ID: "attempt", Status: "interrupted", PreviousVersion: "1.0.0", TargetVersion: "9.0.0"},
		})
		synctest.Wait()
		individual, batches := notices.snapshot()
		if len(individual) != 1 || individual[0].Status != "interrupted" || len(batches) != 0 {
			t.Fatalf("terminal outcome waited for availability window: individual=%+v batches=%+v", individual, batches)
		}

		stop()
		time.Sleep(runtimeUpdateAvailabilityWindow)
		synctest.Wait()
		individual, batches = notices.snapshot()
		if len(individual) != 1 || len(batches) != 0 {
			t.Fatalf("shutdown delivered pending availability: individual=%+v batches=%+v", individual, batches)
		}
	})
}
