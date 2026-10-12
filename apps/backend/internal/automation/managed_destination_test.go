package automation

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestManagedConversationDestination(t *testing.T) {
	tests := []struct {
		name          string
		workflowID    string
		repositoryIDs []string
		wantErr       error
	}{
		{name: "explicit destination without task resources"},
		{name: "workflow is task-only", workflowID: "workflow-1", wantErr: ErrInvalidTaskMode},
		{name: "repository is task-only", repositoryIDs: []string{"repository-1"}, wantErr: ErrInvalidTaskMode},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAutomationTarget(TaskModeManagedConversation, RepositoryModeNone, test.workflowID, test.repositoryIDs)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("validate managed destination error = %v, want %v", err, test.wantErr)
			}
		})
	}

	t.Run("destination is required", func(t *testing.T) {
		_, err := newTestService(t).CreateAutomation(context.Background(), &CreateAutomationRequest{
			WorkspaceID: "ws-1",
			Name:        "managed schedule",
			TaskMode:    TaskModeManagedConversation,
		})
		if err == nil {
			t.Fatal("managed conversation automation without a destination was created")
		}
	})

	t.Run("destination survives storage round trip", func(t *testing.T) {
		ctx := context.Background()
		store := setupTestStore(t)
		target := &ManagedConversationDestination{PluginID: "coordinator", InstanceKey: "daily-brief", Revision: 3}
		automation := &Automation{
			WorkspaceID: "ws-1", Name: "managed schedule", TaskMode: TaskModeManagedConversation,
			ManagedDestination: target, Enabled: true, MaxConcurrentRuns: 1,
		}
		if err := store.CreateAutomation(ctx, automation); err != nil {
			t.Fatalf("create managed automation: %v", err)
		}
		got, err := store.GetAutomation(ctx, automation.ID)
		if err != nil {
			t.Fatalf("get managed automation: %v", err)
		}
		if got.ManagedDestination == nil || *got.ManagedDestination != *target {
			t.Fatalf("stored destination = %+v, want %+v", got.ManagedDestination, target)
		}
	})

	t.Run("delivery receipt survives run history storage", func(t *testing.T) {
		ctx := context.Background()
		store := setupTestStore(t)
		automation := &Automation{WorkspaceID: "ws-1", Name: "managed schedule", Enabled: true, MaxConcurrentRuns: 1}
		if err := store.CreateAutomation(ctx, automation); err != nil {
			t.Fatalf("create automation: %v", err)
		}
		run := &AutomationRun{
			AutomationID: automation.ID, TriggerID: "trigger-1", TriggerType: TriggerTypeScheduled,
			Status: RunStatusTriggered, ManagedInputID: "input-1", DeliveryStatus: ManagedDeliveryAccepted,
		}
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatalf("create run: %v", err)
		}
		got, err := store.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if got.ManagedInputID != run.ManagedInputID || got.DeliveryStatus != ManagedDeliveryAccepted {
			t.Fatalf("stored managed delivery = %q/%q, want %q/%q", got.ManagedInputID, got.DeliveryStatus, run.ManagedInputID, ManagedDeliveryAccepted)
		}
	})

	t.Run("startup reconciliation leaves managed delivery open", func(t *testing.T) {
		ctx := context.Background()
		svc := newTestService(t)
		svc.SetManagedConversationDestinationResolver(managedScheduleResolverFake{})
		automation, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
			WorkspaceID: "ws-1", Name: "managed schedule", TaskMode: TaskModeManagedConversation,
			ManagedDestination: &ManagedConversationDestination{PluginID: "coordinator", InstanceKey: "daily-brief", Revision: 1},
		})
		if err != nil {
			t.Fatalf("create automation: %v", err)
		}
		run := &AutomationRun{
			AutomationID: automation.ID, TriggerID: "trigger-1", TriggerType: TriggerTypeScheduled,
			Status: RunStatusTriggered, ManagedInputID: "input-1", DeliveryStatus: ManagedDeliveryAccepted,
		}
		if err := svc.store.CreateRun(ctx, run); err != nil {
			t.Fatalf("create run: %v", err)
		}
		if err := svc.ReconcileOpenRuns(ctx); err != nil {
			t.Fatalf("reconcile open runs: %v", err)
		}
		got, err := svc.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if got.Status != RunStatusTriggered || got.DeliveryStatus != ManagedDeliveryAccepted {
			t.Fatalf("managed run after restart = status %q, delivery %q; want triggered/accepted", got.Status, got.DeliveryStatus)
		}
	})

	t.Run("installation-owned schedules use CAS and durable replay", func(t *testing.T) {
		ctx := context.Background()
		svc := newTestService(t)
		svc.SetManagedConversationDestinationResolver(managedScheduleResolverFake{})
		schedule := managedScheduleFixture()
		created, replayed, err := svc.CreateManagedConversationSchedule(ctx, "install-owner", "coordinator", schedule, "op-create", "digest-create")
		if err != nil || replayed || created.ResourceRevision != 1 {
			t.Fatalf("create schedule = %+v, replayed=%t, err=%v", created, replayed, err)
		}
		items, err := svc.ListManagedConversationSchedules(ctx, "install-owner", "ws-1")
		if err != nil || len(items) != 1 || items[0].ID != created.ID {
			t.Fatalf("owned schedules = %+v, err=%v", items, err)
		}
		foreign, err := svc.ListManagedConversationSchedules(ctx, "install-other", "ws-1")
		if err != nil || len(foreign) != 0 {
			t.Fatalf("foreign schedules = %+v, err=%v", foreign, err)
		}

		updatedInput := created
		updatedInput.Name = "Updated daily brief"
		updatedInput.ResourceRevision = 1
		updated, alreadyApplied, err := svc.UpdateManagedConversationSchedule(ctx, "install-owner", "coordinator", "ws-1", created.ID, 1, updatedInput, "op-update", "digest-update")
		if err != nil || alreadyApplied || updated.ResourceRevision != 2 || updated.Name != updatedInput.Name {
			t.Fatalf("update schedule = %+v, replayed=%t, err=%v", updated, alreadyApplied, err)
		}
		retried, alreadyApplied, err := svc.UpdateManagedConversationSchedule(ctx, "install-owner", "coordinator", "ws-1", created.ID, 1, updatedInput, "op-update", "digest-update")
		if err != nil || !alreadyApplied || retried.ResourceRevision != 2 {
			t.Fatalf("retry update = %+v, replayed=%t, err=%v", retried, alreadyApplied, err)
		}
		stale := updatedInput
		stale.Name = "stale update"
		if _, _, err := svc.UpdateManagedConversationSchedule(ctx, "install-owner", "coordinator", "ws-1", created.ID, 1, stale, "op-stale", "digest-stale"); !errors.Is(err, ErrManagedScheduleRevisionConflict) {
			t.Fatalf("stale update error = %v, want revision conflict", err)
		}
		if _, _, err := svc.UpdateManagedConversationSchedule(ctx, "install-other", "coordinator", "ws-1", created.ID, 2, updatedInput, "op-foreign", "digest-foreign"); !errors.Is(err, ErrManagedScheduleNotFound) {
			t.Fatalf("foreign update error = %v, want not found", err)
		}

		paused, replayed, err := svc.SetManagedConversationScheduleEnabled(ctx, "install-owner", "ws-1", created.ID, 2, false, "op-pause", "digest-pause")
		if err != nil || replayed || paused.Enabled || paused.ResourceRevision != 3 {
			t.Fatalf("pause schedule = %+v, replayed=%t, err=%v", paused, replayed, err)
		}
		if _, err := svc.DeleteManagedConversationSchedule(ctx, "install-owner", "ws-1", created.ID, 2, "op-delete-stale", "digest-delete-stale"); !errors.Is(err, ErrManagedScheduleRevisionConflict) {
			t.Fatalf("stale delete error = %v, want revision conflict", err)
		}
		if _, err := svc.DeleteManagedConversationSchedule(ctx, "install-owner", "ws-1", created.ID, 3, "op-delete", "digest-delete"); err != nil {
			t.Fatalf("delete schedule: %v", err)
		}
		if _, err := svc.DeleteManagedConversationSchedule(ctx, "install-owner", "ws-1", created.ID, 3, "op-delete", "digest-delete"); err != nil {
			t.Fatalf("retry delete: %v", err)
		}
		if item, err := svc.store.GetAutomation(ctx, created.ID); err != nil || item != nil {
			t.Fatalf("deleted schedule readback = %+v, err=%v", item, err)
		}
	})

	t.Run("firing retries reuse the run occurrence and read back completion", func(t *testing.T) {
		ctx := context.Background()
		svc := newTestService(t)
		svc.SetManagedConversationDestinationResolver(managedScheduleResolverFake{})
		delivery := &managedAutomationDeliveryFake{loseFirstEnqueueAck: true}
		svc.SetManagedConversationAutomationDelivery(delivery)
		schedule, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
			WorkspaceID: "ws-1", Name: "Daily brief", Prompt: "Summarize blocked work",
			TaskMode:           TaskModeManagedConversation,
			ManagedDestination: &ManagedConversationDestination{PluginID: "coordinator", InstanceKey: "daily-brief", Revision: 4},
			Triggers:           []CreateTriggerSpec{{Type: TriggerTypeScheduled, Config: []byte(`{"cron_expression":"0 9 * * 1-5"}`), Enabled: true}},
		})
		if err != nil {
			t.Fatalf("create managed schedule: %v", err)
		}
		run, skipReason, duplicate, err := svc.admitTrigger(ctx, schedule, schedule.Triggers[0].ID,
			TriggerTypeScheduled, []byte(`{"summary":"queue work"}`), DedupNotConfigured())
		if err != nil || run == nil || skipReason != "" || duplicate {
			t.Fatalf("admit run = %+v, skip=%q, duplicate=%t, err=%v", run, skipReason, duplicate, err)
		}
		if err := svc.DispatchManagedAutomationRun(ctx, run.ID); err != nil {
			t.Fatalf("first dispatch: %v", err)
		}
		first, err := svc.GetRun(ctx, run.ID)
		if err != nil || first.DeliveryStatus != ManagedDeliveryUnavailable || first.ManagedInputID != "" || first.DeliveryAttempts != 1 {
			t.Fatalf("lost-ack run receipt = %+v, err=%v", first, err)
		}
		if err := svc.DispatchManagedAutomationRun(ctx, run.ID); err != nil {
			t.Fatalf("retry dispatch: %v", err)
		}
		second, err := svc.GetRun(ctx, run.ID)
		if err != nil || second.DeliveryStatus != ManagedDeliveryAccepted || second.ManagedInputID != "input-1" ||
			second.TaskID != "" || second.ManagedConversationID != "conversation-task" {
			t.Fatalf("accepted managed run = %+v, err=%v", second, err)
		}
		if delivery.admissions != 1 || delivery.occurrences[0] != run.ID || delivery.occurrences[1] != run.ID {
			t.Fatalf("managed admissions = %d, occurrence keys = %#v; want one admission and same run ID on retry", delivery.admissions, delivery.occurrences)
		}
		if len(delivery.enqueuedDestinations) != 2 || delivery.enqueuedDestinations[0].conversationID != "conversation-task" {
			t.Fatalf("enqueue destinations = %#v, want the admitted conversation on each retry", delivery.enqueuedDestinations)
		}
		delivery.setState(pluginsdk.ManagedAgentInputCompleted)
		if err := svc.ReconcileManagedConversationDeliveries(ctx); err != nil {
			t.Fatalf("reconcile completed input: %v", err)
		}
		completed, err := svc.GetRun(ctx, run.ID)
		if err != nil || completed.DeliveryStatus != ManagedDeliveryCompleted || completed.Status != RunStatusSucceeded {
			t.Fatalf("completed managed run = %+v, err=%v", completed, err)
		}
		if len(delivery.readDestinations) != 1 || delivery.readDestinations[0].conversationID != "conversation-task" {
			t.Fatalf("receipt destinations = %#v, want the admitted conversation", delivery.readDestinations)
		}
	})

	t.Run("admitted occurrence remains bound after destination rebinding", func(t *testing.T) {
		ctx := context.Background()
		svc := newTestService(t)
		svc.SetManagedConversationDestinationResolver(switchingManagedScheduleResolverFake{})
		delivery := &managedAutomationDeliveryFake{}
		svc.SetManagedConversationAutomationDelivery(delivery)
		schedule, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
			WorkspaceID: "ws-1", Name: "Daily brief", Prompt: "Summarize blocked work",
			TaskMode: TaskModeManagedConversation, ManagedDestination: &ManagedConversationDestination{
				PluginID: "coordinator", InstanceKey: "instance-a", Revision: 4,
			},
			Triggers: []CreateTriggerSpec{{Type: TriggerTypeScheduled, Enabled: true}},
		})
		if err != nil {
			t.Fatalf("create managed schedule: %v", err)
		}
		run, skipReason, duplicate, err := svc.admitTrigger(ctx, schedule, schedule.Triggers[0].ID,
			TriggerTypeScheduled, []byte(`{}`), DedupNotConfigured())
		if err != nil || run == nil || skipReason != "" || duplicate {
			t.Fatalf("admit run = %+v, skip=%q, duplicate=%t, err=%v", run, skipReason, duplicate, err)
		}
		if run.ManagedDestinationInstallationID != "install-instance-a" || run.ManagedDestinationPluginID != "coordinator" ||
			run.ManagedDestinationInstanceKey != "instance-a" || run.ManagedConversationID != "conversation-instance-a" {
			t.Fatalf("admitted destination snapshot = %+v", run)
		}
		newDestination := &ManagedConversationDestination{PluginID: "coordinator", InstanceKey: "instance-b", Revision: 5}
		if _, err := svc.UpdateAutomation(ctx, schedule.ID, &UpdateAutomationRequest{ManagedDestination: newDestination}); err != nil {
			t.Fatalf("rebind schedule: %v", err)
		}

		if err := svc.DispatchManagedAutomationRun(ctx, run.ID); err != nil {
			t.Fatalf("dispatch admitted run: %v", err)
		}
		if len(delivery.enqueuedDestinations) != 1 || delivery.enqueuedDestinations[0] != (managedDeliveryDestination{
			installationID: "install-instance-a", pluginID: "coordinator", instanceKey: "instance-a",
			conversationID: "conversation-instance-a", revision: 4,
		}) {
			t.Fatalf("enqueue destination = %#v, want original instance A", delivery.enqueuedDestinations)
		}
		if err := svc.DispatchManagedAutomationRun(ctx, run.ID); err != nil {
			t.Fatalf("observe admitted run: %v", err)
		}
		if len(delivery.readDestinations) != 1 || delivery.readDestinations[0] != delivery.enqueuedDestinations[0] {
			t.Fatalf("receipt destination = %#v, want original instance A", delivery.readDestinations)
		}
	})

	t.Run("receipt read failures do not consume enqueue retry attempts", func(t *testing.T) {
		ctx := context.Background()
		svc := newTestService(t)
		svc.SetManagedConversationDestinationResolver(managedScheduleResolverFake{})
		delivery := &managedAutomationDeliveryFake{readErrorsRemaining: 3}
		svc.SetManagedConversationAutomationDelivery(delivery)
		schedule, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
			WorkspaceID: "ws-1", Name: "Daily brief", TaskMode: TaskModeManagedConversation,
			ManagedDestination: &ManagedConversationDestination{PluginID: "coordinator", InstanceKey: "daily-brief", Revision: 4},
		})
		if err != nil {
			t.Fatalf("create managed schedule: %v", err)
		}
		run := &AutomationRun{
			AutomationID: schedule.ID, TriggerID: "trigger-1", TriggerType: TriggerTypeScheduled,
			Status: RunStatusTriggered, ManagedInputID: "input-1", DeliveryAttempts: managedAutomationDeliveryAttemptLimit,
		}
		snapshotManagedAutomationDestination(schedule, run)
		run.DeliveryStatus = ManagedDeliveryAccepted
		run.DeliveryAttempts = managedAutomationDeliveryAttemptLimit
		if err := svc.store.CreateRun(ctx, run); err != nil {
			t.Fatalf("create accepted run: %v", err)
		}
		for attempt := 0; attempt < 3; attempt++ {
			if err := svc.DispatchManagedAutomationRun(ctx, run.ID); err != nil {
				t.Fatalf("read receipt attempt %d: %v", attempt+1, err)
			}
			got, err := svc.GetRun(ctx, run.ID)
			if err != nil || got.Status != RunStatusTriggered || got.DeliveryAttempts != managedAutomationDeliveryAttemptLimit || got.DeliveryStatus != ManagedDeliveryUnavailable {
				t.Fatalf("run after receipt read failure %d = %+v, err=%v", attempt+1, got, err)
			}
		}
		delivery.setState(pluginsdk.ManagedAgentInputCompleted)
		if err := svc.DispatchManagedAutomationRun(ctx, run.ID); err != nil {
			t.Fatalf("read recovered receipt: %v", err)
		}
		got, err := svc.GetRun(ctx, run.ID)
		if err != nil || got.Status != RunStatusSucceeded || got.DeliveryStatus != ManagedDeliveryCompleted || got.DeliveryAttempts != managedAutomationDeliveryAttemptLimit {
			t.Fatalf("run after receipt recovery = %+v, err=%v", got, err)
		}
	})
}

