package automation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// fakeExportWorkspaceLookup answers AC-44 step 2's workspace-existence
// check: exists[id] true -> nil, otherwise an error wrapping
// repoerrors.ErrWorkspaceNotFound, unless err is set (an infrastructure
// failure unrelated to existence).
type fakeExportWorkspaceLookup struct {
	exists map[string]bool
	err    error
}

func (f *fakeExportWorkspaceLookup) WorkspaceExists(_ context.Context, workspaceID string) error {
	if f.err != nil {
		return f.err
	}
	if f.exists[workspaceID] {
		return nil
	}
	return fmt.Errorf("workspace %q: %w", workspaceID, repoerrors.ErrWorkspaceNotFound)
}

// exportServiceTestFixture wires every Export*Lookup (including the
// workspace-existence one) on a fresh service, so ExportAutomationsDocument
// / ExportAutomationsZip can run end to end through the real store.
func exportServiceTestFixture(t *testing.T) (*Service, *fakeExportWorkspaceLookup) {
	t.Helper()
	svc, _, _, _, _, _ := resolveTestFixture(t)
	wsLookup := &fakeExportWorkspaceLookup{exists: map[string]bool{}}
	svc.SetExportWorkspaceLookup(wsLookup)
	return svc, wsLookup
}

func createExportTestAutomation(t *testing.T, svc *Service, req *CreateAutomationRequest) *Automation {
	t.Helper()
	a, err := svc.CreateAutomation(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}
	return a
}

// AC-32: an authorized, existing workspace with no automations exports a
// valid document with an empty automations list, not an error.

func TestExportAutomationsDocument_NoAutomations_EmptyListAC32(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-empty"] = true

	body, err := svc.ExportAutomationsDocument(context.Background(), "ws-empty")
	if err != nil {
		t.Fatalf("ExportAutomationsDocument: %v", err)
	}
	var doc exportDocument
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if doc.Version != exportDocumentVersion || doc.Type != exportDocumentType {
		t.Errorf("envelope = %+v, want version=%d type=%q", doc, exportDocumentVersion, exportDocumentType)
	}
	if len(doc.Automations) != 0 {
		t.Errorf("Automations = %v, want empty", doc.Automations)
	}
}

// AC-8: automations are ordered by name ascending, tiebroken by id
// ascending — independent of creation order or the store's own ordering.

func TestExportAutomationsDocument_OrdersAutomationsByNameAscending(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Zeta"})
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Alpha"})
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Mid"})

	body, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ExportAutomationsDocument: %v", err)
	}
	var doc exportDocument
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	want := []string{"Alpha", "Mid", "Zeta"}
	if len(doc.Automations) != len(want) {
		t.Fatalf("got %d automations, want %d", len(doc.Automations), len(want))
	}
	for i, name := range want {
		if doc.Automations[i].Name != name {
			t.Errorf("index %d: got %q, want %q", i, doc.Automations[i].Name, name)
		}
	}
}

// AC-2: only the target workspace's automations are exported.

func TestExportAutomationsDocument_OnlyTargetWorkspaceAutomations(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	wsLookup.exists["ws-2"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Mine"})
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-2", Name: "Theirs"})

	body, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ExportAutomationsDocument: %v", err)
	}
	var doc exportDocument
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(doc.Automations) != 1 || doc.Automations[0].Name != "Mine" {
		t.Errorf("Automations = %+v, want only [Mine]", doc.Automations)
	}
}

// AC-4: an empty prompt omits the prompt key entirely, before any fidelity
// rule applies.

func TestExportAutomationsDocument_EmptyPromptOmitsPromptKey(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "No Prompt"})

	body, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ExportAutomationsDocument: %v", err)
	}
	var doc exportDocument
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(doc.Automations) != 1 {
		t.Fatalf("got %d automations, want 1", len(doc.Automations))
	}
	if doc.Automations[0].Prompt != nil {
		t.Errorf("Prompt = %+v, want nil (omitted)", doc.Automations[0].Prompt)
	}
}

