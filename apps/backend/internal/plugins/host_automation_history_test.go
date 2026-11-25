package plugins

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAutomationHistorySource struct {
	definitions        []AutomationHistoryDefinition
	definitionOverflow bool
	runs               []AutomationHistoryRun
	runOverflow        bool
	run                AutomationHistoryRun
	err                error

	definitionCalls int
	runCalls        int
	getCalls        int
}

func (f *fakeAutomationHistorySource) ListAutomationHistoryDefinitions(
	context.Context, string, string, *time.Time, *time.Time, int,
) (AutomationHistoryDefinitionList, error) {
	f.definitionCalls++
	if f.err != nil {
		return AutomationHistoryDefinitionList{}, f.err
	}
	return AutomationHistoryDefinitionList{Items: f.definitions, Overflow: f.definitionOverflow}, nil
}

func (f *fakeAutomationHistorySource) ListAutomationHistoryRuns(
	context.Context, string, string, *time.Time, *time.Time, int,
) (AutomationHistoryRunList, error) {
	f.runCalls++
	if f.err != nil {
		return AutomationHistoryRunList{}, f.err
	}
	return AutomationHistoryRunList{Items: f.runs, Overflow: f.runOverflow}, nil
}

func (f *fakeAutomationHistorySource) GetAutomationHistoryRun(
	context.Context, string, string,
) (AutomationHistoryRun, error) {
	f.getCalls++
	if f.err != nil {
		return AutomationHistoryRun{}, f.err
	}
	return f.run, nil
}

func newAutomationHistoryHost(t *testing.T, source AutomationHistoryDataSource, grants []string) *pluginHost {
	t.Helper()
	data := &fakeTaskDataSource{workspaces: []*taskmodels.Workspace{{ID: "workspace-one"}}}
	host := newExactReadHost(t, data, []string{"automations"}, grants)
	host.automationHistoryDataDep = func() AutomationHistoryDataSource { return source }
	return host
}

func historyDefinitionFixture(id, name string) AutomationHistoryDefinition {
	timezone := "Europe/Berlin"
	return AutomationHistoryDefinition{
		ID: id, WorkspaceID: "workspace-one", Name: name, Enabled: true,
		CreatedAt:       time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		ResourceVersion: "definition-" + id,
		Triggers: []AutomationHistoryTrigger{{
			ID: "trigger-" + id, Kind: "scheduled", Enabled: true,
			CronExpression: "0 9 * * *", Timezone: timezone, ScheduleAvailable: true,
		}},
	}
}

func historyRunFixture(id, status, sourceStatus string) AutomationHistoryRun {
	attempt := int64(2)
	return AutomationHistoryRun{
		ID: id, WorkspaceID: "workspace-one", AutomationID: "automation-one", AutomationName: "nightly",
		TriggerID: "trigger-one", TriggerKind: "scheduled", TaskID: "task-one", SessionID: "session-one",
		TurnID: "turn-one", ManagedConversationID: "conversation-one",
		SourceStatus: sourceStatus, Status: status, RetryState: "scheduled", DeliveryStatus: "accepted",
		RetryGroupID: "group-one", RetryParentRunID: "run-parent",
		AttemptNumber: &attempt, CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Summary: "completed later", ErrorExcerpt: "safe error", ResourceVersion: "run-" + id,
	}
}

