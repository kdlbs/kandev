package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

func TestListPorts_UnwrapsPortsEnvelopeAndDecodesEveryField(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"ports":[
			{"port":3000,"address":"0.0.0.0","process":"node"},
			{"port":5432,"address":"127.0.0.1"}
		]
	}`))

	ports, err := newHTTPOnlyClient(srv.URL).ListPorts(context.Background())
	if err != nil {
		t.Fatalf("ListPorts: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/ports" {
		t.Errorf("request = %s %s, want GET /api/v1/ports", got.Method, got.Path)
	}
	if len(ports) != 2 {
		t.Fatalf("ports = %d, want 2", len(ports))
	}
	if ports[0].Port != 3000 || ports[0].Address != "0.0.0.0" || ports[0].Process != "node" {
		t.Errorf("ports[0] = %+v, want 3000 on 0.0.0.0 owned by node", ports[0])
	}
	if ports[1].Port != 5432 || ports[1].Address != "127.0.0.1" || ports[1].Process != "" {
		t.Errorf("ports[1] = %+v, want 5432 on loopback with no owner", ports[1])
	}
}

func TestListPorts_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-2xx status", http.StatusInternalServerError, `ss unavailable`, "list ports failed with status 500"},
		{"malformed body", http.StatusOK, `{"ports":`, "failed to parse list ports response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			ports, err := newHTTPOnlyClient(srv.URL).ListPorts(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if ports != nil {
				t.Errorf("ports = %+v, want nil on failure", ports)
			}
		})
	}
}

func TestListPorts_EchoesServerBodyInStatusError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusInternalServerError, `ss unavailable`))

	_, err := newHTTPOnlyClient(srv.URL).ListPorts(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ss unavailable") {
		t.Fatalf("error = %v, want the server body echoed", err)
	}
}

func TestSystemMetrics_EncodesMetricIDsAsCommaSeparatedQuery(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"id":"agentctl","label":"Container","kind":"container",
		"executor_type":"local_docker","session_id":"sess-1","task_id":"task-1",
		"metrics":[
			{"id":"cpu","label":"CPU","unit":"percent","value":42.5,"available":true},
			{"id":"disk","label":"Disk","available":false,"error":"statfs failed"}
		]
	}`))

	snapshot, err := newHTTPOnlyClient(srv.URL).SystemMetrics(
		context.Background(), []string{"cpu", "memory", "disk"}, "/workspace")
	if err != nil {
		t.Fatalf("SystemMetrics: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/system/metrics" {
		t.Errorf("request = %s %s, want GET /api/v1/system/metrics", got.Method, got.Path)
	}
	if m := got.Query.Get("metrics"); m != "cpu,memory,disk" {
		t.Errorf("metrics = %q, want one comma-joined param, not repeated keys", m)
	}
	if len(got.Query["metrics"]) != 1 {
		t.Errorf("metrics appears %d times, want 1", len(got.Query["metrics"]))
	}
	if d := got.Query.Get("disk_path"); d != "/workspace" {
		t.Errorf("disk_path = %q, want /workspace", d)
	}

	if snapshot.ID != "agentctl" || snapshot.Label != "Container" || snapshot.Kind != "container" {
		t.Errorf("id/label/kind = %q / %q / %q", snapshot.ID, snapshot.Label, snapshot.Kind)
	}
	if snapshot.ExecutorType != "local_docker" || snapshot.SessionID != "sess-1" ||
		snapshot.TaskID != "task-1" {
		t.Errorf("executor/session/task = %q / %q / %q",
			snapshot.ExecutorType, snapshot.SessionID, snapshot.TaskID)
	}
	if len(snapshot.Metrics) != 2 {
		t.Fatalf("Metrics = %d, want 2", len(snapshot.Metrics))
	}
	cpu := snapshot.Metrics[0]
	if cpu.ID != "cpu" || cpu.Label != "CPU" || cpu.Unit != "percent" || !cpu.Available {
		t.Errorf("Metrics[0] = %+v, want an available cpu percent sample", cpu)
	}
	if cpu.Value == nil || *cpu.Value != 42.5 {
		t.Errorf("Metrics[0].Value = %v, want pointer to 42.5", cpu.Value)
	}
	disk := snapshot.Metrics[1]
	if disk.Available || disk.Error != "statfs failed" {
		t.Errorf("Metrics[1] = %+v, want an unavailable sample carrying its error", disk)
	}
	if disk.Value != nil {
		t.Errorf("Metrics[1].Value = %v, want nil for an unavailable sample", disk.Value)
	}
}

func TestSystemMetrics_OmitsEmptyQueryParams(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"id":"a","metrics":[]}`))

	if _, err := newHTTPOnlyClient(srv.URL).SystemMetrics(context.Background(), nil, ""); err != nil {
		t.Fatalf("SystemMetrics: %v", err)
	}
	if got.RawQuery != "" {
		t.Errorf("raw query = %q, want no '?' appended when nothing is selected", got.RawQuery)
	}
}

func TestSystemMetrics_ReturnsSnapshotAndErrorOnHTTPError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusServiceUnavailable,
		`{"id":"agentctl","metrics":[]}`))

	snapshot, err := newHTTPOnlyClient(srv.URL).SystemMetrics(context.Background(), nil, "")
	if err == nil || !strings.Contains(err.Error(), "system metrics failed with status 503") {
		t.Fatalf("error = %v, want status 503 named", err)
	}
	if snapshot == nil || snapshot.ID != "agentctl" {
		t.Errorf("snapshot = %+v, want the decoded body alongside the error", snapshot)
	}
}

func TestSystemMetrics_ReturnsNilSnapshotOnMalformedBody(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"id":`))

	snapshot, err := newHTTPOnlyClient(srv.URL).SystemMetrics(context.Background(), nil, "")
	if err == nil || !strings.Contains(err.Error(), "failed to parse response") {
		t.Fatalf("error = %v, want parse failure", err)
	}
	if snapshot != nil {
		t.Errorf("snapshot = %+v, want nil on decode failure", snapshot)
	}
}

