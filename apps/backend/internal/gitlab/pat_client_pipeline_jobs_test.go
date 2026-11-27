package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
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
		{"optional-manual", PipelineJob{Status: "manual", AllowFailure: true}, pipelineJobBucketPassed},
	}
	for _, status := range []string{"running", "pending", "created", "scheduled", "unknown"} {
		for _, allowed := range []bool{false, true} {
			cases = append(cases, struct {
				name string
				job  PipelineJob
				want string
			}{status, PipelineJob{Status: status, AllowFailure: allowed}, pipelineJobBucketInProgress})
		}
	}
	// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.18
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

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.16
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.19
func TestPATClient_MRJobCounts_OptionalManual(t *testing.T) {
	cases := []struct {
		name     string
		jobs     []PipelineJob
		passing  int
		pipeline string
	}{
		{"optional manual", []PipelineJob{{ID: 1, Status: "success"}, optionalManualJob()}, 2, "success"},
		{"blocking manual", []PipelineJob{{ID: 1, Status: "success"}, {ID: 2, Status: "manual"}}, 1, "manual"},
		{"allowed running", []PipelineJob{{ID: 1, Status: "success"}, {ID: 2, Status: "running", AllowFailure: true}}, 1, "running"},
		{"allowed failed", []PipelineJob{{ID: 1, Status: "success"}, {ID: 2, Status: "failed", AllowFailure: true}}, 2, "success"},
		{"mixed", mixedPipelineJobs(), 5, "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, stop := pipelineSummaryServer(t, tc.jobs, tc.pipeline)
			t.Cleanup(stop)
			client := NewPATClient(host, "disposable-token")
			status, err := client.GetMRStatus(context.Background(), "team/service", 23)
			if err != nil {
				t.Fatal(err)
			}
			if status.PipelineJobsTotal != len(tc.jobs) || status.PipelineJobsPassing != tc.passing {
				t.Errorf("GetMRStatus counts = %d/%d, want %d/%d", status.PipelineJobsPassing, status.PipelineJobsTotal, tc.passing, len(tc.jobs))
			}
			feedback, err := client.GetMRFeedback(context.Background(), "team/service", 23)
			if err != nil {
				t.Fatal(err)
			}
			if len(feedback.Pipelines) != 1 {
				t.Fatalf("pipelines = %+v", feedback.Pipelines)
			}
			pipeline := feedback.Pipelines[0]
			if pipeline.JobsTotal != len(tc.jobs) || pipeline.JobsPassing != tc.passing {
				t.Errorf("GetMRFeedback counts = %d/%d, want %d/%d", pipeline.JobsPassing, pipeline.JobsTotal, tc.passing, len(tc.jobs))
			}
			if pipeline.Status != tc.pipeline || !reflect.DeepEqual(pipeline.Jobs, tc.jobs) {
				t.Errorf("raw pipeline/job metadata changed: %+v", pipeline)
			}
		})
	}
}

func pipelineSummaryServer(t *testing.T, jobs []PipelineJob, pipeline string) (string, func()) {
	t.Helper()
	return newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		switch r.URL.Path {
		case "/projects/team/service/merge_requests/23":
			body = map[string]any{"id": 123, "iid": 23, "title": "CI summary", "state": "opened", "source_branch": "feature", "target_branch": "main", "sha": "abc123", "web_url": "https://gitlab.example/team/service/-/merge_requests/23", "merge_status": "can_be_merged"}
		case "/projects/team/service/merge_requests/23/approvals":
			body = map[string]any{"approved_by": []any{}, "approvals_required": 0}
		case "/projects/team/service/merge_requests/23/discussions":
			body = []any{}
		case "/projects/team/service/pipelines":
			body = []map[string]any{{"id": 52, "status": pipeline, "ref": "feature", "sha": "abc123"}}
		case "/projects/team/service/pipelines/52/jobs":
			body = jobs
		default:
			t.Errorf("unexpected provider path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error(err)
		}
	}))
}

func optionalManualJob() PipelineJob {
	return PipelineJob{ID: 2, Name: "deploy", Stage: "deploy", Status: "manual", AllowFailure: true, WebURL: "https://gitlab.example/jobs/2"}
}

func mixedPipelineJobs() []PipelineJob {
	return []PipelineJob{
		{ID: 1, Status: "success"}, optionalManualJob(),
		{ID: 3, Status: "skipped"}, {ID: 4, Status: "failed", AllowFailure: true},
		{ID: 5, Status: "manual"}, {ID: 6, Status: "running", AllowFailure: true},
		{ID: 7, Status: "pending", AllowFailure: true}, {ID: 8, Status: "failed"},
		{ID: 9, Status: "canceled"}, {ID: 10, Status: "canceled", AllowFailure: true},
	}
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.17
func TestSummarizePipelineJobs_MixedManual(t *testing.T) {
	jobs := mixedPipelineJobs()
	before := append([]PipelineJob(nil), jobs...)
	total, passing, failing := summarizePipelineJobs(jobs)
	if total != 10 || passing != 5 {
		t.Errorf("counts = %d/%d, want 5/10", passing, total)
	}
	if !reflect.DeepEqual(failing, []PipelineJob{jobs[7], jobs[8]}) {
		t.Errorf("failing = %+v, want only non-allowed failed/canceled jobs", failing)
	}
	if !reflect.DeepEqual(jobs, before) {
		t.Error("raw input jobs changed")
	}
}
