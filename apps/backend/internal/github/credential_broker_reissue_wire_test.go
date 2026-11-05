package github

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/githubauth"
)

// brokerReissueRoutes pairs each reissue route with the resolve route that
// redeems its leases. The GitHub-specific and provider-neutral routes share a
// handler and must accept the same wire contract.
var brokerReissueRoutes = []struct {
	reissue string
	resolve string
	code    string
}{
	{
		reissue: "/api/v1/github/credentials/reissue",
		resolve: "/api/v1/github/credentials/resolve",
		code:    "github_credential_reissue_denied",
	},
	{
		reissue: "/api/v1/git/credentials/reissue",
		resolve: "/api/v1/git/credentials/resolve",
		code:    "git_credential_reissue_denied",
	},
}

// newReissueCapabilityBroker builds the PAT broker with reissue enabled. Two
// brokers created with the same key model the same backend before and after a
// restart: the signing key survives, the in-memory lease map does not.
func newReissueCapabilityBroker(t *testing.T, signingKey string) (*CredentialBroker, *WorkspaceConnection) {
	t.Helper()
	broker, connection, _ := newPATCredentialBroker(t)
	signer, err := gitcredentials.NewReissueCapabilitySigner(signingKey)
	if err != nil {
		t.Fatalf("NewReissueCapabilitySigner() error = %v", err)
	}
	broker.broker.SetReissueCapabilitySigner(signer)
	return broker, connection
}

func issueBrokerLeaseWithCapability(
	t *testing.T,
	broker *CredentialBroker,
) (gitcredentials.Lease, gitcredentials.ReissueCapability) {
	t.Helper()
	lease, capability, err := broker.broker.IssueWithReissueCapability(t.Context(), gitcredentials.Scope{
		ProviderID: "github", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Host: "github.com", Path: "/kdlbs/kandev.git",
	})
	if err != nil {
		t.Fatalf("IssueWithReissueCapability() error = %v", err)
	}
	return lease, capability
}

func postBrokerBody(t *testing.T, router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal %s body: %v", path, err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeBrokerLeaseToken(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("reissue status = %d body=%s, want %d", rec.Code, rec.Body.String(), http.StatusOK)
	}
	var lease struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &lease); err != nil {
		t.Fatalf("decode reissue lease: %v", err)
	}
	if lease.Token == "" {
		t.Fatalf("reissue returned an empty lease: %s", rec.Body.String())
	}
	return lease.Token
}

func brokerResolveRequest(lease string) githubauth.CredentialBrokerRequest {
	return githubauth.CredentialBrokerRequest{
		Lease: lease, TaskID: "task-1", SessionID: "session-1", RepositoryID: "repository-1",
		Owner: "kdlbs", Repo: "kandev", Host: "github.com", Path: "/kdlbs/kandev.git",
	}
}

func brokerReissueRequest(capability string) githubauth.CredentialBrokerRequest {
	return githubauth.CredentialBrokerRequest{
		ReissueCapability: capability, TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Owner: "kdlbs", Repo: "kandev", Host: "github.com",
		Path: "/kdlbs/kandev.git",
	}
}

func requireBrokerCredential(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve status = %d body=%s, want %d", rec.Code, rec.Body.String(), http.StatusOK)
	}
	var credential struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &credential); err != nil {
		t.Fatalf("decode credential: %v", err)
	}
	if credential.Username == "" || credential.Password == "" {
		t.Fatalf("expected a populated credential, got %s", rec.Body.String())
	}
}

func requireBrokerLeaseFailure(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), http.StatusUnauthorized)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(wantCode)) {
		t.Fatalf("body = %s, want %s", rec.Body.String(), wantCode)
	}
}

