package gitlab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

type squashChoiceCase struct {
	name, policy, method        string
	selected, automation        bool
	wantSquash, wantOverride    bool
	failWrite, badHost, invalid bool
}

type squashChoiceProvider struct {
	mu            sync.Mutex
	t             *testing.T
	tc            squashChoiceCase
	reads, writes int
	body          map[string]any
	effective     bool
}

func (p *squashChoiceProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.Header.Get("PRIVATE-TOKEN") != "selected-token" {
		p.t.Error("wrong workspace credential")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.Method + " " + r.URL.EscapedPath() {
	case "GET /api/v4/projects/group%2Fsub%2Ftarget":
		_, _ = fmt.Fprintf(w, `{"merge_method":"merge","squash_option":%q}`, p.tc.policy)
	case "GET /api/v4/projects/group%2Fsub%2Ftarget/merge_requests/17":
		p.reads++
		_, _ = fmt.Fprintf(w, `{"sha":%q,"diff_refs":{"head_sha":"stale"}}`, mergeHeadSHA)
	case "PUT /api/v4/projects/group%2Fsub%2Ftarget/merge_requests/17/merge":
		p.writes++
		if err := json.NewDecoder(r.Body).Decode(&p.body); err != nil {
			p.t.Errorf("decode merge body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		p.effective = p.tc.selected
		if squash, ok := p.body["squash"].(bool); ok && squash {
			p.effective = true
		}
		switch p.tc.policy {
		case "always":
			p.effective = true
		case "never":
			p.effective = false
		}
		if p.tc.failWrite {
			w.WriteHeader(http.StatusConflict)
			return
		}
		_, _ = w.Write([]byte(`{"iid":17,"state":"merged"}`))
	default:
		p.t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.EscapedPath())
		w.WriteHeader(http.StatusNotFound)
	}
}

func squashChoiceService(t *testing.T, host string) *Service {
	t.Helper()
	store := newTestStore(t)
	seedWorkspace(t, store, "selected")
	if err := store.UpsertConfigForWorkspace(t.Context(), "selected", &GitLabConfig{Host: host, AuthMethod: AuthMethodPAT}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	svc.SetStore(store)
	svc.SetWorkspaceSecretStore(&configTestSecrets{values: map[string]string{SecretKeyForWorkspace("selected"): "selected-token"}})
	return svc
}

func runSquashChoice(t *testing.T, tc squashChoiceCase) {
	t.Helper()
	provider := &squashChoiceProvider{t: t, tc: tc}
	server := httptest.NewServer(provider)
	t.Cleanup(server.Close)
	svc := squashChoiceService(t, server.URL)
	var failed bool
	if tc.automation {
		_, err := svc.MergeMRForAutomation(t.Context(), "selected", server.URL, "group/sub/target", 17)
		failed = err != nil
	} else {
		router := gin.New()
		NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)
		host := server.URL
		if tc.badHost {
			host = "https://other.example"
		}
		target := "/api/v1/gitlab/mrs/merge?workspace_id=selected&expected_host=" + url.QueryEscape(host)
		body := fmt.Sprintf(`{"project":"group/sub/target","iid":17,"method":%q,"squash_commit_message":"combined"}`, tc.method)
		req := httptest.NewRequest(http.MethodPut, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		failed = response.Code != http.StatusOK
	}
	if failed != (tc.invalid || tc.badHost || tc.failWrite) {
		t.Errorf("failed = %v", failed)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if tc.invalid || tc.badHost {
		if provider.reads != 0 || provider.writes != 0 {
			t.Errorf("head reads = %d, writes = %d; want no merge attempt", provider.reads, provider.writes)
		}
		return
	}
	if provider.reads != 1 || provider.writes != 1 {
		t.Errorf("head reads = %d, writes = %d; want 1/1", provider.reads, provider.writes)
	}
	if provider.body["sha"] != mergeHeadSHA {
		t.Errorf("source SHA = %v", provider.body["sha"])
	}
	if _, ok := provider.body["should_remove_source_branch"]; ok {
		t.Error("unexpected source-branch removal request")
	}
	if !tc.automation && provider.body["squash_commit_message"] != "combined" {
		t.Errorf("squash message = %v", provider.body["squash_commit_message"])
	}
	if tc.wantOverride {
		if provider.body["squash"] != true {
			t.Errorf("explicit squash field = %v, want true", provider.body["squash"])
		}
	} else if !tc.wantSquash {
		if value, ok := provider.body["squash"]; ok {
			t.Errorf("default squash field = %v, want omitted", value)
		}
	}
	if provider.effective != tc.wantSquash {
		t.Errorf("modeled GitLab squash = %v, want %v", provider.effective, tc.wantSquash)
	}
}

// TestGitLabSquashChoice exercises the registered workspace route and automation
// with a real PAT client against a disposable model of GitLab merge semantics.
func TestGitLabSquashChoice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []squashChoiceCase{
		{name: "default_off_unchecked", policy: "default_off"},
		{name: "default_on_unchecked_automation", policy: "default_on", automation: true},
		{name: "default_on_selected", policy: "default_on", selected: true, wantSquash: true},
		{name: "required", policy: "always", automation: true, wantSquash: true},
		{name: "never", policy: "never"},
		{name: "explicit_squash", policy: "default_off", method: "squash", wantSquash: true, wantOverride: true},
		{name: "disallowed_squash", policy: "never", method: "squash", invalid: true},
		{name: "unknown_method", policy: "default_off", method: "unknown", invalid: true},
		{name: "disallowed_merge_method", policy: "default_off", method: "ff", invalid: true},
		{name: "host_mismatch", policy: "default_off", badHost: true},
		{name: "head_mismatch", policy: "never", failWrite: true},
	} {
		t.Run(tc.name, func(t *testing.T) { runSquashChoice(t, tc) })
	}
}