func TestAutomationHistoryHostAuthorization(t *testing.T) {
	ctx := context.Background()
	source := &fakeAutomationHistorySource{
		definitions: []AutomationHistoryDefinition{historyDefinitionFixture("automation-one", "nightly")},
		runs:        []AutomationHistoryRun{historyRunFixture("run-one", "succeeded", "succeeded")},
		run:         historyRunFixture("run-one", "succeeded", "succeeded"),
	}
	denied := newAutomationHistoryHost(t, source, nil)
	_, err := denied.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "denied", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.PermissionDenied || source.runCalls != 0 {
		t.Fatalf("missing grant = %v with %d source calls; want PermissionDenied before reading", err, source.runCalls)
	}

	host := newAutomationHistoryHost(t, source, []string{"host.v2.read:automations"})
	page, err := host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "allowed", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "run-one" {
		t.Fatalf("authorized runs = %+v, err=%v", page, err)
	}
	if _, err := host.service.approvalRevoke(host.installationID, "workspace-one", "human", "revoke", "revoke-one"); err != nil {
		t.Fatalf("revoke approval: %v", err)
	}
	callsBefore := source.runCalls
	if _, err := host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "revoked", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	}); status.Code(err) != codes.PermissionDenied || source.runCalls != callsBefore {
		t.Fatalf("revoked read = %v with %d source calls; want denial before reading", err, source.runCalls-callsBefore)
	}

	foreignInstallation := newAutomationHistoryHost(t, source, []string{"host.v2.read:automations"})
	foreignInstallation.installationID = "foreign-installation"
	if _, err := foreignInstallation.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "foreign-installation", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("foreign installation = %v, want PermissionDenied", err)
	}

	host = newAutomationHistoryHost(t, &fakeAutomationHistorySource{err: ErrAutomationHistoryNotFound}, []string{"host.v2.read:automations"})
	_, err = host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "foreign-filter", WorkspaceID: "workspace-one",
		Filter: pluginsdk.AutomationRunFilter{AutomationID: "automation-foreign"}, Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("foreign automation filter = %v, want NotFound", err)
	}
	_, _, err = host.AutomationHistory().GetRun(ctx, pluginsdk.AutomationRunGetQuery{
		RequestID: "foreign-run", WorkspaceID: "workspace-one", RunID: "run-foreign",
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("foreign run = %v, want NotFound", err)
	}

	source = &fakeAutomationHistorySource{runs: []AutomationHistoryRun{historyRunFixture("run-one", "succeeded", "succeeded")}}
	host = newAutomationHistoryHost(t, source, []string{"host.v2.read:automations"})
	badFrom := "not-a-timestamp"
	_, err = host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "bad-range", WorkspaceID: "workspace-one",
		Filter: pluginsdk.AutomationRunFilter{CreatedFrom: &badFrom}, Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.InvalidArgument || source.runCalls != 0 {
		t.Fatalf("malformed filter = %v with %d source calls", err, source.runCalls)
	}
	from := "2026-10-02T00:00:00Z"
	before := "2026-10-01T00:00:00Z"
	_, err = host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "reversed-range", WorkspaceID: "workspace-one",
		Filter: pluginsdk.AutomationRunFilter{CreatedFrom: &from, CreatedBefore: &before}, Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.InvalidArgument || source.runCalls != 0 {
		t.Fatalf("reversed filter = %v with %d source calls", err, source.runCalls)
	}

	source = &fakeAutomationHistorySource{err: errors.New("database is down")}
	host = newAutomationHistoryHost(t, source, []string{"host.v2.read:automations"})
	_, err = host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "source-error", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("source error = %v, want Unavailable", err)
	}

	missing := newAutomationHistoryHost(t, source, []string{"host.v2.read:automations"})
	missing.automationHistoryDataDep = nil
	capability, err := missing.GetCapabilityContext(ctx, "workspace-one")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, operation := range capability.Operations {
		if operation.Method != automationHistoryListRunsMethod {
			continue
		}
		found = true
		if operation.Supported || operation.UnavailableReason != "automation_history_data_unavailable" {
			t.Fatalf("missing dependency discovery = %+v, want unsupported", operation)
		}
	}
	if !found {
		t.Fatal("automation history method missing from capability discovery")
	}
	if _, err := missing.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "missing-source", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	}); status.Code(err) != codes.Unavailable {
		t.Fatalf("missing dependency read = %v, want Unavailable", err)
	}
}

