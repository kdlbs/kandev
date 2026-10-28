package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/sessionmcp"
	"github.com/kandev/kandev/internal/secrets"
)

// keyedStore is a SecretStore that also holds the signing key.
type keyedStore struct {
	secrets.SecretStore
	key      []byte
	err      error
	replaced bool
}

func (k *keyedStore) SessionMCPKey(context.Context) ([]byte, error) { return k.key, k.err }
func (k *keyedStore) EnsureSessionMCPKey(context.Context) ([]byte, bool, error) {
	return k.key, k.replaced, k.err
}

type createBackend struct {
	ExecutorBackend
	creates  []string // nonce of each attempt
	failWith []error  // consumed per attempt
	stopped  []string // fingerprint carried by each stop
	srv      *httptest.Server
	handler  atomic.Pointer[http.HandlerFunc]
	t        *testing.T
}

func (b *createBackend) CreateInstance(_ context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	b.creates = append(b.creates, req.SessionMCPNonce)
	if len(b.failWith) > 0 {
		err := b.failWith[0]
		b.failWith = b.failWith[1:]
		if err != nil {
			return nil, fmt.Errorf("create: %w", err)
		}
	}
	host, portStr, _ := net.SplitHostPort(b.srv.Listener.Addr().String())
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)
	return &ExecutorInstance{InstanceID: req.InstanceID, Client: agentctl.NewClient(host, port, newTestLogger())}, nil
}

func (b *createBackend) StopInstance(ctx context.Context, _ *ExecutorInstance, _ bool) error {
	b.stopped = append(b.stopped, agentctl.ExpectedTokenFingerprintFrom(ctx))
	return nil
}

func newCreateBackend(t *testing.T, identity http.HandlerFunc) *createBackend {
	b := &createBackend{t: t}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := b.handler.Load(); h != nil {
			(*h)(w, r)
		}
	}))
	if identity != nil {
		b.setHandler(identity)
	}
	t.Cleanup(b.srv.Close)
	return b
}

func (b *createBackend) setHandler(h http.HandlerFunc) { b.handler.Store(&h) }

func newLaunchManager(store secrets.SecretStore) *Manager {
	return &Manager{logger: newTestLogger(), secretStore: store, sessionMCPPolicy: NewSessionMCPPolicy(true)}
}

