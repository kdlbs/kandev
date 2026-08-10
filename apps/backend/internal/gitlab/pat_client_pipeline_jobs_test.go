package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestPATClient_ListPipelineJobs_Paginates(t *testing.T) {
	pages := [][]PipelineJob{
		{{ID: 1, Name: "build", Status: "success"}},
		{{ID: 2, Name: "test", Status: "failed"}},
	}
	calls := 0
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/g/p/pipelines/42/jobs" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		page := calls
		calls++
		if page == 0 {
			w.Header().Set("Link", `<http://`+r.Host+`/api/v4/projects/g%2Fp/pipelines/42/jobs?page=2>; rel="next"`)
		}
		_ = json.NewEncoder(w).Encode(pages[page])
	}))
	t.Cleanup(stop)

	c := NewPATClient(host, "tok")
	jobs, err := c.ListPipelineJobs(context.Background(), "g/p", 42)
	if err != nil {
		t.Fatalf("ListPipelineJobs() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("server called %d times, want 2 (one per page)", calls)
	}
	if len(jobs) != 2 || jobs[0].Name != "build" || jobs[1].Name != "test" {
		t.Fatalf("jobs = %+v, want build+test across both pages", jobs)
	}
}

func TestPATClient_GetMRStatus_PopulatesJobCounts(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/projects/g/p/merge_requests/1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"iid": 1, "state": "opened", "source_branch": "feature", "sha": "abc123",
			})
		case "/projects/g/p/merge_requests/1/approvals":
			_ = json.NewEncoder(w).Encode(map[string]any{"approved_by": []any{}, "approvals_required": 0})
		case "/projects/g/p/pipelines":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 99, "status": "failed"}})
		case "/projects/g/p/pipelines/99/jobs":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "name": "build", "status": "success"},
				{"id": 2, "name": "test", "status": "failed"},
				{"id": 3, "name": "flaky", "status": "failed", "allow_failure": true},
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(stop)

	c := NewPATClient(host, "tok")
	status, err := c.GetMRStatus(context.Background(), "g/p", 1)
	if err != nil {
		t.Fatalf("GetMRStatus() error = %v", err)
	}
	if status.PipelineJobsTotal != 3 {
		t.Errorf("PipelineJobsTotal = %d, want 3", status.PipelineJobsTotal)
	}
	// build passes, flaky's allow_failure counts it as passing too; only
	// test (allow_failure=false, status=failed) is not passing.
	if status.PipelineJobsPassing != 2 {
		t.Errorf("PipelineJobsPassing = %d, want 2", status.PipelineJobsPassing)
	}
}

func TestPipelineJobBucket(t *testing.T) {
	cases := []struct {
		name string
		job  PipelineJob
		want string
	}{
		{"success", PipelineJob{Status: "success"}, pipelineJobBucketPassed},
		{pipelineStatusSkipped, PipelineJob{Status: pipelineStatusSkipped}, pipelineJobBucketPassed},
		{"failed", PipelineJob{Status: "failed"}, pipelineJobBucketFailed},
		{"failed-allow-failure", PipelineJob{Status: "failed", AllowFailure: true}, pipelineJobBucketPassed},
		{pipelineStatusCanceled, PipelineJob{Status: pipelineStatusCanceled}, pipelineJobBucketFailed},
		{"canceled-allow-failure", PipelineJob{Status: pipelineStatusCanceled, AllowFailure: true}, pipelineJobBucketPassed},
		{"running", PipelineJob{Status: "running"}, pipelineJobBucketInProgress},
		{"pending", PipelineJob{Status: "pending"}, pipelineJobBucketInProgress},
		{"manual", PipelineJob{Status: "manual"}, pipelineJobBucketInProgress},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pipelineJobBucket(tc.job); got != tc.want {
				t.Errorf("pipelineJobBucket(%+v) = %q, want %q", tc.job, got, tc.want)
			}
		})
	}
}

func TestSummarizePipelineJobs(t *testing.T) {
	jobs := []PipelineJob{
		{ID: 1, Status: "success"},
		{ID: 2, Status: "failed"},
		{ID: 3, Status: "failed", AllowFailure: true},
		{ID: 4, Status: "running"},
	}
	total, passing, failing := summarizePipelineJobs(jobs)
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
	if passing != 2 {
		t.Errorf("passing = %d, want 2", passing)
	}
	if len(failing) != 1 || failing[0].ID != 2 {
		t.Errorf("failing = %+v, want only job 2", failing)
	}
}
