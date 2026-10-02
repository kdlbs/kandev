package controller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type autoMemorySettings struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (s *autoMemorySettings) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[key]
	return append([]byte(nil), v...), ok, nil
}
func (s *autoMemorySettings) Save(_ context.Context, key string, v []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = map[string][]byte{}
	}
	s.values[key] = append([]byte(nil), v...)
	return nil
}
func (s *autoMemorySettings) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.1, AC-AGENTS-RUNTIME-NOTIFY-002.3
func TestAutomaticUpdateUsesValidatedActivationOnlyAfterConsent(t *testing.T) {
	for _, ag := range []agents.Agent{agents.NewGemini(), agents.NewCodexAppServer(true)} {
		t.Run(ag.ID(), func(t *testing.T) {
			c := newTestController(map[string]agents.Agent{ag.ID(): ag})
			spec := ag.(agents.ManagedNPMRuntimeAgent).ManagedNPMRuntime()
			previous := spec.DefaultVersionOrPinned()
			updater := &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0", previous}}, currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: previous}, probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "9.0.0"}}
			selection := newRecoverySelectionStore()
			settings := &autoMemorySettings{}
			c.SetRuntimeAutoUpdateStore(managedruntime.NewAutoUpdateStore(settings))
			c.SetRuntimeUpdater(updater)
			c.SetManagedRuntimeSelectionStore(selection)
			broadcaster := newUpdateTerminalBroadcaster()
			c.SetJobBroadcaster(broadcaster)
			c.updateJobStore.onRefresh = nil
			ctx := context.Background()
			if err := c.RunRuntimeUpdatePass(ctx); err != nil {
				t.Fatal(err)
			}
			if updater.runCalls != 0 {
				t.Fatal("default-off policy ran installer")
			}
			if err := c.SetAgentAutomaticUpdates(ctx, ag.ID(), true); err != nil {
				t.Fatal(err)
			}
			if err := c.RunRuntimeUpdatePass(ctx); err != nil {
				t.Fatal(err)
			}
			jobs := c.ListAgentUpdateJobs()
			if len(jobs) != 1 {
				t.Fatalf("automatic jobs = %d, want one after consent", len(jobs))
			}
			job := waitForUpdateStatus(t, broadcaster.completed, jobs[0].JobID, dto.AgentUpdateJobStatusSucceeded)
			if !job.Automatic || job.PreviousVersion != previous {
				t.Fatalf("origin/version lost: %+v", job)
			}
			policy, err := c.runtimeAutoUpdateStore.Get(ctx, ag.ID(), "npm:"+spec.Package)
			if err != nil || policy.Outcome == nil || policy.Outcome.Status != "succeeded" {
				t.Fatalf("durable outcome: %+v, %v", policy, err)
			}
			got, ok, err := selection.Get(ctx, ag.ID(), spec.Package)
			if err != nil || !ok || got.Version != "9.0.0" {
				t.Fatalf("automatic activation: %+v,%v,%v", got, ok, err)
			}
		})
	}
}

type blockedAutoUpdater struct {
	*recoveryRuntimeUpdater
	probed  chan struct{}
	release chan struct{}
}

func (u *blockedAutoUpdater) Probe(ctx context.Context, name string, command agents.Command) (hostutility.AgentCapabilities, error) {
	close(u.probed)
	select {
	case <-u.release:
	case <-ctx.Done():
		return hostutility.AgentCapabilities{}, ctx.Err()
	}
	return u.recoveryRuntimeUpdater.Probe(ctx, name, command)
}