func TestAutomationHistoryHostSnapshots(t *testing.T) {
	ctx := context.Background()
	from := "2026-10-01T00:00:00Z"
	source := &fakeAutomationHistorySource{
		definitions: []AutomationHistoryDefinition{
			historyDefinitionFixture("automation-one", "nightly"),
			historyDefinitionFixture("automation-two", "weekly"),
		},
	}
	host := newAutomationHistoryHost(t, source, []string{"host.v2.read:automations"})
	reader := host.AutomationHistory()

	first, err := reader.ListAutomations(ctx, pluginsdk.AutomationDefinitionQuery{
		RequestID: "definition-first", WorkspaceID: "workspace-one",
		Filter: pluginsdk.AutomationDefinitionFilter{CreatedFrom: &from},
		Page:   pluginsdk.ExactReadPage{Limit: 1},
	})
	if err != nil || len(first.Items) != 1 || !first.PageInfo.HasMore {
		t.Fatalf("first definition page = %+v, err=%v", first, err)
	}
	source.definitions[1].Name = "changed after snapshot"
	second, err := reader.ListAutomations(ctx, pluginsdk.AutomationDefinitionQuery{
		RequestID: "definition-second", WorkspaceID: "workspace-one",
		Filter: pluginsdk.AutomationDefinitionFilter{CreatedFrom: &from},
		Page: pluginsdk.ExactReadPage{
			Limit: 1, Cursor: first.PageInfo.NextCursor, SnapshotVersion: first.PageInfo.SnapshotVersion,
		},
	})
	if err != nil || len(second.Items) != 1 || second.Items[0].Name != "weekly" {
		t.Fatalf("continuation page = %+v, err=%v; want the frozen snapshot row", second, err)
	}
	fresh, err := reader.ListAutomations(ctx, pluginsdk.AutomationDefinitionQuery{
		RequestID: "definition-fresh", WorkspaceID: "workspace-one",
		Filter: pluginsdk.AutomationDefinitionFilter{CreatedFrom: &from},
		Page:   pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || fresh.Items[1].Name != "changed after snapshot" {
		t.Fatalf("fresh definition query = %+v, err=%v", fresh, err)
	}

	// A changed filter cannot continue the previous snapshot.
	_, err = reader.ListAutomations(ctx, pluginsdk.AutomationDefinitionQuery{
		RequestID: "definition-changed-filter", WorkspaceID: "workspace-one",
		Page: pluginsdk.ExactReadPage{
			Limit: 1, Cursor: first.PageInfo.NextCursor, SnapshotVersion: first.PageInfo.SnapshotVersion,
		},
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("changed filter continuation = %v, want FailedPrecondition", err)
	}

	// Revocation denies a cached continuation; regranting at a new revision
	// invalidates the snapshot identity instead of restoring access.
	if _, err := host.service.approvalRevoke(host.installationID, "workspace-one", "human", "revoke", "revoke-snapshot"); err != nil {
		t.Fatal(err)
	}
	_, err = reader.ListAutomations(ctx, pluginsdk.AutomationDefinitionQuery{
		RequestID: "definition-revoked", WorkspaceID: "workspace-one",
		Filter: pluginsdk.AutomationDefinitionFilter{CreatedFrom: &from},
		Page: pluginsdk.ExactReadPage{
			Limit: 1, Cursor: first.PageInfo.NextCursor, SnapshotVersion: first.PageInfo.SnapshotVersion,
		},
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("revoked continuation = %v, want PermissionDenied", err)
	}
	if _, _, err := reader.GetRun(ctx, pluginsdk.AutomationRunGetQuery{
		RequestID: "definition-revoked-refresh", WorkspaceID: "workspace-one", RunID: "run-one",
	}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("revoked exact refresh = %v, want PermissionDenied", err)
	}
	if _, err := host.service.approvalGrant(host.installationID, "workspace-one", 3,
		ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest),
		[]string{"host.v2.read:automations"}, "human", "regrant", "regrant-snapshot"); err != nil {
		t.Fatal(err)
	}
	_, err = reader.ListAutomations(ctx, pluginsdk.AutomationDefinitionQuery{
		RequestID: "definition-replaced", WorkspaceID: "workspace-one",
		Filter: pluginsdk.AutomationDefinitionFilter{CreatedFrom: &from},
		Page: pluginsdk.ExactReadPage{
			Limit: 1, Cursor: first.PageInfo.NextCursor, SnapshotVersion: first.PageInfo.SnapshotVersion,
		},
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("replaced approval continuation = %v, want FailedPrecondition", err)
	}

	// Exact refresh is independent of list snapshots and observes later state.
	source.run = historyRunFixture("run-one", "task_created", "task_created")
	initial, _, err := reader.GetRun(ctx, pluginsdk.AutomationRunGetQuery{
		RequestID: "run-before", WorkspaceID: "workspace-one", RunID: "run-one",
	})
	if err != nil || initial.Status != "task_created" {
		t.Fatalf("initial refresh = %+v, err=%v", initial, err)
	}
	source.run = historyRunFixture("run-one", "archived", "task_created")
	refreshed, receipt, err := reader.GetRun(ctx, pluginsdk.AutomationRunGetQuery{
		RequestID: "run-after", WorkspaceID: "workspace-one", RunID: "run-one",
	})
	if err != nil || refreshed.Status != "archived" || refreshed.SourceStatus != "task_created" {
		t.Fatalf("refreshed run = %+v, err=%v", refreshed, err)
	}
	if receipt.RequestID != "run-after" || receipt.InstallationID != host.installationID {
		t.Fatalf("read receipt = %+v", receipt)
	}
}

func TestAutomationHistoryHostLimits(t *testing.T) {
	ctx := context.Background()
	source := &fakeAutomationHistorySource{runOverflow: true}
	host := newAutomationHistoryHost(t, source, []string{"host.v2.read:automations"})
	_, err := host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "overflow", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("overflowing source = %v, want ResourceExhausted", err)
	}

	rows := make([]AutomationHistoryRun, 5001)
	for i := range rows {
		rows[i] = historyRunFixture(fmt.Sprintf("run-%04d", i), "succeeded", "succeeded")
		rows[i].CreatedAt = time.Date(2026, 10, 1, 0, 0, 0, i, time.UTC)
	}
	source = &fakeAutomationHistorySource{runs: rows}
	host = newAutomationHistoryHost(t, source, []string{"host.v2.read:automations"})
	_, err = host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "row-limit", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 200},
	})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("row limit = %v, want ResourceExhausted", err)
	}

	large := historyRunFixture("run-large", "succeeded", "succeeded")
	large.Summary = strings.Repeat("a", exactSnapshotMaxBytes+1024)
	source = &fakeAutomationHistorySource{runs: []AutomationHistoryRun{large}}
	host = newAutomationHistoryHost(t, source, []string{"host.v2.read:automations"})
	_, err = host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "byte-limit", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("byte limit = %v, want ResourceExhausted", err)
	}

	source = &fakeAutomationHistorySource{}
	host = newAutomationHistoryHost(t, source, []string{"host.v2.read:automations"})
	empty, err := host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "empty", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(empty.Items) != 0 || empty.PageInfo.HasMore {
		t.Fatalf("empty query = %+v, err=%v; want complete empty page", empty, err)
	}
}

