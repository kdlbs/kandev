package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const expectedHeadSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestGitLabMergeSendsShaAndSquash(t *testing.T) {
	var body struct {
		SHA                      string `json:"sha"`
		Squash                   bool   `json:"squash"`
		MergeWhenPipelineSucceed *bool  `json:"merge_when_pipeline_succeeds"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"sha":"` + mergeHeadSHA + `"}`))
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			_, _ = w.Write([]byte(`{"iid":17,"state":"merged","merge_commit_sha":"abc123"}`))
		}
	}))
	t.Cleanup(server.Close)

	mr, err := NewPATClient(server.URL, "token").MergeMR(t.Context(), "group/project", 17, true, "", expectedHeadSHA)
	if err != nil {
		t.Fatal(err)
	}
	if body.SHA != expectedHeadSHA || !body.Squash || body.MergeWhenPipelineSucceed != nil {
		t.Errorf("request body = %+v, want sha %s, squash, no deferred merge", body, expectedHeadSHA)
	}
	if mr.MergeCommitSHA != "abc123" {
		t.Errorf("MergeCommitSHA = %q", mr.MergeCommitSHA)
	}
}

func TestGitLabMergeStatusMapsToHeadMoved(t *testing.T) {
	for _, tc := range []struct {
		status    int
		headMoved bool
	}{{http.StatusConflict, true}, {http.StatusMethodNotAllowed, false}, {http.StatusUnprocessableEntity, false}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"sha":"` + mergeHeadSHA + `"}`))
				return
			}
			w.WriteHeader(tc.status)
		}))
		_, err := NewPATClient(server.URL, "token").MergeMR(t.Context(), "g/p", 1, false, "", expectedHeadSHA)
		server.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if got := errors.Is(err, ErrMergeHeadMoved); got != tc.headMoved {
			t.Errorf("status %d: errors.Is(ErrMergeHeadMoved) = %v, want %v", tc.status, got, tc.headMoved)
		}
	}
}

func TestMockMergeMRRefusesMovedHead(t *testing.T) {
	mock := NewMockClient("https://gitlab.example.com")
	mock.SeedMR("g/p", &MR{IID: 1, HeadSHA: mergeHeadSHA, State: mrStateOpen})
	if _, err := mock.MergeMR(t.Context(), "g/p", 1, false, "", expectedHeadSHA); !errors.Is(err, ErrMergeHeadMoved) {
		t.Fatalf("error = %v, want ErrMergeHeadMoved", err)
	}
	if _, err := mock.MergeMR(t.Context(), "g/p", 1, false, "", mergeHeadSHA); err != nil {
		t.Fatalf("matching head: %v", err)
	}
}

func TestReadMRLiveForAutomationCountsUnresolvedDiscussions(t *testing.T) {
	svc, mock := newMRAutomationAuthFixture(t)
	mock.SeedMR("group/project", &MR{IID: 1, HeadBranch: "feature", HeadSHA: mergeHeadSHA, State: mrStateOpen})
	mock.SeedDiscussions("group/project", 1, []MRDiscussion{
		{ID: "d1", Resolvable: true},
		{ID: "d2", Resolvable: true, Resolved: true},
		{ID: "d3"},
	})
	read, err := svc.ReadMRLiveForAutomation(context.Background(), "ws-1", "https://gitlab.example.com", "group/project", 1)
	if err != nil {
		t.Fatal(err)
	}
	if read.UnresolvedDiscussions != 1 || read.Status.MR.HeadSHA != mergeHeadSHA {
		t.Errorf("read = %+v", read)
	}
	if _, err := svc.ReadMRLiveForAutomation(context.Background(), "ws-1", "https://other.example.com", "group/project", 1); err == nil {
		t.Error("host mismatch must fail")
	}
}

func TestProjectProtectionReadsSettingsAndApprovalRules(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules string
		want  ProjectProtection
	}{
		{"rules require approvals", `[{"approvals_required":2,"protected_branches":[]}]`, ProjectProtection{PipelineRequired: true, ThreadsRequired: true, ApprovalsKnown: true, ApprovalsRequired: true}},
		{"rule for another branch", `[{"approvals_required":1,"protected_branches":[{"name":"release"}]}]`, ProjectProtection{PipelineRequired: true, ThreadsRequired: true, ApprovalsKnown: true}},
		{"rules unreadable", ``, ProjectProtection{PipelineRequired: true, ThreadsRequired: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.EscapedPath() {
				case "/api/v4/projects/g%2Fp":
					_, _ = w.Write([]byte(`{"default_branch":"main","only_allow_merge_if_pipeline_succeeds":true,"only_allow_merge_if_all_discussions_are_resolved":true}`))
				case "/api/v4/projects/g%2Fp/approval_rules":
					if tc.rules == "" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					_, _ = w.Write([]byte(tc.rules))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			got, err := NewPATClient(server.URL, "token").GetProjectProtection(t.Context(), "g/p", "")
			if err != nil {
				t.Fatal(err)
			}
			if *got != tc.want {
				t.Errorf("protection = %+v, want %+v", *got, tc.want)
			}
		})
	}
}

func TestProjectProtectionFailedProjectReadIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	t.Cleanup(server.Close)
	if _, err := NewPATClient(server.URL, "token").GetProjectProtection(t.Context(), "g/p", "main"); err == nil {
		t.Fatal("expected error")
	}
}

func TestListBranchCommitsPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref_name") != "main" || r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "100" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"id":"c1","message":"This reverts commit abc"}]`))
	}))
	t.Cleanup(server.Close)
	commits, err := NewPATClient(server.URL, "token").ListBranchCommits(t.Context(), "g/p", "main", 2)
	if err != nil || len(commits) != 1 || commits[0].SHA != "c1" {
		t.Fatalf("commits = %+v, err = %v", commits, err)
	}
}

func TestProjectSquashOptionIsReadRaw(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"merge_method":"ff","squash_option":"default_on"}`))
	}))
	t.Cleanup(server.Close)
	got, err := NewPATClient(server.URL, "token").GetProjectSquashOption(t.Context(), "g/p")
	if err != nil || got != "default_on" {
		t.Fatalf("squash option = %q, err = %v", got, err)
	}
}

func TestRequireHeadPipelineDowngradesStalePipeline(t *testing.T) {
	for _, tc := range []struct {
		name      string
		pipelines []Pipeline
		want      string
	}{
		{"head pipeline kept", []Pipeline{{SHA: mergeHeadSHA, Status: "success"}}, "success"},
		{"older pipeline pending", []Pipeline{{SHA: "old", Status: "success"}}, "pending"},
		{"no pipeline pending", nil, "pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := NewMockClient("https://gitlab.example.com")
			mock.SeedPipelines("g/p", tc.pipelines)
			status := &MRStatus{MR: &MR{HeadSHA: mergeHeadSHA, HeadBranch: "b"}, PipelineState: "success"}
			if err := requireHeadPipeline(context.Background(), mock, "g/p", status); err != nil {
				t.Fatal(err)
			}
			if status.PipelineState != tc.want {
				t.Errorf("PipelineState = %q, want %q", status.PipelineState, tc.want)
			}
		})
	}
}