type managedScheduleResolverFake struct{}

func (managedScheduleResolverFake) ResolveManagedConversationDestination(_ context.Context, _, _, _ string, _ uint64) (string, string, bool, error) {
	return "install-target", "conversation-task", false, nil
}

func managedScheduleFixture() pluginsdk.ManagedConversationSchedule {
	return pluginsdk.ManagedConversationSchedule{
		ID: "schedule-1", WorkspaceID: "ws-1", Name: "Daily brief", Prompt: "Summarize blocked work",
		PluginID: "coordinator", InstanceKey: "daily-brief", DestinationRevision: 4,
		Enabled: true, Triggers: []pluginsdk.ManagedConversationScheduleTrigger{{
			Type: string(TriggerTypeScheduled), Config: []byte(`{"cron_expression":"0 9 * * 1-5"}`), Enabled: true,
		}},
	}
}

type managedAutomationDeliveryFake struct {
	mu                   sync.Mutex
	loseFirstEnqueueAck  bool
	admissions           int
	occurrences          []string
	enqueuedDestinations []managedDeliveryDestination
	readDestinations     []managedDeliveryDestination
	readErrorsRemaining  int
	state                pluginsdk.ManagedAgentInputState
}

type managedDeliveryDestination struct {
	installationID string
	pluginID       string
	instanceKey    string
	conversationID string
	revision       uint64
}

