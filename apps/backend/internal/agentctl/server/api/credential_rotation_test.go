package api

import (
	"net/http/httptest"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/common/logger"
)

// --- credentialState unit tests ---

// TestCredentialStateRotatePresentingLatestAllocatesNewRotation pins the
// ordinary rotation path: presenting the current fully-authenticating
// credential allocates rotation 1 and a fresh replacement.
func TestCredentialStateRotatePresentingLatestAllocatesNewRotation(t *testing.T) {
	c := newCredentialState("initial-token")

	id, replacement, err := c.Rotate("initial-token")
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if id != 1 {
		t.Fatalf("rotation id = %d, want 1", id)
	}
	if replacement == "" || replacement == "initial-token" {
		t.Fatalf("replacement = %q, want a fresh non-empty credential", replacement)
	}
	if got := c.Latest(); got != replacement {
		t.Fatalf("Latest() = %q, want the replacement %q", got, replacement)
	}
}

// TestCredentialStateRotateRejectsUnknownCredential pins that a credential
// outside the acceptable set (AC-002.8) is refused.
func TestCredentialStateRotateRejectsUnknownCredential(t *testing.T) {
	c := newCredentialState("initial-token")

	if _, _, err := c.Rotate("some-other-token"); err == nil {
		t.Fatal("Rotate with unknown credential = nil error, want a rejection")
	}
}

// TestCredentialStateRotateIsIdempotentUnderRetryWithSupersededCredential
// pins AC-EXECUTORS-CONTROL-OWNERSHIP-002.10: presenting the credential the
// highest-numbered rotation superseded, while unconfirmed, returns the same
// rotation again rather than allocating a new one.
func TestCredentialStateRotateIsIdempotentUnderRetryWithSupersededCredential(t *testing.T) {
	c := newCredentialState("initial-token")

	id1, replacement1, err := c.Rotate("initial-token")
	if err != nil {
		t.Fatalf("first Rotate: %v", err)
	}

	id2, replacement2, err := c.Rotate("initial-token")
	if err != nil {
		t.Fatalf("retry Rotate: %v", err)
	}

	if id2 != id1 {
		t.Fatalf("retry rotation id = %d, want the same id %d (no new allocation)", id2, id1)
	}
	if replacement2 != replacement1 {
		t.Fatalf("retry replacement = %q, want the same replacement %q", replacement2, replacement1)
	}
}

// TestCredentialStateAcceptsAdoptionOnlyForSupersededUntilConfirmed pins
// AC-002.7: the superseded credential authenticates adoption-only
// operations until confirmed, and the replacement authenticates everything
// throughout.
func TestCredentialStateAcceptsAdoptionOnlyForSupersededUntilConfirmed(t *testing.T) {
	c := newCredentialState("initial-token")
	_, replacement, err := c.Rotate("initial-token")
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	if !c.AcceptsAdoptionOnly("initial-token") {
		t.Fatal("AcceptsAdoptionOnly(superseded) = false before confirm, want true")
	}
	if !c.AcceptsAdoptionOnly(replacement) {
		t.Fatal("AcceptsAdoptionOnly(latest) = false, want true (latest always qualifies)")
	}
	if c.AcceptsFull("initial-token") {
		t.Fatal("AcceptsFull(superseded) = true, want false (superseded never authenticates a full operation)")
	}
	if !c.AcceptsFull(replacement) {
		t.Fatal("AcceptsFull(latest) = false, want true")
	}
}

// TestCredentialStateConfirmDropsSupersededCredential pins that confirming
// the highest-numbered rotation removes the superseded credential from the
// acceptable set entirely.
func TestCredentialStateConfirmDropsSupersededCredential(t *testing.T) {
	c := newCredentialState("initial-token")
	id, _, err := c.Rotate("initial-token")
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	if err := c.Confirm(id); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	if c.AcceptsAdoptionOnly("initial-token") {
		t.Fatal("AcceptsAdoptionOnly(superseded) = true after confirm, want false")
	}
	if _, _, err := c.Rotate("initial-token"); err == nil {
		t.Fatal("Rotate(superseded) after confirm = nil error, want a rejection")
	}
}

