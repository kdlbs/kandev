package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.34
func TestStripDraftMarkers(t *testing.T) {
	for _, tc := range []struct {
		name      string
		title     string
		want      string
		wantFound bool
	}{
		{name: "colon", title: "Draft: add thing", want: "add thing", wantFound: true},
		{name: "lowercase colon", title: "draft: add thing", want: "add thing", wantFound: true},
		{name: "brackets", title: "[Draft] add thing", want: "add thing", wantFound: true},
		{name: "parens", title: "(Draft) add thing", want: "add thing", wantFound: true},
		{name: "uppercase", title: "DRAFT: add thing", want: "add thing", wantFound: true},
		{name: "repeated mixed", title: "Draft: [Draft]  (draft) add thing", want: "add thing", wantFound: true},
		{name: "unicode whitespace between markers", title: "Draft:\u00a0[Draft]\u2003(draft)\u00a0fix", want: "fix", wantFound: true},
		{name: "leading whitespace", title: "  \tDraft:   add thing", want: "add thing", wantFound: true},
		{name: "marker only", title: "Draft:", want: "", wantFound: true},
		{name: "no prefix", title: "add thing", want: "add thing"},
		{name: "mid title", title: "add Draft: thing", want: "add Draft: thing"},
		{name: "mid title after marker", title: "Draft: add [Draft] thing", want: "add [Draft] thing", wantFound: true},
		{name: "word starting with draft", title: "Drafting notes", want: "Drafting notes"},
		{name: "empty", title: "", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, found := stripDraftMarkers(tc.title)
			if got != tc.want || found != tc.wantFound {
				t.Errorf("stripDraftMarkers(%q) = (%q, %v), want (%q, %v)", tc.title, got, found, tc.want, tc.wantFound)
			}
		})
	}
}

type readyServer struct {
	mu       sync.Mutex
	gets     int
	puts     []string
	getBody  func(n int) (int, string)
	putCode  int
	mergePut int
}

func (s *readyServer) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /api/v4/projects/g%2Fp/merge_requests/7":
			s.gets++
			code, body := s.getBody(s.gets)
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		case "PUT /api/v4/projects/g%2Fp/merge_requests/7":
			raw, _ := io.ReadAll(r.Body)
			s.puts = append(s.puts, string(raw))
			code := s.putCode
			if code == 0 {
				code = http.StatusOK
			}
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{}`))
		case "GET /api/v4/projects/g%2Fp/merge_requests/7/approvals":
			_, _ = w.Write([]byte(`{"approved_by":[],"approvals_required":0}`))
		case "GET /api/v4/projects/g%2Fp/pipelines":
			_, _ = w.Write([]byte(`[]`))
		case "PUT /api/v4/projects/g%2Fp/merge_requests/7/merge":
			s.mergePut++
			_, _ = w.Write([]byte(`{"iid":7,"state":"merged"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
			http.Error(w, "unexpected", 404)
		}
	})
}

func newReadyServer(t *testing.T, getBody func(n int) (int, string)) (*readyServer, *PATClient) {
	t.Helper()
	rs := &readyServer{getBody: getBody}
	srv := httptest.NewServer(rs.handler(t))
	t.Cleanup(srv.Close)
	return rs, NewPATClient(srv.URL, "tok")
}