// AC-31/AC-44 step 1: an authorizer denial classifies as
// repoerrors.ErrWorkspaceNotFound so the HTTP layer can return 404, never a
// different status or distinguishing text.

func TestExportAutomationsDocument_AuthorizeDenied_ClassifiesAsWorkspaceNotFound(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error {
		return fmt.Errorf("denied: %w", repoerrors.ErrWorkspaceNotFound)
	})

	_, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("err = %v, want wrapping repoerrors.ErrWorkspaceNotFound", err)
	}
}

// AC-45: an authorizer failure for a reason other than denial is a 500, not
// a 404 — the sentinel, not mere non-nil-ness, is what the HTTP layer keys
// on (AC-44 step 1).

func TestExportAutomationsDocument_AuthorizeOtherError_NotClassifiedAsNotFound(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	sentinel := errors.New("boom")
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return sentinel })

	_, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapping sentinel", err)
	}
	if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("err = %v, must not classify as ErrWorkspaceNotFound", err)
	}
}

// AC-44 step 2: the workspace lookup not being wired is a construction
// error (500), never a 404 — it must not be reported through the
// ErrWorkspaceNotFound sentinel.

func TestExportAutomationsDocument_WorkspaceLookupNotWired_ReturnsUnavailableError(t *testing.T) {
	svc, _, _, _, _, _ := resolveTestFixture(t)

	_, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if !errors.Is(err, ErrExportWorkspaceLookupUnavailable) {
		t.Fatalf("err = %v, want wrapping ErrExportWorkspaceLookupUnavailable", err)
	}
	if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("err = %v, must not classify as ErrWorkspaceNotFound", err)
	}
}

// AC-35/AC-44 step 2: a workspace that does not exist classifies as
// repoerrors.ErrWorkspaceNotFound, indistinguishable from the AC-31 denial.

func TestExportAutomationsDocument_WorkspaceDoesNotExist_ClassifiesAsWorkspaceNotFound(t *testing.T) {
	svc, _ := exportServiceTestFixture(t)

	_, err := svc.ExportAutomationsDocument(context.Background(), "ws-nonexistent")
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("err = %v, want wrapping repoerrors.ErrWorkspaceNotFound", err)
	}
}

// AC-19/AC-45: a descriptor lookup failure (as opposed to reporting
// "absent") aborts the whole export rather than degrading to a warning.

func TestExportAutomationsDocument_DescriptorLookupFailure_AbortsExport(t *testing.T) {
	svc, agentLookup, _, _, _, _ := resolveTestFixture(t)
	wsLookup := &fakeExportWorkspaceLookup{exists: map[string]bool{"ws-1": true}}
	svc.SetExportWorkspaceLookup(wsLookup)
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "A", AgentProfileID: "agent-1"})
	sentinel := errors.New("boom")
	agentLookup.err = sentinel

	_, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapping sentinel", err)
	}
}

// AC-24: the zip form produces one entry per automation, at the AC-25/26/27
// slug path, built from the same orchestration as the document form.

func TestExportAutomationsZip_OneEntryPerAutomation(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Daily Sync"})

	data, err := svc.ExportAutomationsZip(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ExportAutomationsZip: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty zip bytes")
	}
	names := zipEntryNames(t, data)
	want := []string{".kandev/automations/daily-sync.yml"}
	if len(names) != len(want) || names[0] != want[0] {
		t.Errorf("entries = %v, want %v", names, want)
	}
}

// AC-39/AC-42: a trigger with malformed config still exports the trigger
// (empty mapping) and surfaces exactly one rendered warning line.

func TestExportAutomationsDocument_TriggerBadConfig_EmitsRenderedWarning(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{
		WorkspaceID: "ws-1",
		Name:        "Hook",
		Triggers: []CreateTriggerSpec{
			{Type: TriggerTypeWebhook, Config: []byte("not-json"), Enabled: true},
		},
	})

	body, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ExportAutomationsDocument: %v", err)
	}
	var doc exportDocument
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	want := "Hook: trigger webhook: config is not valid JSON"
	if len(doc.Warnings) != 1 || doc.Warnings[0] != want {
		t.Errorf("Warnings = %v, want [%q]", doc.Warnings, want)
	}
	if len(doc.Automations) != 1 || len(doc.Automations[0].Triggers) != 1 {
		t.Fatalf("expected the trigger still exported despite bad config, got %+v", doc.Automations)
	}
}

