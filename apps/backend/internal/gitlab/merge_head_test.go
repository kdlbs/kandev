package gitlab

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const mergeHeadSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// TestWorkspaceMergeMRRequiredSHA exercises the registered route and real
// workspace client against a disposable simulation of GitLab's merge contract.
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.24
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.27
func TestWorkspaceMergeMRRequiredSHA(t *testing.T) {
	for _, tc := range []struct {
		name       string
		requireSHA bool
	}{
		{name: "required", requireSHA: true},
		{name: "legacy", requireSHA: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var writes int
			var seenSHA string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("PRIVATE-TOKEN") != "workspace-token" {
					http.Error(w, "wrong token", http.StatusUnauthorized)
					return
				}
				switch r.Method + " " + r.URL.EscapedPath() {
				case "GET /api/v4/projects/group%2Fsub%2Ftarget":
					_, _ = w.Write([]byte(`{"merge_method":"merge","squash_option":"never"}`))
				case "GET /api/v4/projects/group%2Fsub%2Ftarget/merge_requests/17":
					_, _ = w.Write([]byte(`{"iid":17,"sha":"` + mergeHeadSHA + `","diff_refs":{"head_sha":"old-diff","base_sha":"base"},"source_project_id":11,"target_project_id":22}`))
				case "PUT /api/v4/projects/group%2Fsub%2Ftarget/merge_requests/17/merge":
					writes++
					var body struct {
						SHA string `json:"sha"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode: %v", err)
						w.WriteHeader(400)
						return
					}
					seenSHA = body.SHA
					if tc.requireSHA && body.SHA == "" {
						http.Error(w, "SHA required", 400)
						return
					}
					if body.SHA != "" && body.SHA != mergeHeadSHA {
						http.Error(w, "SHA mismatch", http.StatusConflict)
						return
					}
					_, _ = w.Write([]byte(`{"iid":17,"state":"merged"}`))
				default:
					http.Error(w, "unexpected request", 404)
				}
			}))
			t.Cleanup(server.Close)
			store := newTestStore(t)
			seedWorkspace(t, store, "workspace-test")
			if err := store.UpsertConfigForWorkspace(t.Context(), "workspace-test", &GitLabConfig{Host: server.URL, AuthMethod: AuthMethodPAT}); err != nil {
				t.Fatal(err)
			}
			svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
			svc.SetStore(store)
			svc.SetWorkspaceSecretStore(&configTestSecrets{values: map[string]string{SecretKeyForWorkspace("workspace-test"): "workspace-token"}})
			gin.SetMode(gin.TestMode)
			router := gin.New()
			NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)
			req := httptest.NewRequest(http.MethodPut, "/api/v1/gitlab/mrs/merge?workspace_id=workspace-test", strings.NewReader(`{"project":"group/sub/target","iid":17,"method":"merge"}`))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusOK {
				t.Errorf("merge route = %d, want 200 (provider SHA=%q; body=%s)", response.Code, seenSHA, response.Body.String())
			}
			if writes != 1 {
				t.Errorf("merge writes = %d, want 1", writes)
			}
			if tc.requireSHA && seenSHA != mergeHeadSHA {
				t.Errorf("provider SHA = %q, want source head %q", seenSHA, mergeHeadSHA)
			}
			if tc.requireSHA {
				control, err := http.NewRequest(http.MethodPut, server.URL+"/api/v4/projects/group%2Fsub%2Ftarget/merge_requests/17/merge", bytes.NewBufferString(fmt.Sprintf(`{"sha":%q}`, mergeHeadSHA)))
				if err != nil {
					t.Fatal(err)
				}
				control.Header.Set("PRIVATE-TOKEN", "workspace-token")
				resp, err := server.Client().Do(control)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = resp.Body.Close() }()
				if resp.StatusCode != http.StatusOK {
					t.Errorf("exact SHA control = %d, want 200", resp.StatusCode)
				}
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.25
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.26
func TestPATMergeHeadFailuresAndMismatch(t *testing.T) {
	for _, tc := range []struct {
		name        string
		readStatus  int
		response    string
		writeStatus int
		wantWrites  int
		wantStatus  int
	}{
		{name: "unavailable", readStatus: http.StatusServiceUnavailable, wantStatus: http.StatusServiceUnavailable},
		{name: "missing", response: `{}`},
		{name: "empty", response: `{"sha":""}`},
		{name: "blank", response: `{"sha":"  "}`},
		{name: "head moved", response: `{"sha":"` + mergeHeadSHA + `"}`, writeStatus: http.StatusConflict, wantWrites: 1, wantStatus: http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads, writes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					reads++
					if tc.readStatus != 0 {
						w.WriteHeader(tc.readStatus)
						return
					}
					_, _ = w.Write([]byte(tc.response))
				case http.MethodPut:
					writes++
					var body struct {
						SHA string `json:"sha"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.SHA != mergeHeadSHA {
						t.Errorf("SHA = %q", body.SHA)
					}
					w.WriteHeader(tc.writeStatus)
				default:
					t.Errorf("unexpected %s", r.Method)
				}
			}))
			t.Cleanup(server.Close)
			_, err := NewPATClient(server.URL, "token").MergeMR(t.Context(), "group/project", 17, false, "")
			if err == nil {
				t.Fatal("expected failure")
			}
			if tc.wantStatus != 0 {
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.wantStatus {
					t.Errorf("error = %v, want provider status %d", err, tc.wantStatus)
				}
			}
			if reads != 1 || writes != tc.wantWrites {
				t.Errorf("reads=%d writes=%d, want 1/%d", reads, writes, tc.wantWrites)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.24
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.27
func TestGLabAndAutomationMergeUseScopedHead(t *testing.T) {
	for _, tc := range []struct {
		name       string
		automation bool
	}{{name: "glab"}, {name: "automation", automation: true}} {
		t.Run(tc.name, func(t *testing.T) {
			reads, writes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("PRIVATE-TOKEN") != "chosen-token" {
					t.Errorf("wrong credential")
					w.WriteHeader(401)
					return
				}
				switch r.Method + " " + r.URL.EscapedPath() {
				case "GET /api/v4/projects/group%2Ftarget":
					_, _ = w.Write([]byte(`{"merge_method":"merge","squash_option":"always"}`))
				case "GET /api/v4/projects/group%2Ftarget/merge_requests/17":
					reads++
					_, _ = w.Write([]byte(`{"sha":"` + mergeHeadSHA + `","diff_refs":{"head_sha":"stale","base_sha":"base"},"source_project_id":11,"target_project_id":22}`))
				case "PUT /api/v4/projects/group%2Ftarget/merge_requests/17/merge":
					writes++
					var body struct {
						SHA     string `json:"sha"`
						Squash  bool   `json:"squash"`
						Message string `json:"squash_commit_message"`
						Remove  bool   `json:"should_remove_source_branch"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.SHA != mergeHeadSHA || !body.Squash || body.Remove || (!tc.automation && body.Message != "combined") {
						t.Errorf("merge body = %+v", body)
					}
					_, _ = w.Write([]byte(`{"iid":17,"state":"merged"}`))
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.EscapedPath())
					w.WriteHeader(404)
				}
			}))
			t.Cleanup(server.Close)
			if tc.automation {
				store := newTestStore(t)
				seedWorkspace(t, store, "selected")
				if err := store.UpsertConfigForWorkspace(t.Context(), "selected", &GitLabConfig{Host: server.URL, AuthMethod: AuthMethodPAT}); err != nil {
					t.Fatal(err)
				}
				svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
				svc.SetStore(store)
				svc.SetWorkspaceSecretStore(&configTestSecrets{values: map[string]string{SecretKeyForWorkspace("selected"): "chosen-token"}})
				if _, err := svc.MergeMRForAutomation(t.Context(), "selected", server.URL, "group/target", 17); err != nil {
					t.Fatal(err)
				}
			} else {
				client := &GLabClient{PATClient: NewPATClient(server.URL, "chosen-token")}
				if _, err := client.MergeMR(t.Context(), "group/target", 17, true, "combined"); err != nil {
					t.Fatal(err)
				}
			}
			if reads != 1 || writes != 1 {
				t.Errorf("reads=%d writes=%d, want 1/1", reads, writes)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.25
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.26
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.27
func TestWorkspaceMergeMRFailuresAndOptions(t *testing.T) {
	for _, tc := range []struct {
		name, head, method, expectedHost                     string
		readStatus, writeStatus, wantRouteStatus, wantWrites int
	}{
		{name: "squash options", head: mergeHeadSHA, method: "squash", wantRouteStatus: 200, wantWrites: 1},
		{name: "missing head", head: "", method: "merge", wantRouteStatus: 502},
		{name: "read fails", readStatus: 503, method: "merge", wantRouteStatus: 502},
		{name: "head moves", head: mergeHeadSHA, method: "merge", writeStatus: 409, wantRouteStatus: 502, wantWrites: 1},
		{name: "host mismatch", head: mergeHeadSHA, method: "merge", expectedHost: "https://other.example", wantRouteStatus: 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads, writes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("PRIVATE-TOKEN") != "workspace-token" {
					t.Error("wrong workspace credential")
					w.WriteHeader(401)
					return
				}
				switch r.Method + " " + r.URL.EscapedPath() {
				case "GET /api/v4/projects/group%2Ftarget":
					_, _ = w.Write([]byte(`{"merge_method":"merge","squash_option":"default_off"}`))
				case "GET /api/v4/projects/group%2Ftarget/merge_requests/17":
					reads++
					if tc.readStatus != 0 {
						w.WriteHeader(tc.readStatus)
						return
					}
					_, _ = fmt.Fprintf(w, `{"sha":%q}`, tc.head)
				case "PUT /api/v4/projects/group%2Ftarget/merge_requests/17/merge":
					writes++
					var body struct {
						SHA     string `json:"sha"`
						Squash  bool   `json:"squash"`
						Message string `json:"squash_commit_message"`
						Remove  bool   `json:"should_remove_source_branch"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.SHA != mergeHeadSHA || body.Remove || (tc.method == "squash" && (!body.Squash || body.Message != "combined")) {
						t.Errorf("body = %+v", body)
					}
					if tc.writeStatus != 0 {
						w.WriteHeader(tc.writeStatus)
						return
					}
					_, _ = w.Write([]byte(`{"iid":17,"state":"merged"}`))
				default:
					t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.EscapedPath())
					w.WriteHeader(404)
				}
			}))
			t.Cleanup(server.Close)
			store := newTestStore(t)
			seedWorkspace(t, store, "workspace-test")
			if err := store.UpsertConfigForWorkspace(t.Context(), "workspace-test", &GitLabConfig{Host: server.URL, AuthMethod: AuthMethodPAT}); err != nil {
				t.Fatal(err)
			}
			svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
			svc.SetStore(store)
			svc.SetWorkspaceSecretStore(&configTestSecrets{values: map[string]string{SecretKeyForWorkspace("workspace-test"): "workspace-token"}})
			router := gin.New()
			NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)
			target := "/api/v1/gitlab/mrs/merge?workspace_id=workspace-test"
			if tc.expectedHost != "" {
				target += "&expected_host=" + url.QueryEscape(tc.expectedHost)
			}
			body := fmt.Sprintf(`{"project":"group/target","iid":17,"method":%q,"squash_commit_message":"combined"}`, tc.method)
			req := httptest.NewRequest(http.MethodPut, target, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.wantRouteStatus {
				t.Errorf("route status = %d, want %d: %s", response.Code, tc.wantRouteStatus, response.Body.String())
			}
			if writes != tc.wantWrites {
				t.Errorf("writes = %d, want %d", writes, tc.wantWrites)
			}
			if tc.expectedHost == "" && reads != 1 {
				t.Errorf("reads = %d, want 1", reads)
			}
			if tc.expectedHost != "" && reads != 0 {
				t.Errorf("host mismatch made %d reads", reads)
			}
		})
	}
}