const (
	draftBody  = `{"iid":7,"title":"Draft: add thing","state":"opened","draft":true,"detailed_merge_status":"draft_status","sha":"abc"}`
	readyBody  = `{"iid":7,"title":"add thing","state":"opened","draft":false,"detailed_merge_status":"checking","sha":"abc"}`
	closedBody = `{"iid":7,"title":"add thing","state":"closed","draft":true,"sha":"abc"}`
)

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.34
func TestPATMarkMRReady(t *testing.T) {
	for _, tc := range []struct {
		name      string
		get       func(n int) (int, string)
		putCode   int
		wantErr   error
		wantPuts  int
		wantTitle string
		wantDraf  bool
		wantNil   bool
	}{
		{name: "success", get: func(n int) (int, string) {
			if n == 1 {
				return 200, draftBody
			}
			return 200, readyBody
		}, wantPuts: 1, wantTitle: "add thing"},
		{name: "not draft is a no-op", get: func(int) (int, string) { return 200, readyBody }},
		{name: "draft status without flag", get: func(n int) (int, string) {
			if n == 1 {
				return 200, `{"iid":7,"title":"Draft: x","state":"opened","detailed_merge_status":"draft_status"}`
			}
			return 200, readyBody
		}, wantPuts: 1, wantTitle: "x"},
		{name: "not open", get: func(int) (int, string) { return 200, closedBody }, wantErr: ErrMRNotOpen},
		{name: "marker not at start", get: func(int) (int, string) {
			return 200, `{"iid":7,"title":"add Draft: x","state":"opened","draft":true}`
		}, wantErr: ErrDraftMarkerNotFound},
		{name: "empty after strip", get: func(int) (int, string) {
			return 200, `{"iid":7,"title":"Draft:","state":"opened","draft":true}`
		}, wantErr: ErrDraftTitleEmpty},
		{name: "reread fails", get: func(n int) (int, string) {
			if n == 1 {
				return 200, draftBody
			}
			return 503, `{}`
		}, wantErr: ErrMRReadAfterReady, wantPuts: 1, wantTitle: "add thing"},
		{name: "reread still draft", get: func(int) (int, string) { return 200, draftBody }, wantErr: ErrMRStillDraft, wantPuts: 1, wantTitle: "add thing"},
		{name: "reread closed wins over draft flag", get: func(n int) (int, string) {
			if n == 1 {
				return 200, draftBody
			}
			return 200, closedBody
		}, wantPuts: 1, wantTitle: "add thing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs, client := newReadyServer(t, tc.get)
			rs.putCode = tc.putCode
			mr, err := client.MarkMRReady(t.Context(), "g/p", 7)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			} else if err != nil || mr == nil {
				t.Fatalf("mr, err = %v, %v, want an MR", mr, err)
			}
			if len(rs.puts) != tc.wantPuts {
				t.Errorf("title writes = %d, want %d", len(rs.puts), tc.wantPuts)
			}
			if tc.wantPuts == 1 {
				var body map[string]string
				if err := json.Unmarshal([]byte(rs.puts[0]), &body); err != nil {
					t.Errorf("decode title write body %s: %v", rs.puts[0], err)
				} else if body["title"] != tc.wantTitle {
					t.Errorf("title write body = %s, want title %q", rs.puts[0], tc.wantTitle)
				}
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.34
func TestPATMarkMRReadyProviderFailuresAreNotWrapped(t *testing.T) {
	_, client := newReadyServer(t, func(int) (int, string) { return 404, `{}` })
	_, err := client.MarkMRReady(t.Context(), "g/p", 7)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("first read failure = %v, want APIError 404", err)
	}
	rs, client := newReadyServer(t, func(int) (int, string) { return 200, draftBody })
	rs.putCode = http.StatusForbidden
	_, err = client.MarkMRReady(t.Context(), "g/p", 7)
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 403 || errors.Is(err, ErrMRReadAfterReady) {
		t.Fatalf("title write failure = %v, want APIError 403", err)
	}
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.31
func TestPATMergeMRRefusesDraftWithoutWrite(t *testing.T) {
	for name, body := range map[string]string{
		"draft flag":                          `{"iid":7,"draft":true,"sha":"abc"}`,
		"work in progress":                    `{"iid":7,"work_in_progress":true,"sha":"abc"}`,
		"draft status":                        `{"iid":7,"detailed_merge_status":"draft_status","sha":"abc"}`,
		"draft beats mergeable and empty sha": `{"iid":7,"draft":true,"detailed_merge_status":"mergeable"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rs, client := newReadyServer(t, func(int) (int, string) { return 200, body })
			_, err := client.MergeMR(t.Context(), "g/p", 7, false, "", "")
			if !errors.Is(err, ErrMRIsDraft) {
				t.Fatalf("err = %v, want ErrMRIsDraft", err)
			}
			if rs.mergePut != 0 {
				t.Errorf("merge writes = %d, want 0", rs.mergePut)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.33
func TestPATMergeMRWrapsRefusalStatusesOnWriteOnly(t *testing.T) {
	for _, status := range []int{405, 406, 409, 422} {
		var refused *MergeRefusedError
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"iid":7,"sha":"abc"}`))
				return
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"secret provider text"}`))
		}))
		client := NewPATClient(srv.URL, "tok")
		_, err := client.MergeMR(t.Context(), "g/p", 7, false, "", "")
		srv.Close()
		if !errors.As(err, &refused) || refused.Status != status {
			t.Fatalf("status %d: err = %v, want MergeRefusedError", status, err)
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
			t.Errorf("status %d: wrapped APIError missing", status)
		}
		if strings.Contains(err.Error(), "secret provider text") {
			t.Errorf("status %d: error text leaks provider body: %v", status, err)
		}
	}
	// A failed head read with the same status is not a refusal.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(405) }))
	defer srv.Close()
	_, err := NewPATClient(srv.URL, "tok").MergeMR(t.Context(), "g/p", 7, false, "", "")
	var refused *MergeRefusedError
	if errors.As(err, &refused) {
		t.Fatalf("head read failure must not be a MergeRefusedError: %v", err)
	}
}

func newMergeRouter(t *testing.T, serverURL string) (*gin.Engine, *Store, *bus.MemoryEventBus) {
	t.Helper()
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-test")
	if err := store.UpsertConfigForWorkspace(t.Context(), "workspace-test", &GitLabConfig{Host: serverURL, AuthMethod: AuthMethodPAT}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	svc.SetStore(store)
	svc.SetWorkspaceSecretStore(&configTestSecrets{values: map[string]string{SecretKeyForWorkspace("workspace-test"): "workspace-token"}})
	eventBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetEventBus(eventBus)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)
	return router, store, eventBus
}

func doRoute(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.31
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.33
func TestMergeRouteMapsDraftAndRefusals(t *testing.T) {
	for _, tc := range []struct {
		name       string
		head       string
		putStatus  int
		wantStatus int
		wantMsg    string
	}{
		{name: "draft", head: `{"iid":7,"draft":true,"sha":"abc"}`, wantStatus: 409, wantMsg: "This merge request is a draft; mark it ready first"},
		{name: "405", head: `{"iid":7,"sha":"abc"}`, putStatus: 405, wantStatus: 409, wantMsg: "GitLab cannot merge this merge request yet"},
		{name: "406", head: `{"iid":7,"sha":"abc"}`, putStatus: 406, wantStatus: 409, wantMsg: "GitLab cannot merge this merge request yet"},
		{name: "409", head: `{"iid":7,"sha":"abc"}`, putStatus: 409, wantStatus: 409, wantMsg: "The source branch changed after it was read; refresh and retry"},
		{name: "422", head: `{"iid":7,"sha":"abc"}`, putStatus: 422, wantStatus: 422, wantMsg: "GitLab rejected the merge options"},
		{name: "500 keeps mapping", head: `{"iid":7,"sha":"abc"}`, putStatus: 500, wantStatus: 502, wantMsg: providerActionFailedMessage + ": merge request merge"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var merges int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.EscapedPath() {
				case "GET /api/v4/projects/g%2Fp":
					_, _ = w.Write([]byte(`{"merge_method":"merge"}`))
				case "GET /api/v4/projects/g%2Fp/merge_requests/7":
					_, _ = w.Write([]byte(tc.head))
				case "PUT /api/v4/projects/g%2Fp/merge_requests/7/merge":
					merges++
					w.WriteHeader(tc.putStatus)
					_, _ = w.Write([]byte(`{"message":"hostile <script> provider text"}`))
				default:
					http.Error(w, "unexpected", 404)
				}
			}))
			t.Cleanup(server.Close)
			router, _, _ := newMergeRouter(t, server.URL)
			rec := doRoute(router, "/api/v1/gitlab/mrs/merge?workspace_id=workspace-test", `{"project":"g/p","iid":7,"method":"merge"}`)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			var out map[string]string
			_ = json.Unmarshal(rec.Body.Bytes(), &out)
			if out["error"] != tc.wantMsg {
				t.Errorf("error = %q, want %q", out["error"], tc.wantMsg)
			}
			if tc.name == "draft" && merges != 0 {
				t.Errorf("merge writes = %d, want 0", merges)
			}
			if strings.Contains(rec.Body.String(), "hostile") {
				t.Errorf("provider text leaked: %s", rec.Body.String())
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.34
func TestMarkReadyRoute(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		get        func(n int) (int, string)
		wantStatus int
		wantMsg    string
	}{
		{name: "blank project", body: `{"project":"","iid":7}`, wantStatus: 400, wantMsg: "project and a positive iid are required"},
		{name: "zero iid", body: `{"project":"g/p","iid":0}`, wantStatus: 400, wantMsg: "project and a positive iid are required"},
		{name: "success", body: `{"project":"g/p","iid":7}`, get: func(n int) (int, string) {
			if n == 1 {
				return 200, draftBody
			}
			return 200, readyBody
		}, wantStatus: 200},
		{name: "not open", body: `{"project":"g/p","iid":7}`, get: func(int) (int, string) { return 200, closedBody }, wantStatus: 409, wantMsg: "This merge request is not open; reopen it in GitLab before marking it ready"},
		{name: "marker not found", body: `{"project":"g/p","iid":7}`, get: func(int) (int, string) {
			return 200, `{"iid":7,"title":"x Draft: y","state":"opened","draft":true}`
		}, wantStatus: 422, wantMsg: "The draft marker is not at the start of the title; edit the title in GitLab"},
		{name: "title empty", body: `{"project":"g/p","iid":7}`, get: func(int) (int, string) {
			return 200, `{"iid":7,"title":"Draft:","state":"opened","draft":true}`
		}, wantStatus: 422, wantMsg: "The title would be empty without the draft marker; edit the title in GitLab"},
		{name: "still draft", body: `{"project":"g/p","iid":7}`, get: func(int) (int, string) { return 200, draftBody }, wantStatus: 409, wantMsg: "GitLab still reports this merge request as a draft; refresh and try again, or check it in GitLab"},
		{name: "reread failed", body: `{"project":"g/p","iid":7}`, get: func(n int) (int, string) {
			if n == 1 {
				return 200, draftBody
			}
			return 503, `{}`
		}, wantStatus: 502, wantMsg: "Merge request title updated, but its status could not be refreshed; refresh the panel"},
		{name: "first read 403", body: `{"project":"g/p","iid":7}`, get: func(int) (int, string) { return 403, `{}` }, wantStatus: 403, wantMsg: "GitLab permission denied"},
		{name: "first read 500", body: `{"project":"g/p","iid":7}`, get: func(int) (int, string) { return 500, `{}` }, wantStatus: 502, wantMsg: providerActionFailedMessage + ": mark merge request ready"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			get := tc.get
			if get == nil {
				get = func(int) (int, string) { return 200, draftBody }
			}
			rs := &readyServer{getBody: get}
			server := httptest.NewServer(rs.handler(t))
			t.Cleanup(server.Close)
			router, _, _ := newMergeRouter(t, server.URL)
			rec := doRoute(router, "/api/v1/gitlab/mrs/ready?workspace_id=workspace-test", tc.body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantMsg != "" {
				var out map[string]string
				_ = json.Unmarshal(rec.Body.Bytes(), &out)
				if out["error"] != tc.wantMsg {
					t.Errorf("error = %q, want %q", out["error"], tc.wantMsg)
				}
			}
			if tc.wantStatus == 200 {
				var mr MR
				if err := json.Unmarshal(rec.Body.Bytes(), &mr); err != nil || mr.Title != "add thing" || mr.Draft {
					t.Errorf("body = %s, want re-read MR", rec.Body.String())
				}
			}
			if tc.wantStatus == 400 && rs.gets != 0 {
				t.Errorf("GitLab was called %d times before validation", rs.gets)
			}
		})
	}
}

func TestMarkReadyRoutePublishesPersistedDiscussionCount(t *testing.T) {
	rs := &readyServer{getBody: func(n int) (int, string) {
		if n == 1 {
			return http.StatusOK, draftBody
		}
		return http.StatusOK, readyBody
	}}
	server := httptest.NewServer(rs.handler(t))
	t.Cleanup(server.Close)
	router, store, eventBus := newMergeRouter(t, server.URL)
	seedTask(t, store, "task-ready", "workspace-test")
	if err := store.UpsertTaskMR(t.Context(), &TaskMR{
		TaskID: "task-ready", Host: server.URL, ProjectPath: "g/p", MRIID: 7,
		MRTitle: "Draft: add thing", State: mrStateOpen, Draft: true,
		UnresolvedDiscussions: 3,
	}); err != nil {
		t.Fatalf("seed task MR: %v", err)
	}

	var received []*TaskMRUpdatedEvent
	if _, err := eventBus.Subscribe(events.GitLabTaskMRUpdated, func(_ context.Context, event *bus.Event) error {
		if payload, ok := event.Data.(*TaskMRUpdatedEvent); ok {
			received = append(received, payload)
		}
		return nil
	}); err != nil {
		t.Fatalf("subscribe to task MR updates: %v", err)
	}

	rec := doRoute(router, "/api/v1/gitlab/mrs/ready?workspace_id=workspace-test", `{"project":"g/p","iid":7}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	stored, err := store.GetTaskMR(t.Context(), "task-ready", "", "g/p", 7)
	if err != nil || stored == nil || stored.UnresolvedDiscussions != 3 || stored.Draft || stored.MRTitle != "add thing" {
		t.Fatalf("stored MR = %+v, %v; want ready title with 3 unresolved discussions", stored, err)
	}
	if len(received) != 1 {
		t.Fatalf("task MR update events = %d, want 1", len(received))
	}
	if received[0].WorkspaceID != "workspace-test" || received[0].UnresolvedDiscussions != 3 {
		t.Fatalf("published MR = %+v, want workspace and 3 unresolved discussions", received[0])
	}
}

func TestMockClientMarkMRReady(t *testing.T) {
	mock := NewMockClient("https://gitlab.example")
	mock.SeedMR("g/p", &MR{IID: 7, Title: "Draft: add thing", State: mrStateOpen, Draft: true, DetailedMergeStatus: "draft_status"})
	if _, err := mock.MergeMR(t.Context(), "g/p", 7, false, "", ""); !errors.Is(err, ErrMRIsDraft) {
		t.Fatalf("mock merge of draft = %v, want ErrMRIsDraft", err)
	}
	mr, err := mock.MarkMRReady(t.Context(), "g/p", 7)
	if err != nil || mr.Title != "add thing" || mr.Draft || mr.DetailedMergeStatus != "checking" {
		t.Fatalf("MarkMRReady = %+v, %v", mr, err)
	}
	// GetMR and GetMRStatus never advance the transition.
	for range 3 {
		if got, _ := mock.GetMR(t.Context(), "g/p", 7); got.DetailedMergeStatus != "checking" {
			t.Fatalf("GetMR status = %q, want checking", got.DetailedMergeStatus)
		}
		if st, _ := mock.GetMRStatus(t.Context(), "g/p", 7); st.DetailedMergeStatus != "checking" {
			t.Fatalf("GetMRStatus status = %q, want checking", st.DetailedMergeStatus)
		}
	}
	// Feedback reads 1 and 2 report checking, read 3 mergeable.
	for i, want := range []string{"checking", "checking", "mergeable", "mergeable"} {
		fb, err := mock.GetMRFeedback(t.Context(), "g/p", 7)
		if err != nil || fb.MR.DetailedMergeStatus != want {
			t.Fatalf("feedback read %d status = %q (%v), want %q", i+1, fb.MR.DetailedMergeStatus, err, want)
		}
	}
	if _, err := mock.MarkMRReady(t.Context(), "g/p", 7); err != nil {
		t.Fatalf("no-op on a non-draft = %v", err)
	}
	if _, err := mock.MarkMRReady(t.Context(), "g/p", 99); err == nil {
		t.Fatal("unknown MR must fail")
	}
	if _, err := mock.MergeMR(t.Context(), "g/p", 7, false, "", ""); err != nil {
		t.Fatalf("merge after Mark as ready = %v", err)
	}
}

func TestMockClientMarkMRReadyAcceptsOpenedState(t *testing.T) {
	mock := NewMockClient("https://gitlab.example")
	mock.SeedMR("g/p", &MR{IID: 7, Title: "Draft: x", State: gitlabStateOpened, Draft: true})
	if mr, err := mock.MarkMRReady(t.Context(), "g/p", 7); err != nil || mr.Title != "x" {
		t.Fatalf("MarkMRReady = %+v, %v", mr, err)
	}
}

func TestNoopClientMarkMRReady(t *testing.T) {
	if _, err := NewNoopClient("").MarkMRReady(t.Context(), "g/p", 1); !errors.Is(err, ErrNoClient) {
		t.Fatalf("err = %v, want ErrNoClient", err)
	}
}

func seedRefreshRow(t *testing.T, svc *Service, store *Store, mock *MockClient, taskID string) {
	t.Helper()
	seedTask(t, store, taskID, "ws-1")
	if _, err := svc.SyncTaskMR(t.Context(), taskID, "", "g/p", 7); err != nil {
		t.Fatalf("sync %s: %v", taskID, err)
	}
}

type countingMRStatusClient struct {
	Client
	statusReads int
}

func (c *countingMRStatusClient) GetMRStatus(ctx context.Context, projectPath string, iid int) (*MRStatus, error) {
	c.statusReads++
	return c.Client.GetMRStatus(ctx, projectPath, iid)
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.35
func TestRefreshCachedTaskMRsAfterReady(t *testing.T) {
	svc, _, _ := newServiceFixture(t, userHandler(func(string) bool { return true }))
	store := withTaskMRStore(t, svc)
	mock := swapToMock(t, svc)
	seedWorkspace(t, store, "ws-1")
	now := time.Now().UTC()
	mock.SeedMR("g/p", &MR{IID: 7, Title: "Draft: add thing", HeadBranch: "x", BaseBranch: "main", State: mrStateOpen, Draft: true, CreatedAt: now})
	mock.SeedMR("g/p", &MR{IID: 8, Title: "other", HeadBranch: "y", BaseBranch: "main", State: mrStateOpen, CreatedAt: now})
	seedRefreshRow(t, svc, store, mock, "task-match")
	seedRefreshRow(t, svc, store, mock, "task-match-two")
	seedRefreshRow(t, svc, store, mock, "task-legacy")
	seedTask(t, store, "task-other", "ws-1")
	if _, err := svc.SyncTaskMR(t.Context(), "task-other", "", "g/p", 8); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE gitlab_task_mrs SET host = '' WHERE task_id = 'task-legacy'`); err != nil {
		t.Fatalf("blank legacy host: %v", err)
	}
	if _, err := mock.MarkMRReady(t.Context(), "g/p", 7); err != nil {
		t.Fatal(err)
	}

	counted := &countingMRStatusClient{Client: mock}
	svc.refreshCachedTaskMRs(t.Context(), counted, "ws-1", "g/p", 7)
	if counted.statusReads != 1 {
		t.Errorf("MR status reads = %d, want one read for the linked rows", counted.statusReads)
	}

	rows, _ := svc.ListTaskMRsByWorkspace(t.Context(), "ws-1")
	if got := rows["task-match"][0]; got.MRTitle != "add thing" || got.Draft {
		t.Errorf("matching row = title %q draft %v, want refreshed", got.MRTitle, got.Draft)
	}
	if got := rows["task-match-two"][0]; got.MRTitle != "add thing" || got.Draft {
		t.Errorf("second matching row = title %q draft %v, want refreshed", got.MRTitle, got.Draft)
	}
	if got := rows["task-legacy"][0]; got.Host != "" || got.MRTitle != "Draft: add thing" {
		t.Errorf("legacy empty-host row = host %q title %q, want untouched", got.Host, got.MRTitle)
	}
	if got := rows["task-other"][0]; got.MRTitle != "other" {
		t.Errorf("other MR row changed: %q", got.MRTitle)
	}
}

func TestRefreshCachedTaskMRsSurvivesFailuresAndMissingStore(t *testing.T) {
	svc, _, _ := newServiceFixture(t, userHandler(func(string) bool { return true }))
	store := withTaskMRStore(t, svc)
	mock := swapToMock(t, svc)
	seedWorkspace(t, store, "ws-1")
	mock.SeedMR("g/p", &MR{IID: 7, Title: "Draft: x", HeadBranch: "x", BaseBranch: "main", State: mrStateOpen, Draft: true, CreatedAt: time.Now().UTC()})
	seedRefreshRow(t, svc, store, mock, "task-a")
	seedRefreshRow(t, svc, store, mock, "task-b")
	// A client that cannot read status must not panic or abort the loop.
	svc.refreshCachedTaskMRs(t.Context(), NewNoopClient(mock.Host()), "ws-1", "g/p", 7)
	rows, _ := svc.ListTaskMRsByWorkspace(t.Context(), "ws-1")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 untouched", len(rows))
	}
	// A workspace with no rows and no store are both a quiet no-op.
	svc.refreshCachedTaskMRs(t.Context(), mock, "ws-missing", "g/p", 7)
	NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t)).
		refreshCachedTaskMRs(t.Context(), mock, "ws-1", "g/p", 7)
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.34
func TestMarkMRReadyNilReads(t *testing.T) {
	draft := &MR{State: mrStateOpen, Title: "Draft: x", Draft: true}
	write := func(string) error { return nil }
	if _, err := markMRReady(func() (*MR, error) { return nil, nil }, write); !errors.Is(err, ErrMRNilResult) {
		t.Errorf("nil first read = %v, want ErrMRNilResult", err)
	}
	reads := 0
	_, err := markMRReady(func() (*MR, error) {
		reads++
		if reads == 1 {
			return draft, nil
		}
		return nil, nil
	}, write)
	if !errors.Is(err, ErrMRReadAfterReady) {
		t.Errorf("nil re-read = %v, want ErrMRReadAfterReady", err)
	}
}