// AC-10: two automations whose stored trigger config JSON differs only in
// whitespace must export identical config YAML, end to end through the real
// service and store (not just the buildTriggerConfigNode helper in
// isolation).

func TestExportAutomationsDocument_AC10_WhitespaceOnlyConfigDifferenceProducesIdenticalOutput(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{
		WorkspaceID: "ws-1",
		Name:        "Compact",
		Triggers: []CreateTriggerSpec{
			{Type: TriggerTypeScheduled, Config: []byte(`{"cron_expression":"0 9 * * *","timezone":"Asia/Singapore"}`), Enabled: true},
		},
	})
	createExportTestAutomation(t, svc, &CreateAutomationRequest{
		WorkspaceID: "ws-1",
		Name:        "Spaced",
		Triggers: []CreateTriggerSpec{
			{Type: TriggerTypeScheduled, Config: []byte(`{"cron_expression": "0 9 * * *", "timezone": "Asia/Singapore"}`), Enabled: true},
		},
	})

	body, err := svc.ExportAutomationsDocument(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ExportAutomationsDocument: %v", err)
	}
	var doc exportDocument
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(doc.Automations) != 2 {
		t.Fatalf("got %d automations, want 2", len(doc.Automations))
	}
	// Name-ascending order (AC-8): "Compact" before "Spaced".
	compactConfig, err := yaml.Marshal(doc.Automations[0].Triggers[0].Config)
	if err != nil {
		t.Fatalf("yaml.Marshal(compact config): %v", err)
	}
	spacedConfig, err := yaml.Marshal(doc.Automations[1].Triggers[0].Config)
	if err != nil {
		t.Fatalf("yaml.Marshal(spaced config): %v", err)
	}
	if string(compactConfig) != string(spacedConfig) {
		t.Errorf("exported config differs by stored whitespace alone:\ncompact: %q\nspaced:  %q", compactConfig, spacedConfig)
	}
}

// AC-8: triggers within an automation are ordered by type ascending,
// tiebroken by id ascending — tested directly since trigger ids are
// service-generated and not caller-controllable through CreateAutomation.

func TestBuildExportTriggers_OrdersByTypeThenID(t *testing.T) {
	svc, _, _, _, _, _ := resolveTestFixture(t)
	a := &Automation{
		ID:   "auto-1",
		Name: "A",
		Triggers: []AutomationTrigger{
			// Enabled distinguishes t-a from t-b in the output, since
			// exportTrigger carries no id.
			{ID: "t-b", AutomationID: "auto-1", Type: TriggerTypeWebhook, Config: []byte("{}"), Enabled: false},
			{ID: "t-a", AutomationID: "auto-1", Type: TriggerTypeWebhook, Config: []byte("{}"), Enabled: true},
			{ID: "t-c", AutomationID: "auto-1", Type: TriggerTypeScheduled, Config: []byte("{}"), Enabled: true},
		},
	}

	triggers, _, err := svc.buildExportTriggers(a)
	if err != nil {
		t.Fatalf("buildExportTriggers: %v", err)
	}
	wantTypes := []string{string(TriggerTypeScheduled), string(TriggerTypeWebhook), string(TriggerTypeWebhook)}
	if len(triggers) != len(wantTypes) {
		t.Fatalf("got %d triggers, want %d", len(triggers), len(wantTypes))
	}
	for i, wt := range wantTypes {
		if triggers[i].Type != wt {
			t.Errorf("index %d: got type %q, want %q", i, triggers[i].Type, wt)
		}
	}
	// Within the two webhook triggers (same type), id "t-a" (Enabled=true)
	// sorts before "t-b" (Enabled=false).
	if !triggers[1].Enabled || triggers[2].Enabled {
		t.Errorf("expected t-a (enabled) before t-b (disabled), got %+v", triggers[1:])
	}
}