func TestSetBaseBranches_PostsPerRepoMapIncludingTheRootKey(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	branches := map[string]string{"": "origin/main", "web": "origin/develop"}
	if err := newHTTPOnlyClient(srv.URL).SetBaseBranches(context.Background(), branches); err != nil {
		t.Fatalf("SetBaseBranches: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/workspace/base-branches" {
		t.Errorf("request = %s %s, want POST /api/v1/workspace/base-branches", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent struct {
		BaseBranches map[string]string `json:"base_branches"`
	}
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	// The empty key addresses the workspace root tracker — dropping it silently
	// reverts single-repo tasks to the hardcoded origin/main fallback.
	if sent.BaseBranches[""] != "origin/main" {
		t.Errorf("base_branches[\"\"] = %q, want origin/main", sent.BaseBranches[""])
	}
	if sent.BaseBranches["web"] != "origin/develop" {
		t.Errorf("base_branches[web] = %q, want origin/develop", sent.BaseBranches["web"])
	}
}

func TestSetBaseBranches_SendsNilMapToClearTheOverride(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	if err := newHTTPOnlyClient(srv.URL).SetBaseBranches(context.Background(), nil); err != nil {
		t.Fatalf("SetBaseBranches: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(got.Body, &raw); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	// The key must still be present: an absent field would leave the server's
	// existing override in place rather than clearing it.
	body, present := raw["base_branches"]
	if !present {
		t.Fatalf("base_branches absent from clear request: %s", got.Body)
	}
	if string(body) != "null" {
		t.Errorf("base_branches = %s, want null", body)
	}
}

func TestSetBaseBranches_FailsOnNonOKStatusAndEchoesBody(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusBadRequest, `unknown repo "web"`))

	err := newHTTPOnlyClient(srv.URL).SetBaseBranches(
		context.Background(), map[string]string{"web": "x"})
	if err == nil || !strings.Contains(err.Error(), "set base branches failed with status 400") {
		t.Fatalf("error = %v, want status 400 named", err)
	}
	if !strings.Contains(err.Error(), `unknown repo "web"`) {
		t.Errorf("error = %v, want the server body echoed", err)
	}
}

func TestSetComparisonTargets_PostsReplacementMap(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	if err := newHTTPOnlyClient(srv.URL).SetComparisonTargets(context.Background(), nil); err != nil {
		t.Fatalf("SetComparisonTargets: %v", err)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v1/workspace/comparison-targets" {
		t.Errorf("request = %s %s, want POST /api/v1/workspace/comparison-targets", got.Method, got.Path)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(got.Body, &raw); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if body, present := raw["comparison_targets"]; !present || string(body) != "null" {
		t.Errorf("comparison_targets = %s, present=%v, want null", body, present)
	}
}

func TestInferencePrompt_PostsRequestAndDecodesUsage(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"success":true,"response":"a summary","model":"claude-haiku-4-5",
		"prompt_tokens":120,"response_tokens":48,"duration_ms":1500
	}`))

	resp, err := newHTTPOnlyClient(srv.URL).InferencePrompt(context.Background(),
		&utility.PromptRequest{
			Prompt:  "summarize this",
			AgentID: "claude-code",
			Model:   "claude-haiku-4-5",
			Mode:    "default",
		})
	if err != nil {
		t.Fatalf("InferencePrompt: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/inference/prompt" {
		t.Errorf("request = %s %s, want POST /api/v1/inference/prompt", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent utility.PromptRequest
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.Prompt != "summarize this" || sent.AgentID != "claude-code" {
		t.Errorf("sent prompt/agent = %q / %q", sent.Prompt, sent.AgentID)
	}
	if sent.Model != "claude-haiku-4-5" || sent.Mode != "default" {
		t.Errorf("sent model/mode = %q / %q", sent.Model, sent.Mode)
	}

	if !resp.Success || resp.Response != "a summary" {
		t.Errorf("success/response = %v / %q", resp.Success, resp.Response)
	}
	if resp.Model != "claude-haiku-4-5" {
		t.Errorf("Model = %q", resp.Model)
	}
	if resp.PromptTokens != 120 || resp.ResponseTokens != 48 {
		t.Errorf("tokens = %d / %d, want 120 / 48", resp.PromptTokens, resp.ResponseTokens)
	}
	if resp.DurationMs != 1500 {
		t.Errorf("DurationMs = %d, want 1500", resp.DurationMs)
	}
}

func TestProbe_PostsRequestAndDecodesAgentIdentity(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"success":true,"duration_ms":900,
		"agent_name":"claude-code","agent_version":"1.2.3","protocol_version":1
	}`))

	resp, err := newHTTPOnlyClient(srv.URL).Probe(context.Background(),
		&utility.ProbeRequest{
			AgentID:       "claude-acp",
			Model:         "claude-opus-5",
			Mode:          "plan",
			ConfigOptions: map[string]string{"thinking": "high"},
			Refresh:       true,
		})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/inference/probe" {
		t.Errorf("request = %s %s, want POST /api/v1/inference/probe", got.Method, got.Path)
	}

	var sent utility.ProbeRequest
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.AgentID != "claude-acp" || sent.Model != "claude-opus-5" || sent.Mode != "plan" {
		t.Errorf("sent agent/model/mode = %q / %q / %q", sent.AgentID, sent.Model, sent.Mode)
	}
	if sent.ConfigOptions["thinking"] != "high" || !sent.Refresh {
		t.Errorf("sent config/refresh = %v / %v", sent.ConfigOptions, sent.Refresh)
	}

	if !resp.Success || resp.DurationMs != 900 {
		t.Errorf("success/duration = %v / %d", resp.Success, resp.DurationMs)
	}
	if resp.AgentName != "claude-code" || resp.AgentVersion != "1.2.3" {
		t.Errorf("agent name/version = %q / %q", resp.AgentName, resp.AgentVersion)
	}
	if resp.ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion = %d, want 1", resp.ProtocolVersion)
	}
}

