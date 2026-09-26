package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

func TestReviewChangeSourceExecutionFailures(t *testing.T) {
	tests := []struct {
		name   string
		source *ReviewChangeSource
		want   string
	}{
		{name: "lookup missing", source: NewReviewChangeSource(nil, nil), want: "no execution lookup configured"},
		{name: "lookup error", source: NewReviewChangeSource(&mockExecutionLookup{ensureErr: errors.New("database unavailable")}, nil), want: "execution for session s: database unavailable"},
		{name: "execution missing client", source: NewReviewChangeSource(&mockExecutionLookup{executions: map[string]*lifecycle.AgentExecution{"s": {}}}, nil), want: "session s workspace is not ready"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.source.UncommittedFiles(context.Background(), "s")
			if err == nil || err.Error() != tt.want {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestReviewChangeSourceUncommittedFiles(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       map[string]any
		wantErr    string
	}{
		{name: "files", statusCode: http.StatusOK, body: `{"files":{"main.go":{"status":"modified"}}}`, want: map[string]any{"main.go": map[string]any{"status": "modified"}}},
		{name: "nil status", statusCode: http.StatusOK, body: `null`},
		{name: "dependency error", statusCode: http.StatusBadGateway, body: `broken`, wantErr: "git status for session s:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := reviewSourceServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/git/status" {
					t.Errorf("path = %s", r.URL.Path)
				}
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			})
			got, err := source.UncommittedFiles(context.Background(), "s")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UncommittedFiles: %v", err)
			}
			if !deepEqualJSON(got, tt.want) {
				t.Fatalf("files = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestReviewChangeSourceCommittedFilesUsesSessionBase(t *testing.T) {
	var query url.Values
	source := reviewSourceServer(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_, _ = w.Write([]byte(`{"files":{"a.go":{"additions":3}}}`))
	})
	source.sessionReader = &mockSessionReader{baseCommits: map[string]string{"s": "abc123"}, baseBranches: map[string]string{"s": "main"}}

	files, err := source.CommittedFiles(context.Background(), "s")
	if err != nil {
		t.Fatalf("CommittedFiles: %v", err)
	}
	if query.Get("base") != "abc123" || query.Get("target_branch") != "main" {
		t.Fatalf("query = %v", query)
	}
	if !deepEqualJSON(files, map[string]any{"a.go": map[string]any{"additions": float64(3)}}) {
		t.Fatalf("files = %#v", files)
	}
}

func TestReviewChangeSourceCommittedFilesFallsBackToStatusBase(t *testing.T) {
	requests := 0
	source := reviewSourceServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/v1/git/status":
			_, _ = w.Write([]byte(`{"base_commit":"fallback"}`))
		case "/api/v1/git/cumulative-diff":
			if r.URL.Query().Get("base") != "fallback" {
				t.Errorf("base = %q", r.URL.Query().Get("base"))
			}
			_, _ = w.Write([]byte(`null`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	files, err := source.CommittedFiles(context.Background(), "s")
	if err != nil || files != nil || requests != 2 {
		t.Fatalf("CommittedFiles = (%#v, %v), requests=%d", files, err, requests)
	}
}

func TestReviewChangeSourceCommittedFilesReturnsEmptyWithoutBase(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `broken`} {
		t.Run(body, func(t *testing.T) {
			source := reviewSourceServer(t, func(w http.ResponseWriter, _ *http.Request) {
				if body == "broken" {
					w.WriteHeader(http.StatusInternalServerError)
				}
				_, _ = w.Write([]byte(body))
			})
			files, err := source.CommittedFiles(context.Background(), "s")
			if err != nil || files != nil {
				t.Fatalf("CommittedFiles = (%#v, %v)", files, err)
			}
		})
	}
}

func TestReviewChangeSourceCommittedFilesWrapsDiffFailure(t *testing.T) {
	source := reviewSourceServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("broken"))
	})
	source.sessionReader = &mockSessionReader{baseCommits: map[string]string{"s": "base"}}
	_, err := source.CommittedFiles(context.Background(), "s")
	if err == nil || !strings.Contains(err.Error(), "cumulative diff for session s:") {
		t.Fatalf("error = %v", err)
	}
}

func reviewSourceServer(t *testing.T, handler http.HandlerFunc) *ReviewChangeSource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	client := agentctl.NewClient(u.Hostname(), port, newTestLogger())
	execution := &lifecycle.AgentExecution{SessionID: "s"}
	execution.SetAgentCtlClientForTesting(client)
	lookup := &mockExecutionLookup{executions: map[string]*lifecycle.AgentExecution{"s": execution}}
	return NewReviewChangeSource(lookup, nil)
}

func deepEqualJSON(got, want any) bool {
	return reflect.DeepEqual(got, want)
}
