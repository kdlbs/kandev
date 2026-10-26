package nomad

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientRegisterUsesCreateOnlyCASAndNeverLeaksTokenOnRedirect(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPut || r.URL.Path != "/v1/jobs" {
			t.Fatalf("request = %s %s, want PUT /v1/jobs", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Nomad-Token") != "fixture-token" {
			t.Fatal("Nomad token missing from authorized API request")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var request JobRegisterRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		if !request.EnforceIndex || request.JobModifyIndex != 0 {
			t.Fatalf("registration was not create-only: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"EvalID":"eval-1","JobModifyIndex":1}`)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.RegisterCreateOnly(t.Context(), Job{ID: "job-1"})
	if err != nil || result.EvalID != "eval-1" || requests != 1 {
		t.Fatalf("RegisterCreateOnly() = %+v, %v, requests=%d", result, err, requests)
	}
}

func TestClientRegisterTreatsTransportFailureAsUncertainAndDoesNotRetry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Redirect(w, r, "/target", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.RegisterCreateOnly(t.Context(), Job{ID: "job-1"})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "uncertain") {
		t.Fatalf("expected uncertain registration error, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("ambiguous registration retried %d times", requests)
	}
}

func TestClientRegisterClassifiesStableJobConflictForExactReconciliation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "conflict", http.StatusConflict)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.RegisterCreateOnly(t.Context(), Job{ID: "job-1"})
	if !errors.Is(err, ErrJobRegistrationConflict) {
		t.Fatalf("RegisterCreateOnly() error = %v, want stable-job conflict", err)
	}
}

func TestClientLooksUpOnlyStableJobAndNamespacedAllocations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Nomad-Token") != "fixture-token" {
			t.Fatal("read request omitted Nomad token")
		}
		switch r.URL.Path {
		case "/v1/job/job%2Fid":
			t.Fatal("unexpected escaped path representation")
		case "/v1/job/job-id":
			if r.URL.Query().Get("namespace") != "default" {
				t.Fatalf("job query namespace = %q", r.URL.Query().Get("namespace"))
			}
			_, _ = io.WriteString(w, `{"ID":"job-id","Namespace":"default","Meta":{"kandev.operation_id":"op-1"},"ModifyIndex":2}`)
		case "/v1/job/job-id/allocations":
			if r.URL.Query().Get("namespace") != "default" || r.URL.Query().Get("all") != "true" {
				t.Fatalf("allocation query = %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `[{"ID":"alloc-1","NodeID":"node-1","JobID":"job-id","Namespace":"default","DesiredStatus":"run","ClientStatus":"running","CreateIndex":3}]`)
		case "/v1/allocation/alloc-1":
			if r.URL.Query().Get("namespace") != "default" {
				t.Fatalf("full allocation namespace = %q", r.URL.Query().Get("namespace"))
			}
			_, _ = io.WriteString(w, `{"ID":"alloc-1","NodeID":"node-1","JobID":"job-id","Namespace":"default","DesiredStatus":"run","ClientStatus":"running","CreateIndex":3,"TaskStates":{"agentctl":{"State":"running"}},"AllocatedResources":{"Shared":{"Ports":[{"Label":"agentctl_control","Value":22341},{"Label":"agentctl_instance","Value":22342}]}}}`)
		default:
			t.Fatalf("unexpected Nomad API path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	job, err := client.GetJob(t.Context(), "default", "job-id")
	if err != nil || job.ID != "job-id" || job.ModifyIndex != 2 {
		t.Fatalf("GetJob = %+v, %v", job, err)
	}
	allocs, err := client.ListAllocations(t.Context(), "default", "job-id")
	if err != nil || len(allocs) != 1 || allocs[0].ClientStatus != "running" {
		t.Fatalf("ListAllocations = %+v, %v", allocs, err)
	}
	if allocs[0].AllocatedResources != nil {
		t.Fatal("allocation summary unexpectedly included full resource data; test must model Nomad's summary response")
	}
	allocation, err := client.GetAllocation(t.Context(), "default", allocs[0].ID)
	if err != nil || allocation.ID != "alloc-1" || allocation.AllocatedResources == nil || len(allocation.AllocatedResources.Shared.Ports) != 2 {
		t.Fatalf("GetAllocation = %+v, %v", allocation, err)
	}
}

func TestBindSingleRunningAllocationRequiresOneExactIdentityAndBothPorts(t *testing.T) {
	state := validTestState()
	state.JobID, _ = StableJobID(JobIdentity{EnvironmentID: state.EnvironmentID, Generation: state.Generation, OperationID: state.OperationID, InstallationID: state.InstallationID})
	state.Phase = "registered"
	allocation := AllocationRecord{
		ID: "alloc-1", NodeID: state.NodeID, JobID: state.JobID, Namespace: state.Namespace,
		DesiredStatus: "run", ClientStatus: "running", CreateIndex: 12,
		TaskStates: map[string]AllocationTaskState{"agentctl": {State: "running"}},
		AllocatedResources: &AllocationResources{Shared: AllocationSharedResources{Ports: []AllocationPort{
			{Label: "agentctl_control", Value: 22341}, {Label: "agentctl_instance", Value: 22342},
		}}},
	}
	bound, err := BindSingleRunningAllocation(state, []AllocationRecord{allocation})
	if err != nil || bound.Phase != "allocated" || bound.AllocationID != allocation.ID || bound.AllocationCreateIndex != allocation.CreateIndex || bound.ControlPort != 22341 || bound.InstancePort != 22342 {
		t.Fatalf("allocation binding = %+v, err=%v", bound, err)
	}
	for name, records := range map[string][]AllocationRecord{
		"empty history":    nil,
		"multiple history": {allocation, allocation},
		"wrong node":       func() []AllocationRecord { x := allocation; x.NodeID = "node-other"; return []AllocationRecord{x} }(),
		"missing ports":    func() []AllocationRecord { x := allocation; x.AllocatedResources = nil; return []AllocationRecord{x} }(),
		"same ports": func() []AllocationRecord {
			x := allocation
			x.AllocatedResources.Shared.Ports[1].Value = x.AllocatedResources.Shared.Ports[0].Value
			return []AllocationRecord{x}
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BindSingleRunningAllocation(state, records); err == nil {
				t.Fatal("BindSingleRunningAllocation accepted ambiguous allocation data")
			}
		})
	}
}