// TestCredentialStateConfirmOfEarlierRotationIsAcceptedAndHasNoEffect pins
// that a confirmation naming an earlier rotation identifier is accepted but
// does not revoke the credential issued after it (AC-002.6): a delayed or
// duplicated confirmation can never win against a later rotation.
func TestCredentialStateConfirmOfEarlierRotationIsAcceptedAndHasNoEffect(t *testing.T) {
	c := newCredentialState("initial-token")
	firstID, firstReplacement, err := c.Rotate("initial-token")
	if err != nil {
		t.Fatalf("first Rotate: %v", err)
	}
	if _, _, err := c.Rotate(firstReplacement); err != nil {
		t.Fatalf("second Rotate: %v", err)
	}

	if err := c.Confirm(firstID); err != nil {
		t.Fatalf("Confirm(earlier id) = %v, want accepted with no effect", err)
	}

	// The second rotation's superseded credential (firstReplacement) must
	// still be adoption-only-acceptable: confirming the stale first
	// identifier must not have dropped it.
	if !c.AcceptsAdoptionOnly(firstReplacement) {
		t.Fatal("AcceptsAdoptionOnly(second rotation's superseded credential) = false, want true (stale confirm had no effect)")
	}
}

// TestCredentialStateConfirmOfUnissuedRotationIsRejected pins that naming
// an identifier the control server never allocated is rejected.
func TestCredentialStateConfirmOfUnissuedRotationIsRejected(t *testing.T) {
	c := newCredentialState("initial-token")

	if err := c.Confirm(1); err == nil {
		t.Fatal("Confirm(never-issued id) = nil error, want a rejection")
	}
}

// TestCredentialStateAcceptableSetCappedAtTwo pins AC-002.8: rotating a
// second time invalidates the credential the *first* rotation superseded,
// even though its rotation was never confirmed.
func TestCredentialStateAcceptableSetCappedAtTwo(t *testing.T) {
	c := newCredentialState("initial-token")
	_, firstReplacement, err := c.Rotate("initial-token")
	if err != nil {
		t.Fatalf("first Rotate: %v", err)
	}
	if _, _, err := c.Rotate(firstReplacement); err != nil {
		t.Fatalf("second Rotate: %v", err)
	}

	if c.AcceptsAdoptionOnly("initial-token") {
		t.Fatal("AcceptsAdoptionOnly(original credential) = true after a second rotation, want false (acceptable set capped at two)")
	}
}

// --- HTTP handler tests ---

func newRotationTestServer(t *testing.T) (*ControlServer, *httptest.Server, string, int) {
	t.Helper()
	cfg := &config.Config{AuthToken: "initial-token"}
	log := logger.Default()
	cs := NewControlServer(cfg, &instance.Manager{}, log)
	server := httptest.NewServer(cs.Router())
	t.Cleanup(server.Close)
	host, port := parseHostPort(t, server.URL)
	return cs, server, host, port
}

// TestHandleCredentialRotateReturnsNewCredentialAndRenewsOwnership pins the
// end-to-end rotate flow through the real handler and client.
func TestHandleCredentialRotateReturnsNewCredentialAndRenewsOwnership(t *testing.T) {
	cs, _, host, port := newRotationTestServer(t)
	log := logger.Default()
	client := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("initial-token"))

	backdateOwnership(cs, time.Hour)

	result, err := client.RotateCredential(t.Context())
	if err != nil {
		t.Fatalf("RotateCredential: %v", err)
	}
	if result.RotationID != 1 {
		t.Fatalf("RotationID = %d, want 1", result.RotationID)
	}
	if result.Credential == "" || result.Credential == "initial-token" {
		t.Fatalf("Credential = %q, want a fresh non-empty replacement", result.Credential)
	}
	if elapsed := cs.ownership.UnownedFor(); elapsed >= time.Minute {
		t.Fatalf("UnownedFor after rotate = %v, want well under the backdated hour (rotation should have renewed)", elapsed)
	}
}

