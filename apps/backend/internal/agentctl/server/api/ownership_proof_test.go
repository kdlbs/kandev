package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/common/logger"
)

// expectedProof mirrors the derivation an adopting backend performs
// independently. It is written out longhand here rather than calling the
// production helper so the test pins the wire contract, not the
// implementation's agreement with itself.
func expectedProof(credential, challenge, binding string) string {
	mac := hmac.New(sha256.New, []byte(credential))
	mac.Write([]byte(challenge))
	mac.Write([]byte{0})
	mac.Write([]byte(binding))
	return hex.EncodeToString(mac.Sum(nil))
}

func postProve(t *testing.T, baseURL, challenge string) (int, []string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"challenge": challenge})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(baseURL+"/ownership/prove", "application/json", bytes.NewReader(body)) //nolint:noctx // test-only, ephemeral httptest server
	if err != nil {
		t.Fatalf("POST /ownership/prove: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}
	var decoded struct {
		Proofs []string `json:"proofs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, decoded.Proofs
}

// TestOwnershipProveAnswersOnlyWithAValueDerivedFromTheHeldCredential is the
// server half of AC-EXECUTORS-CONTROL-OWNERSHIP-001.10: a control server
// demonstrates it already holds the credential without ever transmitting it,
// and the demonstration is bound to the caller's challenge so a value
// harvested for one attempt is worthless for the next.
func TestOwnershipProveAnswersOnlyWithAValueDerivedFromTheHeldCredential(t *testing.T) {
	log := logger.Default()
	cfg := &config.Config{
		AuthToken:      "the-real-credential",
		HomeDir:        "/home/kandev-test/.kandev",
		ServerIdentity: "server-identity-abc",
	}
	mgr := instance.NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	server := httptest.NewServer(NewControlServer(cfg, mgr, log).Router())
	defer server.Close()

	status, proofs := postProve(t, server.URL, "challenge-one")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(proofs) != 1 {
		t.Fatalf("len(proofs) = %d, want 1 (only the current credential is acceptable)", len(proofs))
	}
	want := expectedProof("the-real-credential", "challenge-one", "/home/kandev-test/.kandev")
	if proofs[0] != want {
		t.Fatalf("proof = %q, want %q", proofs[0], want)
	}

	// The credential itself is never the demonstration.
	if proofs[0] == "the-real-credential" {
		t.Fatal("the proof is the credential verbatim")
	}

	// A different challenge yields a different proof, so a captured proof
	// cannot be replayed against the next attempt's challenge.
	_, second := postProve(t, server.URL, "challenge-two")
	if len(second) != 1 || second[0] == proofs[0] {
		t.Fatalf("proof did not vary with the challenge: %v vs %v", proofs, second)
	}

	// A server holding a different credential cannot produce this answer.
	if proofs[0] == expectedProof("some-other-credential", "challenge-one", "/home/kandev-test/.kandev") {
		t.Fatal("proof did not depend on the credential")
	}
	// Nor can one whose home directory differs, which is bound into the
	// derivation precisely because it is no longer disclosed.
	if proofs[0] == expectedProof("the-real-credential", "challenge-one", "/some/other/home") {
		t.Fatal("proof did not depend on the home directory binding")
	}
}

// TestOwnershipProveCoversEveryAcceptableCredentialDuringTheTwoPhaseWindow:
// AC-EXECUTORS-CONTROL-OWNERSHIP-002.8 keeps at most two credentials
// acceptable while a rotation is unconfirmed, and AC-002.10's replay path
// means the adopting backend may still hold the superseded one. The proof
// must cover both, or that replay would be refused as an impostor.
func TestOwnershipProveCoversEveryAcceptableCredentialDuringTheTwoPhaseWindow(t *testing.T) {
	creds := newCredentialState("bootstrap-credential")

	single := creds.ProveOwnership("chal", "/home")
	if len(single) != 1 || single[0] != expectedProof("bootstrap-credential", "chal", "/home") {
		t.Fatalf("before any rotation, proofs = %v", single)
	}

	_, replacement, err := creds.Rotate("bootstrap-credential")
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	both := creds.ProveOwnership("chal", "/home")
	if len(both) != 2 {
		t.Fatalf("len(proofs) = %d, want 2 while the rotation is unconfirmed", len(both))
	}
	wantNew := expectedProof(replacement, "chal", "/home")
	wantOld := expectedProof("bootstrap-credential", "chal", "/home")
	if both[0] != wantNew || both[1] != wantOld {
		t.Fatalf("proofs = %v, want [replacement, superseded]", both)
	}

	if err := creds.Confirm(1); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	after := creds.ProveOwnership("chal", "/home")
	if len(after) != 1 || after[0] != wantNew {
		t.Fatalf("after confirmation, proofs = %v, want only the replacement", after)
	}
}

// TestOwnershipProveRejectsAnEmptyChallenge: a response must never be
// accepted for a challenge the backend did not generate
// (AC-EXECUTORS-CONTROL-OWNERSHIP-001.10), so a caller that supplies no
// challenge gets no proof rather than one over a fixed empty string.
func TestOwnershipProveRejectsAnEmptyChallenge(t *testing.T) {
	log := logger.Default()
	cfg := &config.Config{AuthToken: "cred", HomeDir: "/home", ServerIdentity: "id"}
	mgr := instance.NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	server := httptest.NewServer(NewControlServer(cfg, mgr, log).Router())
	defer server.Close()

	if status, _ := postProve(t, server.URL, ""); status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an empty challenge", status)
	}
}

// TestIdentityDisclosesNoFilesystemPath is
// AC-EXECUTORS-CONTROL-OWNERSHIP-001.11: the endpoint that sits below
// authentication carries only what the decision to authenticate needs.
func TestIdentityDisclosesNoFilesystemPath(t *testing.T) {
	log := logger.Default()
	cfg := &config.Config{
		AuthToken:         "cred",
		HomeDir:           "/home/kandev-test/.kandev",
		ServerIdentity:    "server-identity-abc",
		DiagnosticLogPath: "/home/kandev-test/.kandev/logs/agentctl-diagnostic.log",
	}
	mgr := instance.NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	server := httptest.NewServer(NewControlServer(cfg, mgr, log).Router())
	defer server.Close()

	resp, err := http.Get(server.URL + "/identity") //nolint:noctx // test-only, ephemeral httptest server
	if err != nil {
		t.Fatalf("GET /identity: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, forbidden := range []string{"home_dir", "diagnostic_log_path"} {
		if _, present := payload[forbidden]; present {
			t.Errorf("unauthenticated /identity still discloses %q", forbidden)
		}
	}
	for _, required := range []string{"server_identity", "capabilities", "unowned_period_ms"} {
		if _, present := payload[required]; !present {
			t.Errorf("/identity no longer reports %q, which adoption needs", required)
		}
	}
}
