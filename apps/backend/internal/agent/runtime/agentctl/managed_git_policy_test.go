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
