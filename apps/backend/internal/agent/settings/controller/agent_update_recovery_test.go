package controller

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"go.uber.org/zap"
)

type recoverySelectionStore struct {
	mu      sync.Mutex
	values  map[string]managedruntime.Selection
	err     error
	saveErr error
	events  *[]string
}

func newRecoverySelectionStore() *recoverySelectionStore {
	return &recoverySelectionStore{values: make(map[string]managedruntime.Selection)}
}

func (s *recoverySelectionStore) Get(
	_ context.Context,
	agentName string,
	packageName string,
) (managedruntime.Selection, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return managedruntime.Selection{}, false, s.err
	}
	selection, ok := s.values[agentName+"\x00"+packageName]
	return selection, ok, nil
}

func (s *recoverySelectionStore) Save(
	_ context.Context,
	agentName string,
	packageName string,
	version string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.values[agentName+"\x00"+packageName] = managedruntime.Selection{
		Package: packageName,
		Version: version,
	}
	if s.events != nil {
		*s.events = append(*s.events, "persist")
	}
	return nil
}

func (s *recoverySelectionStore) Delete(
	_ context.Context,
	agentName string,
	packageName string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	delete(s.values, agentName+"\x00"+packageName)
	if s.events != nil {
		*s.events = append(*s.events, "delete")
	}
	return nil
}

type recoveryRuntimeUpdater struct {
	mu            sync.Mutex
	metadata      RuntimeVersionMetadata
	metadataCalls int
	resolveErr    error
	current       hostutility.AgentCapabilities
	currentFound  bool
	probeCaps     hostutility.AgentCapabilities
	probeErr      error
	runErrs       []error
	updateOutput  string
	runCalls      int
	prepare       []string
	probe         []string
	invalidate    []string
	events        *[]string
}

func (u *recoveryRuntimeUpdater) CurrentCapabilities(string) (hostutility.AgentCapabilities, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.current, u.currentFound
}

func (u *recoveryRuntimeUpdater) ResolveTarget(context.Context, string) (string, error) {
	return u.metadata.Latest, u.resolveErr
}

func (u *recoveryRuntimeUpdater) ResolveVersions(context.Context, string) (RuntimeVersionMetadata, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.metadataCalls++
	return u.metadata, nil
}

func (u *recoveryRuntimeUpdater) RunUpdate(
	_ context.Context,
	command agents.Command,
	onChunk func(string),
) error {
	u.mu.Lock()
	u.runCalls++
	u.prepare = append(u.prepare, strings.Join(command.Args(), " "))
	index := u.runCalls - 1
	var err error
	if index < len(u.runErrs) {
		err = u.runErrs[index]
	}
	output := u.updateOutput
	u.mu.Unlock()
	if output == "" {
		output = "prepared\n"
	}
	onChunk(output)
	return err
}

func (u *recoveryRuntimeUpdater) InvalidateExecutionCache(context.Context, string) error {
	return nil
}

func (u *recoveryRuntimeUpdater) InvalidateExecutionCacheVersion(
	_ context.Context,
	packageName string,
	version string,
) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.invalidate = append(u.invalidate, packageName+"@"+version)
	return nil
}

func (u *recoveryRuntimeUpdater) Refresh(
	context.Context,
	string,
	agents.Command,
) (hostutility.AgentCapabilities, error) {
	return hostutility.AgentCapabilities{}, errors.New("legacy refresh must not be used")
}

func (u *recoveryRuntimeUpdater) Probe(
	_ context.Context,
	_ string,
	command agents.Command,
) (hostutility.AgentCapabilities, error) {
	u.mu.Lock()
	u.probe = append(u.probe, strings.Join(command.Args(), " "))
	if u.events != nil {
		*u.events = append(*u.events, "probe")
	}
	caps, err := u.probeCaps, u.probeErr
	u.mu.Unlock()
	return caps, err
}

func (u *recoveryRuntimeUpdater) PublishCapabilities(
	_ string,
	_ hostutility.AgentCapabilities,
) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.events != nil {
		*u.events = append(*u.events, "publish")
	}
}

func newRecoveryStore(
	updater RuntimeUpdater,
	selectionStore managedruntime.SelectionStore,
) (*AgentUpdateJobStore, <-chan dto.AgentUpdateJobDTO) {
	hub := newUpdateTerminalBroadcaster()
	return NewAgentUpdateJobStore(
		hub,
		zap.NewNop(),
		updater,
		newMaintenanceCoordinator(),
		nil,
		selectionStore,
	), hub.completed
}