// Both utility endpoints share doLongRunningJSON, so the failure modes are
// asserted once through each entry point's label.
func TestLongRunningJSONEndpoints_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		call    func(c *Client) error
		wantErr string
	}{
		{
			name:   "prompt non-2xx status",
			status: http.StatusBadGateway,
			body:   `agent unreachable`,
			call: func(c *Client) error {
				_, err := c.InferencePrompt(context.Background(), &utility.PromptRequest{})
				return err
			},
			wantErr: "utility prompt failed with status 502: agent unreachable",
		},
		{
			name:   "prompt malformed body",
			status: http.StatusOK,
			body:   `{"success":`,
			call: func(c *Client) error {
				_, err := c.InferencePrompt(context.Background(), &utility.PromptRequest{})
				return err
			},
			wantErr: "failed to parse utility prompt response",
		},
		{
			name:    "probe non-2xx status",
			status:  http.StatusInternalServerError,
			body:    `handshake timed out`,
			call:    func(c *Client) error { _, err := c.Probe(context.Background(), &utility.ProbeRequest{}); return err },
			wantErr: "probe failed with status 500: handshake timed out",
		},
		{
			name:    "probe malformed body",
			status:  http.StatusOK,
			body:    `{"success":`,
			call:    func(c *Client) error { _, err := c.Probe(context.Background(), &utility.ProbeRequest{}); return err },
			wantErr: "failed to parse probe response",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			err := tc.call(newHTTPOnlyClient(srv.URL))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestLongRunningJSONEndpoints_HonourContextCancellation(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))
	c := newHTTPOnlyClient(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.InferencePrompt(ctx, &utility.PromptRequest{}); err == nil ||
		!strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("InferencePrompt error = %v, want context canceled", err)
	}
	if _, err := c.Probe(ctx, &utility.ProbeRequest{}); err == nil ||
		!strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("Probe error = %v, want context canceled", err)
	}
}
