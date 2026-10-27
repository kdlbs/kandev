package lifecycle

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/sessionmcp"
	"github.com/kandev/kandev/internal/task/models"
)

// tokenAdoptionServer plays both the control server and every instance
// listener; instances are told apart by the X-Instance-ID header.
type tokenAdoptionServer struct {
	mu       sync.Mutex
	identity map[string]func(w http.ResponseWriter)
	deletes  map[string]string // instance id -> precondition header
	revokes  map[string]int
}

func (s *tokenAdoptionServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.URL.Path == "/identity":
		s.identity[r.Header.Get("X-Instance-ID")](w)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/instances/"):
		s.deletes[strings.TrimPrefix(r.URL.Path, "/api/v1/instances/")] = r.Header.Get("X-Expected-Token-Fingerprint")
		w.WriteHeader(http.StatusOK)
	case strings.HasSuffix(r.URL.Path, "/mcp-token/revoke"):
		s.revokes[strings.Split(r.URL.Path, "/")[4]]++
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func identityBody(fp string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		_, _ = fmt.Fprintf(w, `{"capabilities":["session_mcp_token"],"mcp_token_fingerprint":%q}`, fp)
	}
}

func TestVerifyWinnerTokensAdoptsMatchDeletesMismatchRevokesUnknown(t *testing.T) {
	key := []byte(strings.Repeat("k", sessionmcp.KeyLength))
	good := sessionmcp.Fingerprint(sessionmcp.Token(key, "inst-good", "n-good"))
	srv := &tokenAdoptionServer{
		identity: map[string]func(http.ResponseWriter){
			"inst-good": identityBody(good),
			"inst-bad":  identityBody("stale-fp"),
			"inst-unk":  func(w http.ResponseWriter) { w.WriteHeader(http.StatusForbidden) },
		},
		deletes: map[string]string{},
		revokes: map[string]int{},
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	host, portStr, _ := net.SplitHostPort(ts.Listener.Addr().String())
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	control := agentctl.NewControlClient(host, port, newTestLogger())
	exec := NewStandaloneExecutor(control, host, port, newTestLogger())
	exec.SetSessionKeyReader(func() ([]byte, error) { return key, nil })

	winners := map[string]*agentctl.InstanceInfo{
		"s-good": {ID: "inst-good", Port: port},
		"s-bad":  {ID: "inst-bad", Port: port},
		"s-unk":  {ID: "inst-unk", Port: port},
	}
	records := map[string]*models.ExecutorRunning{
		"s-good": {SessionID: "s-good", Metadata: map[string]interface{}{MetadataKeyMCPTokenNonce: "n-good"}},
		"s-bad":  {SessionID: "s-bad", Metadata: map[string]interface{}{MetadataKeyMCPTokenNonce: "n-bad"}},
		"s-unk":  {SessionID: "s-unk", Metadata: map[string]interface{}{MetadataKeyMCPTokenNonce: "n-unk"}},
	}
	outcomes := exec.verifyWinnerTokens(context.Background(), control, nil, winners, records)

	if _, ok := winners["s-good"]; !ok || len(winners) != 1 {
		t.Fatalf("winners = %v, want only s-good", winners)
	}
	if outcomes["s-bad"] != RecoveryOutcomeNoMatchingInstance || outcomes["s-unk"] != RecoveryOutcomeUnknown {
		t.Fatalf("outcomes = %v", outcomes)
	}
	if srv.deletes["inst-bad"] != "stale-fp" || len(srv.deletes) != 1 {
		t.Fatalf("deletes = %v, want only inst-bad with observed fingerprint", srv.deletes)
	}
	if srv.revokes["inst-unk"] != 1 || len(srv.revokes) != 1 {
		t.Fatalf("revokes = %v", srv.revokes)
	}
}

func TestVerifyWinnerTokensSkippedWithoutKeyReader(t *testing.T) {
	exec := NewStandaloneExecutor(nil, "127.0.0.1", 1, newTestLogger())
	winners := map[string]*agentctl.InstanceInfo{"s": {ID: "i", Port: 1}}
	if out := exec.verifyWinnerTokens(context.Background(), nil, nil, winners, nil); out != nil || len(winners) != 1 {
		t.Fatalf("out=%v winners=%v", out, winners)
	}
}