// TestSupersededCredentialCannotListInstances pins that the superseded
// credential authenticates only the adoption-only paths, not an ordinary
// instance operation.
func TestSupersededCredentialCannotListInstances(t *testing.T) {
	_, _, host, port := newRotationTestServer(t)
	log := logger.Default()
	rotatingClient := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("initial-token"))
	if _, err := rotatingClient.RotateCredential(t.Context()); err != nil {
		t.Fatalf("RotateCredential: %v", err)
	}

	supersededClient := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("initial-token"))
	if _, err := supersededClient.ListInstances(t.Context()); err == nil {
		t.Fatal("ListInstances with superseded credential = nil error, want a rejection")
	}
}

// TestSupersededCredentialCanRetryRotateUntilConfirmed pins the
// idempotent-replay path reachable over HTTP: retrying rotate with the
// superseded credential returns the same rotation again.
func TestSupersededCredentialCanRetryRotateUntilConfirmed(t *testing.T) {
	_, _, host, port := newRotationTestServer(t)
	log := logger.Default()
	client := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("initial-token"))

	first, err := client.RotateCredential(t.Context())
	if err != nil {
		t.Fatalf("first RotateCredential: %v", err)
	}

	retryClient := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("initial-token"))
	retry, err := retryClient.RotateCredential(t.Context())
	if err != nil {
		t.Fatalf("retry RotateCredential: %v", err)
	}

	if retry.RotationID != first.RotationID || retry.Credential != first.Credential {
		t.Fatalf("retry = %+v, want the same rotation as first %+v", retry, first)
	}
}

// TestHandleCredentialConfirmDropsSupersededCredential pins that after
// confirmation the superseded credential is refused even for rotation
// retries.
func TestHandleCredentialConfirmDropsSupersededCredential(t *testing.T) {
	_, _, host, port := newRotationTestServer(t)
	log := logger.Default()
	client := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("initial-token"))

	result, err := client.RotateCredential(t.Context())
	if err != nil {
		t.Fatalf("RotateCredential: %v", err)
	}

	client.SetAuthToken(result.Credential)
	if err := client.ConfirmCredentialRotation(t.Context(), result.RotationID); err != nil {
		t.Fatalf("ConfirmCredentialRotation: %v", err)
	}

	supersededClient := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("initial-token"))
	if _, err := supersededClient.RotateCredential(t.Context()); err == nil {
		t.Fatal("RotateCredential with confirmed-away superseded credential = nil error, want a rejection")
	}
}

// TestHandleCredentialConfirmRejectsUnissuedRotationID pins the HTTP-layer
// rejection of an identifier the control server never allocated.
func TestHandleCredentialConfirmRejectsUnissuedRotationID(t *testing.T) {
	_, _, host, port := newRotationTestServer(t)
	log := logger.Default()
	client := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("initial-token"))

	if err := client.ConfirmCredentialRotation(t.Context(), 999); err == nil {
		t.Fatal("ConfirmCredentialRotation(never-issued id) = nil error, want a rejection")
	}
}

// TestHandleCredentialRotateRefusedAfterShutdownBegun pins the one-way-door
// refusal: rotate is the first step of an adoption attempt and must be
// refused once the unowned-shutdown latch has fired.
func TestHandleCredentialRotateRefusedAfterShutdownBegun(t *testing.T) {
	cs, _, host, port := newRotationTestServer(t)
	cs.ownership.BeginShutdown()
	log := logger.Default()
	client := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("initial-token"))

	if _, err := client.RotateCredential(t.Context()); err == nil {
		t.Fatal("RotateCredential after shutdown began = nil error, want a rejection")
	}
}