func managedDeliveryDestinationFrom(a *Automation) managedDeliveryDestination {
	if a == nil || a.ManagedDestination == nil {
		return managedDeliveryDestination{}
	}
	return managedDeliveryDestination{
		installationID: a.ManagedDestinationInstallationID, pluginID: a.ManagedDestination.PluginID,
		instanceKey: a.ManagedDestination.InstanceKey, conversationID: a.ManagedDestinationConversationID,
		revision: a.ManagedDestination.Revision,
	}
}

func (f *managedAutomationDeliveryFake) EnqueueManagedAutomationInput(_ context.Context, schedule *Automation, occurrenceID, _ string) (ManagedAutomationInputReceipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.occurrences = append(f.occurrences, occurrenceID)
	f.enqueuedDestinations = append(f.enqueuedDestinations, managedDeliveryDestinationFrom(schedule))
	if f.admissions == 0 {
		f.admissions++
		f.state = pluginsdk.ManagedAgentInputAccepted
		if f.loseFirstEnqueueAck {
			f.loseFirstEnqueueAck = false
			return ManagedAutomationInputReceipt{}, errors.New("simulated lost enqueue acknowledgement")
		}
	}
	return ManagedAutomationInputReceipt{InputID: "input-1", State: string(f.state)}, nil
}

func (f *managedAutomationDeliveryFake) ReadManagedAutomationInput(_ context.Context, schedule *Automation, inputID string) (ManagedAutomationInputReceipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readDestinations = append(f.readDestinations, managedDeliveryDestinationFrom(schedule))
	if f.readErrorsRemaining > 0 {
		f.readErrorsRemaining--
		return ManagedAutomationInputReceipt{}, errors.New("temporary receipt read failure")
	}
	return ManagedAutomationInputReceipt{InputID: inputID, State: string(f.state)}, nil
}

func (f *managedAutomationDeliveryFake) setState(state pluginsdk.ManagedAgentInputState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = state
}

type switchingManagedScheduleResolverFake struct{}

func (switchingManagedScheduleResolverFake) ResolveManagedConversationDestination(_ context.Context, _, _, instanceKey string, _ uint64) (string, string, bool, error) {
	return "install-" + instanceKey, "conversation-" + instanceKey, false, nil
}