func TestAgentUpdateExactCandidatePersistsBeforePublishing(t *testing.T) {
	selectionStore := newRecoverySelectionStore()
	selectionStore.values["managed-acp\x00@example/managed-acp"] = managedruntime.Selection{
		Package: "@example/managed-acp",
		Version: "1.0.3",
	}
	events := make([]string, 0, 4)
	selectionStore.events = &events
	updater := &recoveryRuntimeUpdater{
		metadata: RuntimeVersionMetadata{
			Versions: []string{"1.0.1", "1.0.2", "1.0.3"},
			Latest:   "1.0.3",
		},
		current:      hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.3"},
		currentFound: true,
		probeCaps:    hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.2"},
		resolveErr:   errors.New("registry unavailable"),
	}
	updater.events = &events
	store, completed := newRecoveryStore(updater, selectionStore)

	job, err := store.Enqueue("managed-acp", managedRuntimeSpec(), "1.0.2")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusSucceeded)
	if final.Operation != string(managedruntime.OperationRollback) {
		t.Fatalf("operation = %q, want rollback", final.Operation)
	}
	if final.ActiveVersion != "1.0.2" {
		t.Fatalf("active version = %q, want 1.0.2", final.ActiveVersion)
	}
	if final.CurrentVersion != "1.0.2" {
		t.Fatalf("current version = %q, want probed 1.0.2", final.CurrentVersion)
	}
	if len(updater.prepare) != 1 || !strings.Contains(updater.prepare[0], "--package=@example/managed-acp@1.0.2") {
		t.Fatalf("prepare commands = %#v", updater.prepare)
	}
	if len(updater.probe) != 1 || !strings.Contains(updater.probe[0], "@example/managed-acp@1.0.2") {
		t.Fatalf("probe commands = %#v", updater.probe)
	}
	if len(updater.invalidate) != 0 {
		t.Fatalf("invalidate calls = %#v, want none", updater.invalidate)
	}
	if len(events) != 3 || events[0] != "probe" || events[1] != "persist" || events[2] != "publish" {
		t.Fatalf("activation order = %#v", events)
	}
}

func TestAgentUpdateUseDefaultDeletesSelectionAfterProbe(t *testing.T) {
	selectionStore := newRecoverySelectionStore()
	selectionStore.values["managed-acp\x00@example/managed-acp"] = managedruntime.Selection{
		Package: "@example/managed-acp",
		Version: "1.0.1",
	}
	events := make([]string, 0, 4)
	selectionStore.events = &events
	updater := &recoveryRuntimeUpdater{
		metadata: RuntimeVersionMetadata{
			Versions: []string{"1.0.1", "1.0.2"},
			Latest:   "1.0.2",
		},
		current:      hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.1"},
		currentFound: true,
		probeCaps:    hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.2"},
	}
	updater.events = &events
	store, completed := newRecoveryStore(updater, selectionStore)
	spec := managedRuntimeSpec()
	spec.DefaultVersion = "1.0.2"

	job, err := store.EnqueueDefault("managed-acp", spec)
	if err != nil {
		t.Fatalf("EnqueueDefault: %v", err)
	}
	final := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusSucceeded)
	if final.Operation != string(managedruntime.OperationUseDefault) {
		t.Fatalf("operation = %q, want use_default", final.Operation)
	}
	if final.ActiveVersion != "" || final.EffectiveVersion != "1.0.2" {
		t.Fatalf("final versions = active %q, effective %q", final.ActiveVersion, final.EffectiveVersion)
	}
	if final.CurrentVersion != "1.0.2" {
		t.Fatalf("current version = %q, want probed default 1.0.2", final.CurrentVersion)
	}
	if _, found, err := selectionStore.Get(context.Background(), "managed-acp", "@example/managed-acp"); err != nil || found {
		t.Fatalf("selection after reset = found %v, err %v; want absent", found, err)
	}
	if len(events) != 3 || events[0] != "probe" || events[1] != "delete" || events[2] != "publish" {
		t.Fatalf("reset activation order = %#v", events)
	}
}

func TestAgentUpdateExactTargetSkipsMetadataRefetch(t *testing.T) {
	selectionStore := newRecoverySelectionStore()
	updater := &recoveryRuntimeUpdater{
		metadata: RuntimeVersionMetadata{
			Versions: []string{"1.0.1", "1.0.2"},
			Latest:   "1.0.2",
		},
		current:      hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.1"},
		currentFound: true,
		probeCaps:    hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.2"},
	}
	store, completed := newRecoveryStore(updater, selectionStore)

	job, err := store.Enqueue("managed-acp", managedRuntimeSpec(), "1.0.2")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusSucceeded)

	updater.mu.Lock()
	metadataCalls := updater.metadataCalls
	updater.mu.Unlock()
	if metadataCalls != 0 {
		t.Fatalf("metadata calls = %d, want no refetch for a validated target", metadataCalls)
	}
}