func autoController(t *testing.T, updater RuntimeUpdater, selection managedruntime.SelectionStore, settings *autoMemorySettings) (*Controller, *updateTerminalBroadcaster) {
	t.Helper()
	ag := agents.NewGemini()
	c := newTestController(map[string]agents.Agent{ag.ID(): ag})
	c.SetRuntimeAutoUpdateStore(managedruntime.NewAutoUpdateStore(settings))
	c.SetRuntimeUpdater(updater)
	c.SetManagedRuntimeSelectionStore(selection)
	broadcaster := newUpdateTerminalBroadcaster()
	c.SetJobBroadcaster(broadcaster)
	c.updateJobStore.onRefresh = nil
	return c, broadcaster
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.5
func TestAutomaticCandidateCannotSurviveWithdrawalAndReenable(t *testing.T) {
	spec := agents.NewGemini().ManagedNPMRuntime()
	previous := spec.DefaultVersionOrPinned()
	updater := &blockedAutoUpdater{recoveryRuntimeUpdater: &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0", previous}}, currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: previous}, probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "9.0.0"}}, probed: make(chan struct{}), release: make(chan struct{})}
	selection := newRecoverySelectionStore()
	c, hub := autoController(t, updater, selection, &autoMemorySettings{})
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "gemini", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	waitForRuntimeSignal(t, updater.probed, "candidate probe")
	if err := c.SetAgentAutomaticUpdates(ctx, "gemini", false); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAgentAutomaticUpdates(ctx, "gemini", true); err != nil {
		t.Fatal(err)
	}
	close(updater.release)
	job := waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusFailed)
	if job.CurrentVersion != previous {
		t.Fatalf("failed activation replaced version: %+v", job)
	}
	if got, found, err := selection.Get(ctx, "gemini", spec.Package); err != nil || found {
		t.Fatalf("withdrawn candidate activated: %+v,%v,%v", got, found, err)
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.3, AC-AGENTS-RUNTIME-NOTIFY-002.6
func TestAutomaticFailurePreservesSelectionAndDoesNotRetryAfterReload(t *testing.T) {
	for _, failure := range []string{"preparation", "probe", "persistence"} {
		t.Run(failure, func(t *testing.T) {
			spec := agents.NewGemini().ManagedNPMRuntime()
			events := []string{}
			updater := &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0", "8.0.0"}}, currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "8.0.0"}, probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "9.0.0"}, events: &events}
			selection := newRecoverySelectionStore()
			if err := selection.Save(context.Background(), "gemini", spec.Package, "8.0.0"); err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "preparation":
				updater.runErrs = []error{errors.New("download unavailable"), errors.New("retry unavailable")}
			case "probe":
				updater.probeErr = errors.New("invalid candidate")
			case "persistence":
				selection.saveErr = errors.New("disk unavailable")
			}
			settings := &autoMemorySettings{}
			c, hub := autoController(t, updater, selection, settings)
			ctx := context.Background()
			if err := c.SetAgentAutomaticUpdates(ctx, "gemini", true); err != nil {
				t.Fatal(err)
			}
			if err := c.RunRuntimeUpdatePass(ctx); err != nil {
				t.Fatal(err)
			}
			job := waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusFailed)
			if job.CurrentVersion != "8.0.0" {
				t.Fatalf("failed job version: %+v", job)
			}
			got, found, err := selection.Get(ctx, "gemini", spec.Package)
			if err != nil || !found || got.Version != "8.0.0" {
				t.Fatalf("rollback state: %+v,%v,%v", got, found, err)
			}
			if slices.Contains(events, "publish") {
				t.Fatal("invalid candidate was published")
			}
			reloaded, _ := autoController(t, updater, selection, settings)
			if err := reloaded.RunRuntimeUpdatePass(ctx); err != nil {
				t.Fatal(err)
			}
			if len(reloaded.ListAgentUpdateJobs()) != 0 {
				t.Fatal("same failed target retried after reload")
			}
			statuses, err := reloaded.ListAgentUpdateStatuses(ctx)
			if err != nil || statuses.Statuses[0].LastOutcome == nil || statuses.Statuses[0].LastOutcome.Status != "failed" {
				t.Fatalf("lost failure after reload: %+v,%v", statuses, err)
			}
		})
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.2
func TestNativeOpenCodeManualAPIRejectsUnverifiedFallback(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(binary))
	ag := agents.NewOpenCodeACP()
	c := newTestController(map[string]agents.Agent{ag.ID(): ag})
	base := &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0"}}}
	c.SetRuntimeUpdater(candidateWithoutCatalogue{RuntimeUpdater: base, candidate: base})
	c.SetManagedRuntimeSelectionStore(newRecoverySelectionStore())
	c.SetJobBroadcaster(newUpdateTerminalBroadcaster())
	if _, err := c.PreviewAgentUpdate(context.Background(), ag.ID(), "9.0.0"); !errors.Is(err, ErrRuntimeUpdateUnsupported) {
		t.Fatalf("external preview: %v", err)
	}
	if _, err := c.EnqueueAgentUpdate(context.Background(), ag.ID(), "9.0.0"); !errors.Is(err, ErrRuntimeUpdateUnsupported) {
		t.Fatalf("external mutation: %v", err)
	}
	if len(c.ListAgentUpdateJobs()) != 0 {
		t.Fatal("external runtime created update job")
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.3, AC-AGENTS-RUNTIME-NOTIFY-002.6
func TestAutomaticCatalogueValidationFailureIsRetainedWithoutMutation(t *testing.T) {
	updater := &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"8.0.0"}}}
	c, _ := autoController(t, updater, newRecoverySelectionStore(), &autoMemorySettings{})
	c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { return "9.0.0", nil })
	notices := &runtimeNoticeCapture{}
	c.SetRuntimeUpdateNotifier(notices)
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "gemini", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	spec := agents.NewGemini().ManagedNPMRuntime()
	policy, err := c.runtimeAutoUpdateStore.Get(ctx, "gemini", "npm:"+spec.Package)
	if err != nil || policy.Outcome == nil || policy.Outcome.Status != "failed" || policy.AttemptedVersion != "9.0.0" {
		t.Fatalf("validation failure disappeared: %+v,%v", policy, err)
	}
	if notices.count() != 2 || notices.notices[1].Status != "failed" {
		t.Fatalf("validation failure was not announced immediately: %+v", notices.notices)
	}
	if updater.runCalls != 0 {
		t.Fatal("unpublished candidate ran")
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.6
func TestAutomaticInterruptedAttemptSettlesEvenWhenSourceUnavailable(t *testing.T) {
	settings := &autoMemorySettings{}
	c, _ := autoController(t, &recoveryRuntimeUpdater{}, newRecoverySelectionStore(), settings)
	c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { return "", errors.New("offline") })
	ctx := context.Background()
	spec := agents.NewGemini().ManagedNPMRuntime()
	policy := managedruntime.AutoUpdatePolicy{RuntimeID: "npm:" + spec.Package, Enabled: true, AttemptedVersion: "9.0.0", Outcome: &managedruntime.UpdateOutcome{ID: "old-process", Status: "running", PreviousVersion: "8.0.0", TargetVersion: "9.0.0"}}
	if err := c.runtimeAutoUpdateStore.Save(ctx, "gemini", policy); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := c.runtimeAutoUpdateStore.Get(ctx, "gemini", policy.RuntimeID)
	if err != nil || got.Outcome.Status != "interrupted" || got.Outcome.FinishedAt.IsZero() {
		t.Fatalf("interrupted outcome: %+v,%v", got, err)
	}
	if len(c.ListAgentUpdateJobs()) != 0 {
		t.Fatal("offline restart scheduled mutation")
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.2
func TestAutomaticActivationReusesTheCheckedSourceCatalogue(t *testing.T) {
	previous := agents.NewGemini().ManagedNPMRuntime().DefaultVersionOrPinned()
	updater := &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0", previous}}, currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: previous}, probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "9.0.0"}}
	c, hub := autoController(t, updater, newRecoverySelectionStore(), &autoMemorySettings{})
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "gemini", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	waitForUpdateStatus(t, hub.completed, c.ListAgentUpdateJobs()[0].JobID, dto.AgentUpdateJobStatusSucceeded)
	updater.mu.Lock()
	calls := updater.metadataCalls
	updater.mu.Unlock()
	if calls != 1 {
		t.Fatalf("source catalogue requests=%d, want one shared lookup", calls)
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.6
func TestAutomaticFinishingJobIsNotReportedAsInterrupted(t *testing.T) {
	previous := agents.NewGemini().ManagedNPMRuntime().DefaultVersionOrPinned()
	updater := &recoveryRuntimeUpdater{metadata: RuntimeVersionMetadata{Latest: "9.0.0", Versions: []string{"9.0.0", previous}}, currentFound: true, current: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: previous}, probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "9.0.0"}}
	c, hub := autoController(t, updater, newRecoverySelectionStore(), &autoMemorySettings{})
	finishing, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(func() { releaseCallback(); c.updateJobStore.automaticWorkers.Wait() })
	original := c.updateJobStore.onFinished
	c.updateJobStore.onFinished = func(job dto.AgentUpdateJobDTO) { close(finishing); <-release; original(job) }
	ctx := context.Background()
	if err := c.SetAgentAutomaticUpdates(ctx, "gemini", true); err != nil {
		t.Fatal(err)
	}
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	waitForRuntimeSignal(t, finishing, "outcome retention")
	if err := c.RunRuntimeUpdatePass(ctx); err != nil {
		t.Fatal(err)
	}
	spec := agents.NewGemini().ManagedNPMRuntime()
	policy, err := c.runtimeAutoUpdateStore.Get(ctx, "gemini", "npm:"+spec.Package)
	if err != nil || policy.Outcome.Status != "running" {
		t.Fatalf("finishing job mistaken for process loss: %+v,%v", policy, err)
	}
	releaseCallback()
	waitForUpdateStatus(t, hub.completed, policy.Outcome.ID, dto.AgentUpdateJobStatusSucceeded)
}

type candidateWithoutCatalogue struct {
	RuntimeUpdater
	candidate RuntimeCandidateUpdater
}

func (u candidateWithoutCatalogue) Probe(ctx context.Context, name string, command agents.Command) (hostutility.AgentCapabilities, error) {
	return u.candidate.Probe(ctx, name, command)
}
func (u candidateWithoutCatalogue) PublishCapabilities(name string, caps hostutility.AgentCapabilities) {
	u.candidate.PublishCapabilities(name, caps)
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.2
func TestAutomaticPolicyRequiresVerifiedReleaseCatalogue(t *testing.T) {
	base := &recoveryRuntimeUpdater{}
	c, _ := autoController(t, candidateWithoutCatalogue{RuntimeUpdater: base, candidate: base}, newRecoverySelectionStore(), &autoMemorySettings{})
	if err := c.SetAgentAutomaticUpdates(context.Background(), "gemini", true); !errors.Is(err, ErrRuntimeUpdateUnsupported) {
		t.Fatalf("unverified catalogue accepted automatic consent: %v", err)
	}
}