func servesFingerprint(caps []string, fp func(*http.Request) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"capabilities":[%s],"mcp_token_fingerprint":%q}`, quoteAll(caps), fp(r))
	}
}

func quoteAll(in []string) string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(out, ",")
}

func launchKey() []byte { return []byte(strings.Repeat("k", sessionmcp.KeyLength)) }

func TestCreateInstanceWithSessionMCPMintsFreshTokenAndPersistsNonce(t *testing.T) {
	b := newCreateBackend(t, nil)
	m := newLaunchManager(&keyedStore{key: launchKey()})
	req := &ExecutorCreateRequest{InstanceID: "exec-1"}
	// The identity handler reports the fingerprint of whatever the request minted.
	b.setHandler(servesFingerprint([]string{"session_mcp_token"}, func(*http.Request) string {
		return sessionmcp.Fingerprint(req.SessionMCPToken)
	}))
	inst, err := m.createInstanceWithSessionMCP(context.Background(), b, req)
	if err != nil {
		t.Fatal(err)
	}
	if req.SessionMCPToken == "" || req.SessionMCPToken != sessionmcp.Token(launchKey(), "exec-1", req.SessionMCPNonce) {
		t.Fatalf("token not derived from key, instance and nonce")
	}
	if inst.Metadata[MetadataKeyMCPTokenNonce] != req.SessionMCPNonce || req.Metadata[MetadataKeyMCPTokenNonce] != req.SessionMCPNonce {
		t.Fatalf("nonce not persisted in metadata: %v / %v", inst.Metadata, req.Metadata)
	}
}

func TestCreateInstanceWithSessionMCPRetriesConflictWithFreshNonce(t *testing.T) {
	b := newCreateBackend(t, nil)
	req := &ExecutorCreateRequest{InstanceID: "exec-1"}
	b.setHandler(servesFingerprint([]string{"session_mcp_token"}, func(*http.Request) string {
		return sessionmcp.Fingerprint(req.SessionMCPToken)
	}))
	b.failWith = []error{&agentctl.CreateConflictError{Code: "instance_cleaning_up"}, nil}
	m := newLaunchManager(&keyedStore{key: launchKey()})
	if _, err := m.createInstanceWithSessionMCP(context.Background(), b, req); err != nil {
		t.Fatal(err)
	}
	if len(b.creates) != 2 || b.creates[0] == b.creates[1] {
		t.Fatalf("attempt nonces = %v, want two distinct", b.creates)
	}
}

func TestCreateInstanceWithSessionMCPCapabilityAbsentRollsBackWithMintedFingerprint(t *testing.T) {
	b := newCreateBackend(t, nil)
	req := &ExecutorCreateRequest{InstanceID: "exec-1"}
	b.setHandler(servesFingerprint(nil, func(*http.Request) string { return "" }))
	m := newLaunchManager(&keyedStore{key: launchKey()})
	_, err := m.createInstanceWithSessionMCP(context.Background(), b, req)
	var refusal *LaunchRefusal
	if !errors.As(err, &refusal) || refusal.Code != CodeMCPTokenUnsupported || !strings.HasPrefix(err.Error(), CodeMCPTokenUnsupported+":") {
		t.Fatalf("err = %v, want %s refusal", err, CodeMCPTokenUnsupported)
	}
	if len(b.stopped) != 1 || b.stopped[0] != sessionmcp.Fingerprint(req.SessionMCPToken) {
		t.Fatalf("stopped with %q, want minted fingerprint", b.stopped)
	}
}

func TestCreateInstanceWithSessionMCPDifferentFingerprintDeletesObservedAndRetries(t *testing.T) {
	b := newCreateBackend(t, nil)
	req := &ExecutorCreateRequest{InstanceID: "exec-1"}
	calls := 0
	b.setHandler(servesFingerprint([]string{"session_mcp_token"}, func(*http.Request) string {
		calls++
		if calls == 1 {
			return "someone-elses"
		}
		return sessionmcp.Fingerprint(req.SessionMCPToken)
	}))
	m := newLaunchManager(&keyedStore{key: launchKey()})
	if _, err := m.createInstanceWithSessionMCP(context.Background(), b, req); err != nil {
		t.Fatal(err)
	}
	if len(b.stopped) != 1 || b.stopped[0] != "someone-elses" || len(b.creates) != 2 {
		t.Fatalf("stopped=%q creates=%d", b.stopped, len(b.creates))
	}
}

func TestCreateInstanceWithSessionMCPKeyStoreFailureRefusesLaunch(t *testing.T) {
	b := newCreateBackend(t, nil)
	m := newLaunchManager(&keyedStore{err: errors.New("disk full")})
	_, err := m.createInstanceWithSessionMCP(context.Background(), b, &ExecutorCreateRequest{InstanceID: "exec-1"})
	if !strings.HasPrefix(fmt.Sprint(err), CodeMCPTokenKeyUnavailable+":") || len(b.creates) != 0 {
		t.Fatalf("err = %v creates=%d", err, len(b.creates))
	}
}

func TestLaunchRefusalTextStartsWithCodeAndCarriesCause(t *testing.T) {
	text := (&LaunchRefusal{Code: CodeMCPTokenCarrierUnavailable, Cause: CauseAgentNotVerified}).Error()
	if !strings.HasPrefix(text, CodeMCPTokenCarrierUnavailable+": ") || !strings.HasSuffix(text, " cause="+CauseAgentNotVerified) {
		t.Fatalf("text = %q", text)
	}
}

// reusedBackend returns an instance a runtime reconnected to rather than
// created, so a refusal must leave it running.
type reusedBackend struct {
	*createBackend
	runtime agentruntime.Runtime
}

func (b *reusedBackend) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	inst, err := b.createBackend.CreateInstance(ctx, req)
	if err != nil {
		return nil, err
	}
	inst.RuntimeName = b.runtime
	inst.Metadata = map[string]interface{}{MetadataKeyReuseExistingProcess: true}
	return inst, nil
}

func TestCreateInstanceWithSessionMCPRefusesReusedDeferredRuntimeWithoutStopping(t *testing.T) {
	for _, rtName := range []agentruntime.Runtime{agentruntime.RuntimeSSH, agentruntime.RuntimeKubernetes} {
		b := &reusedBackend{createBackend: newCreateBackend(t, nil), runtime: rtName}
		req := &ExecutorCreateRequest{InstanceID: "exec-1"}
		b.setHandler(servesFingerprint([]string{"session_mcp_token"}, func(*http.Request) string { return "retained-token" }))
		m := newLaunchManager(&keyedStore{key: launchKey()})
		_, err := m.createInstanceWithSessionMCP(context.Background(), b, req)
		var refusal *LaunchRefusal
		if !errors.As(err, &refusal) || refusal.Code != CodeMCPTokenUnsupported || refusal.Cause != CauseRetainedInstanceDeferred {
			t.Fatalf("%s: err = %v, want %s cause %s", rtName, err, CodeMCPTokenUnsupported, CauseRetainedInstanceDeferred)
		}
		if len(b.stopped) != 0 {
			t.Fatalf("%s: reused instance was stopped: %q", rtName, b.stopped)
		}
	}
}

func TestCreateInstanceWithSessionMCPReusedInstanceWithoutCapabilityIsNotStopped(t *testing.T) {
	b := &reusedBackend{createBackend: newCreateBackend(t, nil), runtime: agentruntime.RuntimeDocker}
	req := &ExecutorCreateRequest{InstanceID: "exec-1"}
	b.setHandler(servesFingerprint(nil, func(*http.Request) string { return "" }))
	m := newLaunchManager(&keyedStore{key: launchKey()})
	_, err := m.createInstanceWithSessionMCP(context.Background(), b, req)
	var refusal *LaunchRefusal
	if !errors.As(err, &refusal) || refusal.Code != CodeMCPTokenUnsupported {
		t.Fatalf("err = %v, want %s refusal", err, CodeMCPTokenUnsupported)
	}
	if len(b.stopped) != 0 {
		t.Fatalf("reused instance was stopped: %q", b.stopped)
	}
}

func TestCreateInstanceWithSessionMCPRetriesTransientIdentityRead(t *testing.T) {
	prev := createIdentityGap
	createIdentityGap = time.Millisecond
	t.Cleanup(func() { createIdentityGap = prev })
	b := newCreateBackend(t, nil)
	req := &ExecutorCreateRequest{InstanceID: "exec-1"}
	var reads atomic.Int32
	b.setHandler(func(w http.ResponseWriter, r *http.Request) {
		if reads.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		servesFingerprint([]string{"session_mcp_token"}, func(*http.Request) string {
			return sessionmcp.Fingerprint(req.SessionMCPToken)
		})(w, r)
	})
	m := newLaunchManager(&keyedStore{key: launchKey()})
	if _, err := m.createInstanceWithSessionMCP(context.Background(), b, req); err != nil {
		t.Fatalf("transient identity failure refused the launch: %v", err)
	}
	if len(b.stopped) != 0 {
		t.Fatalf("instance stopped after a transient read failure: %q", b.stopped)
	}
}
