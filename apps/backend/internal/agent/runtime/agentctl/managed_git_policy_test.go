package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/subproc"
)

const managedGitIdentityBody = `{"server_identity":"test","capabilities":["` + subproc.ManagedGitPolicyCapability + `"]}`

// withManagedGitIdentity answers GET /identity with the managed Git policy
// capability and delegates every other request.
func withManagedGitIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/identity" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(managedGitIdentityBody))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func TestCreateInstanceRefusesPeerWithoutManagedGitPolicy(t *testing.T) {
	cases := []struct {
		name     string
		identity http.HandlerFunc
	}{
		{"capability absent", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"server_identity":"old","capabilities":["agent-survival.v1"]}`))
		}},
		{"identity route missing", func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }},
		{"malformed identity", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{`)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var created atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					tc.identity(w, r)
					return
				}
				created.Store(true)
				w.WriteHeader(http.StatusCreated)
			}))
			t.Cleanup(srv.Close)

			resp, err := newTestControlClient(t, srv).CreateInstance(context.Background(), &CreateInstanceRequest{WorkspacePath: "/w"})
			if !errors.Is(err, ErrManagedGitPolicyUnsupported) || resp != nil {
				t.Fatalf("CreateInstance = %+v, %v, want ErrManagedGitPolicyUnsupported", resp, err)
			}
			if created.Load() {
				t.Fatal("instance creation was attempted on a peer without the managed Git policy")
			}
		})
	}
}

func TestCreateInstanceAcceptsPeerWithManagedGitPolicy(t *testing.T) {
	srv := httptest.NewServer(withManagedGitIdentity(jsonResponder(http.StatusCreated, `{"id":"inst-1","port":41234}`)))
	t.Cleanup(srv.Close)

	resp, err := newTestControlClient(t, srv).CreateInstance(context.Background(), &CreateInstanceRequest{WorkspacePath: "/w"})
	if err != nil || resp == nil || resp.Port != 41234 {
		t.Fatalf("CreateInstance = %+v, %v", resp, err)
	}
}

func managedGitIdentityResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(managedGitIdentityBody)),
		Request:    req,
	}
}

func TestCreateInstanceWithIsolationOffSkipsCapabilityAndSendsNoMode(t *testing.T) {
	previous := subproc.ManagedGitIsolation()
	subproc.SetManagedGitIsolation(false)
	t.Cleanup(func() { subproc.SetManagedGitIsolation(previous) })

	var identityCalls atomic.Int32
	var body atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity" {
			identityCalls.Add(1)
			_, _ = w.Write([]byte(`{"server_identity":"old","capabilities":[]}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		body.Store(string(raw))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"inst-1","port":41234}`))
	}))
	t.Cleanup(srv.Close)

	resp, err := newTestControlClient(t, srv).CreateInstance(context.Background(), &CreateInstanceRequest{WorkspacePath: "/w"})
	if err != nil || resp == nil {
		t.Fatalf("CreateInstance = %+v, %v; an agentctl without the capability must be accepted with the mode off", resp, err)
	}
	if identityCalls.Load() != 0 {
		t.Fatal("the capability was checked with the mode off")
	}
	sent, _ := body.Load().(string)
	if strings.Contains(sent, "git_isolation") || strings.Contains(sent, "git_hooks") {
		t.Fatalf("create request carries isolation values with the mode off: %s", sent)
	}
}

func TestCreateInstanceWithIsolationOnSendsMode(t *testing.T) {
	var body atomic.Value
	srv := httptest.NewServer(withManagedGitIdentity(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body.Store(string(raw))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"inst-1","port":41234}`))
	})))
	t.Cleanup(srv.Close)

	if _, err := newTestControlClient(t, srv).CreateInstance(context.Background(), &CreateInstanceRequest{WorkspacePath: "/w"}); err != nil {
		t.Fatal(err)
	}
	if sent, _ := body.Load().(string); !strings.Contains(sent, `"git_isolation":true`) {
		t.Fatalf("create request = %s, want git_isolation true", sent)
	}
}
