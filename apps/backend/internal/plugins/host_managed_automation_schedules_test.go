package plugins

import (
	"context"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestManagedConversationScheduleHostAuthorizationAndReplay(t *testing.T) {
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open command database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	commandStore, err := state.NewCommandStore(db.NewPool(connection, connection))
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	record := &pluginstore.Record{
		Manifest: manifest.Manifest{ID: "schedule-owner", Capabilities: manifest.Capabilities{
			APIRead: []string{"automations"}, APIWrite: []string{"automations"},
		}},
		InstallationID: "schedule-installation", Status: StatusActive,
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	schedules := &managedScheduleServiceFake{items: make(map[string]pluginsdk.ManagedConversationSchedule)}
	svc.SetManagedConversationSchedules(schedules)
	t.Cleanup(func() { _ = svc.Close() })
	digest := ManifestCapabilityDigest(record.Manifest)
	capabilities := []string{"host.v2.read:automations", "host.v2.write:automations"}
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-schedule", 1, digest, capabilities, "human", "grant", "schedule-grant"); err != nil {
		t.Fatalf("grant schedule capabilities: %v", err)
	}
	host, ok := pluginsdk.HostManagedConversationSchedules(svc.hostForPlugin(record.ID))
	if !ok {
		t.Fatal("plugin Host does not expose managed conversation schedules")
	}
	input := pluginsdk.ManagedConversationScheduleCreate{
		RequestID: "schedule-request", IdempotencyKey: "schedule-create",
		ApprovalRevision: 1, ManifestDigest: digest,
		Schedule: pluginsdk.ManagedConversationSchedule{
			WorkspaceID: "workspace-schedule", Name: "Daily brief", Prompt: "Summarize blockers",
			PluginID: "target-plugin", InstanceKey: "daily-brief", DestinationRevision: 3, Enabled: true,
			Triggers: []pluginsdk.ManagedConversationScheduleTrigger{{
				Type: "scheduled", Config: []byte(`{"cron_expression":"0 9 * * 1-5"}`), Enabled: true,
			}},
		},
	}
	created, schedule, err := host.Create(context.Background(), input)
	if err != nil || created.Status != pluginsdk.CommandApplied || schedule.ID == "" || schedule.ResourceRevision != 1 {
		t.Fatalf("Create schedule = %+v %+v, err=%v", created, schedule, err)
	}
	if schedules.installationID != record.InstallationID || schedules.pluginID != record.ID {
		t.Fatalf("schedule ownership passed to service = %q/%q, want %q/%q", schedules.installationID, schedules.pluginID, record.InstallationID, record.ID)
	}
	replayed, same, err := host.Create(context.Background(), input)
	if err != nil || replayed.Status != pluginsdk.CommandAlreadyApplied || same.ID != schedule.ID {
		t.Fatalf("Create retry = %+v %+v, err=%v", replayed, same, err)
	}
	if schedules.creates != 1 {
		t.Fatalf("domain creates = %d, want one", schedules.creates)
	}
	if _, err := svc.RevokeCapabilityApproval(record.InstallationID, "workspace-schedule", 1, "human", "revoke", "schedule-revoke"); err != nil {
		t.Fatalf("revoke schedule capability: %v", err)
	}
	if _, err := host.List(context.Background(), pluginsdk.ManagedConversationScheduleQuery{
		WorkspaceID: "workspace-schedule", ApprovalRevision: 1, ManifestDigest: digest,
	}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("List after revocation error = %v, want permission denied", err)
	}
}

type managedScheduleServiceFake struct {
	mu             sync.Mutex
	items          map[string]pluginsdk.ManagedConversationSchedule
	installationID string
	pluginID       string
	creates        int
}

func (f *managedScheduleServiceFake) ListManagedConversationSchedules(_ context.Context, installationID, workspaceID string) ([]pluginsdk.ManagedConversationSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []pluginsdk.ManagedConversationSchedule
	for _, schedule := range f.items {
		if schedule.WorkspaceID == workspaceID && installationID == f.installationID {
			result = append(result, schedule)
		}
	}
	return result, nil
}

func (f *managedScheduleServiceFake) CreateManagedConversationSchedule(_ context.Context, installationID, pluginID string, schedule pluginsdk.ManagedConversationSchedule, _, _ string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installationID, f.pluginID = installationID, pluginID
	if existing, ok := f.items[schedule.ID]; ok {
		return existing, true, nil
	}
	schedule.ResourceRevision = 1
	f.items[schedule.ID] = schedule
	f.creates++
	return schedule, false, nil
}

func (f *managedScheduleServiceFake) UpdateManagedConversationSchedule(context.Context, string, string, string, string, uint64, pluginsdk.ManagedConversationSchedule, string, string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	return pluginsdk.ManagedConversationSchedule{}, false, nil
}

func (f *managedScheduleServiceFake) SetManagedConversationScheduleEnabled(context.Context, string, string, string, uint64, bool, string, string) (pluginsdk.ManagedConversationSchedule, bool, error) {
	return pluginsdk.ManagedConversationSchedule{}, false, nil
}

func (f *managedScheduleServiceFake) DeleteManagedConversationSchedule(context.Context, string, string, string, uint64, string, string) (bool, error) {
	return false, nil
}

var _ ManagedConversationScheduleService = (*managedScheduleServiceFake)(nil)