// TestCredentialBrokerReissueAcceptsHelperWireField is the regression guard for
// the helper/handler field mismatch: a request built from the shared type and
// the equivalent literal JSON body, including the path the helper always sends,
// must both reissue.
func TestCredentialBrokerReissueAcceptsHelperWireField(t *testing.T) {
	forms := []struct {
		name string
		body func(capability string) any
	}{
		{
			name: "shared helper request type",
			body: func(capability string) any {
				return brokerReissueRequest(capability)
			},
		},
		{
			name: "literal reissue_capability body",
			body: func(capability string) any {
				return map[string]string{
					"reissue_capability": capability,
					"task_id":            "task-1",
					"session_id":         "session-1",
					"repository_id":      "repository-1",
					"owner":              "kdlbs",
					"repo":               "kandev",
					"host":               "github.com",
					"path":               "/kdlbs/kandev.git",
				}
			},
		},
	}
	for _, route := range brokerReissueRoutes {
		for _, form := range forms {
			t.Run(route.reissue+" "+form.name, func(t *testing.T) {
				broker, _ := newReissueCapabilityBroker(t, "test-signing-key")
				_, capability := issueBrokerLeaseWithCapability(t, broker)
				router := newBrokerHTTPTestRouter(t, broker)

				lease := decodeBrokerLeaseToken(t, postBrokerBody(t, router, route.reissue, form.body(capability.Token)))
				requireBrokerCredential(t, postBrokerBody(t, router, route.resolve, brokerResolveRequest(lease)))
			})
		}
	}
}

// TestCredentialBrokerReissueRejectsLegacyCapabilityField pins that the route
// reads only the helper's field. No production caller sends "capability", and
// accepting it would add a permanent second spelling nobody uses.
func TestCredentialBrokerReissueRejectsLegacyCapabilityField(t *testing.T) {
	for _, route := range brokerReissueRoutes {
		t.Run(route.reissue, func(t *testing.T) {
			broker, _ := newReissueCapabilityBroker(t, "test-signing-key")
			_, capability := issueBrokerLeaseWithCapability(t, broker)
			router := newBrokerHTTPTestRouter(t, broker)

			rec := postBrokerBody(t, router, route.reissue, map[string]string{
				"capability": capability.Token, "task_id": "task-1", "session_id": "session-1",
				"repository_id": "repository-1", "owner": "kdlbs", "repo": "kandev", "host": "github.com",
			})
			requireBrokerLeaseFailure(t, rec, route.code)
		})
	}
}

// TestCredentialBrokerReissueRecoversRotatedLeaseOverHTTP covers
// AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.7 through the real router: the
// old lease is revoked by the rotation, one reissue binds a lease to the new
// generation, and the same Git request then succeeds.
func TestCredentialBrokerReissueRecoversRotatedLeaseOverHTTP(t *testing.T) {
	broker, connection := newReissueCapabilityBroker(t, "test-signing-key")
	lease, capability := issueBrokerLeaseWithCapability(t, broker)
	router := newBrokerHTTPTestRouter(t, broker)

	requireBrokerCredential(t, postBrokerBody(t, router, "/api/v1/github/credentials/resolve", brokerResolveRequest(lease.Token)))
	connection.CredentialGeneration++
	requireBrokerLeaseFailure(
		t,
		postBrokerBody(t, router, "/api/v1/github/credentials/resolve", brokerResolveRequest(lease.Token)),
		"github_credential_lease_revoked",
	)

	reissued := decodeBrokerLeaseToken(t, postBrokerBody(t, router, "/api/v1/github/credentials/reissue", brokerReissueRequest(capability.Token)))
	requireBrokerCredential(t, postBrokerBody(t, router, "/api/v1/github/credentials/resolve", brokerResolveRequest(reissued)))
}

// TestCredentialBrokerReissueRecoversAfterRestartOverHTTP covers
// AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.8: the capability is sealed with
// the stable signing key, so a broker that never saw the original lease still
// reissues one and resolves it.
func TestCredentialBrokerReissueRecoversAfterRestartOverHTTP(t *testing.T) {
	const signingKey = "stable-restart-signing-key"
	beforeRestart, _ := newReissueCapabilityBroker(t, signingKey)
	lease, capability := issueBrokerLeaseWithCapability(t, beforeRestart)

	afterRestart, _ := newReissueCapabilityBroker(t, signingKey)
	if afterRestart.ActiveLeaseCount() != 0 {
		t.Fatalf("restarted broker active leases = %d, want 0", afterRestart.ActiveLeaseCount())
	}
	router := newBrokerHTTPTestRouter(t, afterRestart)

	requireBrokerLeaseFailure(
		t,
		postBrokerBody(t, router, "/api/v1/github/credentials/resolve", brokerResolveRequest(lease.Token)),
		"github_credential_lease_invalid",
	)
	reissued := decodeBrokerLeaseToken(t, postBrokerBody(t, router, "/api/v1/github/credentials/reissue", brokerReissueRequest(capability.Token)))
	requireBrokerCredential(t, postBrokerBody(t, router, "/api/v1/github/credentials/resolve", brokerResolveRequest(reissued)))
}
