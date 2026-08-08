package automation

import (
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestWsListWorkspaceRuns_RequiresWorkspaceID(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, err := ws.NewRequest("req-1", ws.ActionAutomationRunsListWorkspace, map[string]any{"limit": 10})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := wsListWorkspaceRuns(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	var ep ws.ErrorPayload
	_ = json.Unmarshal(resp.Payload, &ep)
	if ep.Code != ws.ErrorCodeBadRequest {
		t.Errorf("expected BAD_REQUEST, got %q", ep.Code)
	}
}

// Locks the wire shape the /runs page is built against: a "runs" object
// key (not a bare array), each row carrying automation_name alongside the
// run's own fields.
func TestWsListWorkspaceRuns_ReturnsAttributedRunsForTheWorkspaceOnly(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	mine := &Automation{WorkspaceID: "ws-A", Name: "nightly sweep", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	theirs := &Automation{WorkspaceID: "ws-B", Name: "not mine", WorkflowID: "wf-2", WorkflowStepID: "s-2"}
	for _, a := range []*Automation{mine, theirs} {
		if err := svc.store.CreateAutomation(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []*Automation{mine, theirs} {
		if err := svc.store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusSucceeded,
			TriggerData:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationRunsListWorkspace, map[string]any{"workspace_id": "ws-A"})
	resp, err := wsListWorkspaceRuns(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("expected response, got %v: %s", resp.Type, string(resp.Payload))
	}

	var got struct {
		Runs []*WorkspaceAutomationRun `json:"runs"`
	}
	if err := json.Unmarshal(resp.Payload, &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got.Runs) != 1 {
		t.Fatalf("expected 1 run from ws-A only, got %d: %s", len(got.Runs), string(resp.Payload))
	}
	if got.Runs[0].AutomationID != mine.ID {
		t.Errorf("expected the ws-A run, got automation %q", got.Runs[0].AutomationID)
	}
	if got.Runs[0].AutomationName != "nightly sweep" {
		t.Errorf("expected automation_name, got %q", got.Runs[0].AutomationName)
	}
}

// An empty feed must serialize as [], not null: the page maps over this
// array directly.
func TestWsListWorkspaceRuns_EmptyWorkspaceReturnsAnEmptyArray(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationRunsListWorkspace, map[string]any{"workspace_id": "ws-empty"})
	resp, err := wsListWorkspaceRuns(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resp.Payload, &raw); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if string(raw["runs"]) != "[]" {
		t.Errorf("expected runs: [], got %q", string(raw["runs"]))
	}
}

// The list maps over this array directly, so a workspace whose automations have
// never run must serialize as [], not null.
func TestWsListAutomationSummaries_EmptyWorkspaceReturnsAnEmptyArray(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationSummaries, map[string]any{"workspace_id": "ws-empty"})
	resp, err := wsListAutomationSummaries(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resp.Payload, &raw); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if string(raw["summaries"]) != "[]" {
		t.Errorf("expected summaries: [], got %q", string(raw["summaries"]))
	}
}

// Without a workspace the query has no scope, so this is a bad request rather
// than an accidental cross-workspace read.
func TestWsListAutomationSummaries_RequiresAWorkspace(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationSummaries, map[string]any{})
	resp, err := wsListAutomationSummaries(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("expected error, got %v: %s", resp.Type, string(resp.Payload))
	}
	var ep ws.ErrorPayload
	if err := json.Unmarshal(resp.Payload, &ep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ep.Code != ws.ErrorCodeBadRequest {
		t.Errorf("expected BAD_REQUEST, got %q", ep.Code)
	}
}

// Summaries carry the automation's own open count, not a count derived from
// whatever slice of the feed the client happened to load.
func TestWsListAutomationSummaries_CarriesTheOpenCountAndLastRun(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		Name: "nightly sweep", WorkspaceID: "ws-A", WorkflowID: "wf", WorkflowStepID: "s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordRun(ctx, &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled,
		Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationSummaries, map[string]any{"workspace_id": "ws-A"})
	resp, err := wsListAutomationSummaries(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Summaries []*AutomationSummary `json:"summaries"`
	}
	if err := json.Unmarshal(resp.Payload, &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got.Summaries) != 1 {
		t.Fatalf("expected one summary, got %d: %s", len(got.Summaries), string(resp.Payload))
	}
	if got.Summaries[0].AutomationID != a.ID {
		t.Errorf("expected %s, got %s", a.ID, got.Summaries[0].AutomationID)
	}
	if got.Summaries[0].LastRun == nil {
		t.Fatal("expected the summary to carry the automation's last run")
	}
	if got.Summaries[0].OpenRuns != 0 {
		t.Errorf("a succeeded run is not open: got %d", got.Summaries[0].OpenRuns)
	}
}

// The detail page's variant. "Never run" is a real answer, so the summary is
// nullable rather than an empty envelope the client has to interpret.
func TestWsGetAutomationSummary_NullWhenTheAutomationHasNeverRun(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		Name: "brand new", WorkspaceID: "ws-A", WorkflowID: "wf", WorkflowStepID: "s",
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationSummary, map[string]any{"automation_id": a.ID})
	resp, err := wsGetAutomationSummary(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resp.Payload, &raw); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if string(raw["summary"]) != "null" {
		t.Errorf("expected summary: null, got %q", string(raw["summary"]))
	}
}

func TestWsGetAutomationSummary_RequiresAnAutomation(t *testing.T) {
	svc := newTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	ctx := context.Background()

	req, _ := ws.NewRequest("req-1", ws.ActionAutomationSummary, map[string]any{})
	resp, err := wsGetAutomationSummary(svc, log)(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("expected error, got %v: %s", resp.Type, string(resp.Payload))
	}
	var ep ws.ErrorPayload
	if err := json.Unmarshal(resp.Payload, &ep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ep.Code != ws.ErrorCodeBadRequest {
		t.Errorf("expected BAD_REQUEST, got %q", ep.Code)
	}
}