func TestAutomationHistoryHostWire(t *testing.T) {
	ctx := context.Background()
	source := &fakeAutomationHistorySource{
		definitions: []AutomationHistoryDefinition{historyDefinitionFixture("automation-one", "nightly")},
		runs:        []AutomationHistoryRun{historyRunFixture("run-one", "archived", "task_created")},
		run:         historyRunFixture("run-one", "archived", "task_created"),
	}
	host := newAutomationHistoryHost(t, source, []string{"host.v2.read:automations"})
	wireHost := dialPluginHostOverWire(t, host)
	reader, ok := pluginsdk.HostAutomationHistory(wireHost)
	if !ok {
		t.Fatal("plugin Host transport does not expose automation history")
	}
	definitions, err := reader.ListAutomations(ctx, pluginsdk.AutomationDefinitionQuery{
		RequestID: "wire-definitions", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(definitions.Items) != 1 {
		t.Fatalf("wire definitions = %+v, err=%v", definitions, err)
	}
	if definitions.Items[0].Triggers[0].CronExpression != "0 9 * * *" || definitions.Items[0].ResourceVersion == "" {
		t.Fatalf("wire definition projection = %+v", definitions.Items[0])
	}
	runs, err := reader.ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "wire-runs", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(runs.Items) != 1 || runs.Items[0].SourceStatus != "task_created" || runs.Items[0].Status != "archived" {
		t.Fatalf("wire runs = %+v, err=%v", runs, err)
	}
	if runs.PageInfo.Receipt.InstallationID != host.installationID {
		t.Fatalf("wire page receipt = %+v", runs.PageInfo.Receipt)
	}
	run, receipt, err := reader.GetRun(ctx, pluginsdk.AutomationRunGetQuery{
		RequestID: "wire-get", WorkspaceID: "workspace-one", RunID: "run-one",
	})
	if err != nil || run.ID != "run-one" || receipt.RequestID != "wire-get" {
		t.Fatalf("wire exact run = %+v, receipt=%+v, err=%v", run, receipt, err)
	}
}

func TestAutomationHistoryHostAvailability(t *testing.T) {
	ctx := context.Background()
	host := newAutomationHistoryHost(t, &fakeAutomationHistorySource{}, []string{"host.v2.read:automations"})
	wireHost := dialPluginHostOverWire(t, host)
	exact, ok := pluginsdk.HostV2(wireHost)
	if !ok {
		t.Fatal("wire Host does not expose exact host capabilities")
	}
	capability, err := exact.GetCapabilityContext(ctx, "workspace-one")
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{automationHistoryListDefinitionsMethod, automationHistoryListRunsMethod, automationHistoryGetRunMethod} {
		found := false
		for _, operation := range capability.Operations {
			if operation.Method != method {
				continue
			}
			found = true
			if !operation.Supported || !operation.Authorized {
				t.Fatalf("operation %s = %+v, want supported and authorized", method, operation)
			}
		}
		if !found {
			t.Fatalf("operation %s missing from wire capability discovery", method)
		}
	}
}

func TestAutomationHistoryLateWiring(t *testing.T) {
	ctx := context.Background()
	data := &fakeTaskDataSource{workspaces: []*taskmodels.Workspace{{ID: "workspace-one"}}}
	host := newExactReadHost(t, data, []string{"automations"}, []string{"host.v2.read:automations"})
	if _, err := host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "before-wiring", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	}); status.Code(err) != codes.Unavailable {
		t.Fatalf("read before wiring = %v, want Unavailable", err)
	}
	source := &fakeAutomationHistorySource{
		runs: []AutomationHistoryRun{historyRunFixture("run-late", "succeeded", "succeeded")},
	}
	host.service.SetAutomationHistoryDataSource(source)
	page, err := host.AutomationHistory().ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "after-wiring", WorkspaceID: "workspace-one", Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "run-late" {
		t.Fatalf("read after wiring = %+v, err=%v; want the late-wired source", page, err)
	}
}
