package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestActionsRunProjectionPreservesCreationTime(t *testing.T) {
	var raw actionsRunResponse
	if err := json.Unmarshal([]byte(`{
		"id":101,
		"created_at":"2026-08-30T12:00:01Z"
	}`), &raw); err != nil {
		t.Fatal(err)
	}

	run := projectActionsRun(raw)
	want := time.Date(2026, 8, 30, 12, 0, 1, 0, time.UTC)
	if run == nil {
		t.Fatal("projected Actions run is nil")
	}
	if !run.CreatedAt.Equal(want) {
		t.Fatalf("projected created_at = %v, want %v", run.CreatedAt, want)
	}
}

func TestTokenClientActionsRunAndWorkflowReads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token installation-secret" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/repos/kdlbs/kandev/actions/runs/100":
			_, _ = w.Write([]byte(`{
				"id":100,"run_attempt":2,"workflow_id":77,"name":"E2E",
				"path":".github/workflows/e2e-tests.yml","event":"pull_request",
				"status":"completed","conclusion":"failure","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"head_branch":"feature/x","repository":{"full_name":"kdlbs/kandev"},
				"head_repository":{"full_name":"fork/kandev"},"pull_requests":[]
			}`))
		case "/repos/kdlbs/kandev/actions/workflows/77":
			_, _ = w.Write([]byte(`{"id":77,"name":"E2E","path":".github/workflows/e2e-tests.yml","state":"active"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newPATClientPointingAt(t, server.URL)
	client.token = "installation-secret"

	run, err := client.GetActionsRun(context.Background(), "kdlbs", "kandev", 100)
	if err != nil {
		t.Fatal(err)
	}
	if run.Attempt != 2 || run.WorkflowID != 77 || run.HeadRepository != "fork/kandev" || len(run.PullRequests) != 0 {
		t.Fatalf("run = %+v", run)
	}
	workflow, err := client.GetActionsWorkflow(context.Background(), "kdlbs", "kandev", 77)
	if err != nil {
		t.Fatal(err)
	}
	if workflow.Path != ".github/workflows/e2e-tests.yml" || workflow.State != "active" {
		t.Fatalf("workflow = %+v", workflow)
	}
}

func TestTokenClientActionsWritesUseClosedProviderInputs(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path == "/repos/kdlbs/kandev/actions/workflows/77/dispatches" {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["ref"] != "feature/x" {
				t.Fatalf("dispatch ref = %#v", payload["ref"])
			}
			inputs, _ := payload["inputs"].(map[string]any)
			if len(inputs) != 1 || inputs["fail_on_flaky"] != "false" {
				t.Fatalf("dispatch inputs = %#v", inputs)
			}
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	client := newPATClientPointingAt(t, server.URL)

	if err := client.RerunFailedActionsJobs(context.Background(), "kdlbs", "kandev", 100); err != nil {
		t.Fatal(err)
	}
	if err := client.DispatchActionsWorkflow(context.Background(), "kdlbs", "kandev", 77,
		"feature/x", map[string]string{"fail_on_flaky": "false"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/repos/kdlbs/kandev/actions/runs/100/rerun-failed-jobs",
		"/repos/kdlbs/kandev/actions/workflows/77/dispatches",
	}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestTokenClientActionsProviderFailureClassesAreRedacted(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		want      CIRunFailureClass
		wantRetry bool
	}{
		{name: "rerun ineligible", status: 422, body: `{"message":"This workflow run cannot be rerun"}`, want: CIRunFailureRerunIneligible},
		{name: "permission", status: 403, body: `{"message":"Must have admin rights to Repository token-secret"}`, want: CIRunFailureInstallationPermission},
		{name: "rate limit", status: 429, body: `{"message":"rate limit exceeded"}`, want: CIRunFailureProviderRateLimited, wantRetry: true},
		{name: "outage", status: 503, body: `private provider outage token-secret`, want: CIRunFailureProviderUnavailable, wantRetry: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client := newPATClientPointingAt(t, server.URL)
			err := client.RerunFailedActionsJobs(context.Background(), "kdlbs", "kandev", 100)
			var providerErr *CIRunProviderError
			if !errors.As(err, &providerErr) || providerErr.Class != tt.want || providerErr.Retryable != tt.wantRetry {
				t.Fatalf("error = %#v, want class %q retry=%v", err, tt.want, tt.wantRetry)
			}
			if strings.Contains(err.Error(), "token-secret") || strings.Contains(err.Error(), tt.body) {
				t.Fatalf("provider error leaked response body: %v", err)
			}
		})
	}
}

func TestActionsWriteCapabilityRequiresActionsWrite(t *testing.T) {
	read := CapabilitiesForPermissions(InstallationPermissions{"actions": PermissionRead})
	if read[CapabilityActionsWrite] {
		t.Fatal("actions read unexpectedly grants actions write")
	}
	write := CapabilitiesForPermissions(InstallationPermissions{"actions": PermissionWrite})
	if !write[CapabilityActionsRead] || !write[CapabilityActionsWrite] {
		t.Fatalf("actions write capabilities = %#v", write)
	}
}