func TestAgentUpdateExactCandidateNpmReleaseAgePolicySkipsCacheRepair(t *testing.T) {
	const packageName = "@agentclientprotocol/claude-agent-acp"
	selectionStore := newRecoverySelectionStore()
	selectionStore.values["claude-acp\x00"+packageName] = managedruntime.Selection{
		Package: packageName,
		Version: "0.80.0",
	}
	updater := &recoveryRuntimeUpdater{
		metadata: RuntimeVersionMetadata{
			Versions: []string{"0.80.0", "0.81.0"},
			Latest:   "0.81.0",
		},
		current: hostutility.AgentCapabilities{
			Status:       hostutility.StatusOK,
			AgentVersion: "0.80.0",
		},
		currentFound: true,
		runErrs:      []error{errors.New("exit status 1")},
		updateOutput: "npm error code ETARGET\nnpm error notarget No matching version found for @agentclientprotocol/claude-agent-acp@0.81.0 with a date before 9/22/2026, 12:28:47 PM.\n",
	}
	store, completed := newRecoveryStore(updater, selectionStore)
	spec := agents.ManagedNPMRuntimeSpec{
		Package:        packageName,
		DefaultVersion: "0.80.0",
		ACPArgs:        []string{"acp"},
	}
	job, err := store.Enqueue("claude-acp", spec, "0.81.0")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusFailed)
	if !strings.Contains(final.Error, "release-age policy") {
		t.Fatalf("Error = %q, want the npm release-age policy failure", final.Error)
	}
	if strings.Contains(final.Error, "12:28:47 PM") {
		t.Fatalf("Error leaked the raw npm date: %q", final.Error)
	}
	if updater.runCalls != 1 || len(updater.invalidate) != 0 || len(updater.probe) != 0 {
		t.Fatalf("policy failure calls: update=%d invalidation=%#v probe=%#v, want 1, none, none", updater.runCalls, updater.invalidate, updater.probe)
	}
	selection, found, err := selectionStore.Get(context.Background(), "claude-acp", packageName)
	if err != nil || !found || selection.Version != "0.80.0" {
		t.Fatalf("selection after policy failure = %#v, found=%v, err=%v", selection, found, err)
	}
}

func TestAgentUpdateFailedCandidatePreservesSelectionAndCapabilities(t *testing.T) {
	selectionStore := newRecoverySelectionStore()
	selectionStore.values["managed-acp\x00@example/managed-acp"] = managedruntime.Selection{
		Package: "@example/managed-acp",
		Version: "1.0.3",
	}
	events := make([]string, 0, 2)
	updater := &recoveryRuntimeUpdater{
		metadata: RuntimeVersionMetadata{
			Versions: []string{"1.0.2", "1.0.3"},
			Latest:   "1.0.3",
		},
		current:      hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.0.3"},
		currentFound: true,
		probeCaps: hostutility.AgentCapabilities{
			Status: hostutility.StatusFailed,
			Error:  "ACP initialization failed",
		},
	}
	updater.events = &events
	store, completed := newRecoveryStore(updater, selectionStore)
	job, err := store.Enqueue("managed-acp", managedRuntimeSpec(), "1.0.2")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusFailed)
	if !strings.Contains(final.Error, "ACP initialization failed") {
		t.Fatalf("error = %q", final.Error)
	}
	selection, found, err := selectionStore.Get(context.Background(), "managed-acp", "@example/managed-acp")
	if err != nil || !found || selection.Version != "1.0.3" {
		t.Fatalf("selection after failure = %#v, found=%v, err=%v", selection, found, err)
	}
	if len(events) != 1 || events[0] != "probe" {
		t.Fatalf("candidate publication events = %#v", updater.events)
	}
}

func TestAgentUpdateActiveHealthyTargetIsUpToDateWithoutPreparation(t *testing.T) {
	selectionStore := newRecoverySelectionStore()
	selectionStore.values["managed-acp\x00@example/managed-acp"] = managedruntime.Selection{
		Package: "@example/managed-acp",
		Version: "1.0.3",
	}
	updater := &recoveryRuntimeUpdater{
		metadata: RuntimeVersionMetadata{Versions: []string{"1.0.3"}, Latest: "1.0.3"},
		current: hostutility.AgentCapabilities{
			Status:       hostutility.StatusOK,
			AgentVersion: "1.0.3",
		},
		currentFound: true,
	}
	store, completed := newRecoveryStore(updater, selectionStore)
	job, err := store.Enqueue("managed-acp", managedRuntimeSpec(), "1.0.3")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	final := waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusSucceeded)
	if final.Operation != string(managedruntime.OperationUpToDate) {
		t.Fatalf("operation = %q, want up_to_date", final.Operation)
	}
	if updater.runCalls != 0 || len(updater.probe) != 0 {
		t.Fatalf("updater mutated up-to-date target: runs=%d probes=%d", updater.runCalls, len(updater.probe))
	}
}
