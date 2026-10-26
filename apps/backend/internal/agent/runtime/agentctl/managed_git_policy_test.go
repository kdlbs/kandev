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
	"time"

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
		want     error
	}{
		{"capability absent", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"server_identity":"old","capabilities":["agent-survival.v1"]}`))
		}, ErrManagedGitPolicyUnsupported},
		{"identity route missing", func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }, ErrManagedGitPolicyUnverified},
		{"malformed identity", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{`)) }, ErrManagedGitPolicyUnverified},
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
			if !errors.Is(err, tc.want) || resp != nil {
				t.Fatalf("CreateInstance = %+v, %v, want %v", resp, err, tc.want)
			}
			if tc.want == ErrManagedGitPolicyUnverified && errors.Is(err, ErrManagedGitPolicyUnsupported) {
				t.Fatalf("a failed verification must not read as a missing capability: %v", err)
			}
			if errors.Is(err, ErrLaunchWrapperUnsupported) {
				t.Fatalf("managed Git policy error must not be a launch-wrapper error: %v", err)
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

func TestCreateInstanceReturnsCallerContextErrorFromCapabilityCheck(t *testing.T) {
	var created atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/identity" {
			created.Store(true)
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	t.Cleanup(cancelExpired)
	for name, tc := range map[string]struct {
		ctx  context.Context
		want error
	}{"cancelled": {cancelled, context.Canceled}, "past deadline": {expired, context.DeadlineExceeded}} {
		t.Run(name, func(t *testing.T) {
			_, err := newTestControlClient(t, srv).CreateInstance(tc.ctx, &CreateInstanceRequest{WorkspacePath: "/w"})
			if !errors.Is(err, tc.want) || errors.Is(err, ErrManagedGitPolicyUnverified) || errors.Is(err, ErrManagedGitPolicyUnsupported) {
				t.Fatalf("CreateInstance error = %v, want the caller's %v", err, tc.want)
			}
		})
	}
	if created.Load() {
		t.Fatal("instance creation was attempted after the caller context ended")
	}
}
