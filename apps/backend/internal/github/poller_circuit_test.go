package github

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/authcircuit"
)

// TestPollerCircuits_RecordFailureThenOpen confirms a classified failure
// opens the circuit for the backoff window, and a subsequent success clears
// it — the two building blocks checkPRWatches/tryBatchedPRWatchCheck rely on.
func TestPollerCircuits_RecordFailureThenOpen(t *testing.T) {
	c := newPollerCircuits()
	now := time.Now().UTC()

	if c.open("ws-1", now) {
		t.Fatalf("a workspace with no recorded outcome must not be open")
	}

	c.recordOutcome("ws-1", authcircuit.FailureClassAuth, now)
	if !c.open("ws-1", now) {
		t.Fatalf("expected circuit to be open immediately after an auth failure")
	}
	if c.open("ws-2", now) {
		t.Fatalf("recording a failure for ws-1 must not open ws-2's circuit")
	}

	c.recordOutcome("ws-1", authcircuit.FailureClassNone, now)
	if c.open("ws-1", now) {
		t.Fatalf("a success must clear the open circuit")
	}
}

// TestPollerCircuits_ResetIfFingerprintChanged confirms the credential
// fingerprint reset path: unchanged/empty fingerprints never reset, but a
// genuine change does and reports true exactly once.
func TestPollerCircuits_ResetIfFingerprintChanged(t *testing.T) {
	c := newPollerCircuits()
	now := time.Now().UTC()
	c.recordOutcome("ws-1", authcircuit.FailureClassAuth, now)

	if c.resetIfFingerprintChanged("ws-1", "") {
		t.Fatalf("an empty fingerprint must never reset an open circuit")
	}
	if !c.open("ws-1", now) {
		t.Fatalf("circuit should still be open after an empty-fingerprint no-op")
	}

	// First observation of a fingerprint is not itself a "change".
	if c.resetIfFingerprintChanged("ws-1", "active:1") {
		t.Fatalf("the first observed fingerprint must not itself count as a change")
	}
	if !c.open("ws-1", now) {
		t.Fatalf("circuit should still be open after first fingerprint observation")
	}

	if !c.resetIfFingerprintChanged("ws-1", "active:2") {
		t.Fatalf("a changed fingerprint must reset the circuit")
	}
	if c.open("ws-1", now) {
		t.Fatalf("circuit must be closed immediately after a fingerprint-triggered reset")
	}
}

// TestPollerCircuits_Summary confirms bounded, class-grouped aggregation
// with no per-workspace identifiers.
func TestPollerCircuits_Summary(t *testing.T) {
	c := newPollerCircuits()
	now := time.Now().UTC()
	c.recordOutcome("ws-auth", authcircuit.FailureClassAuth, now)
	c.recordOutcome("ws-config", authcircuit.FailureClassConfig, now)
	c.recordOutcome("ws-transient", authcircuit.FailureClassTransient, now)
	c.recordOutcome("ws-ok", authcircuit.FailureClassNone, now)

	auth, config, transient := c.summary(now)
	if auth != 1 || config != 1 || transient != 1 {
		t.Fatalf("summary = (auth=%d, config=%d, transient=%d), want (1, 1, 1)", auth, config, transient)
	}
}

func TestClassifyPollErr_NilIsNone(t *testing.T) {
	if got := classifyPollErr(nil); got != authcircuit.FailureClassNone {
		t.Fatalf("classifyPollErr(nil) = %q, want none", got)
	}
}

func TestClassifyPollErr_ConnectionSentinelsAreAuth(t *testing.T) {
	for _, err := range []error{ErrGitHubConnectionInvalid, ErrGitHubNotConfigured} {
		if got := classifyPollErr(err); got != authcircuit.FailureClassAuth {
			t.Fatalf("classifyPollErr(%v) = %q, want auth", err, got)
		}
	}
}

func TestClassifyPollErr_RepoNotResolvableIsConfig(t *testing.T) {
	if got := classifyPollErr(ErrRepoNotResolvable); got != authcircuit.FailureClassConfig {
		t.Fatalf("classifyPollErr(ErrRepoNotResolvable) = %q, want config", got)
	}
	wrapped := errors.New("Could not resolve to a Repository with the name 'x'")
	if got := classifyPollErr(wrapped); got != authcircuit.FailureClassConfig {
		t.Fatalf("classifyPollErr(graphql-not-found) = %q, want config", got)
	}
}

func TestClassifyPollErr_GitHubAPIErrorByStatus(t *testing.T) {
	tests := []struct {
		status int
		want   authcircuit.FailureClass
	}{
		{http.StatusUnauthorized, authcircuit.FailureClassAuth},
		{http.StatusForbidden, authcircuit.FailureClassAuth},
		{http.StatusNotFound, authcircuit.FailureClassConfig},
		{http.StatusUnprocessableEntity, authcircuit.FailureClassConfig},
		{http.StatusBadRequest, authcircuit.FailureClassConfig},
		{http.StatusInternalServerError, authcircuit.FailureClassTransient},
	}
	for _, tt := range tests {
		err := &GitHubAPIError{StatusCode: tt.status, Endpoint: "/x"}
		if got := classifyPollErr(err); got != tt.want {
			t.Fatalf("classifyPollErr(status=%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestClassifyPollErr_UnrecognizedIsTransient(t *testing.T) {
	if got := classifyPollErr(errors.New("connection reset")); got != authcircuit.FailureClassTransient {
		t.Fatalf("classifyPollErr(unrecognized) = %q, want transient", got)
	}
}

// TestCheckPRWatches_OpenCircuitSkipsSearchEntirely confirms checkPRWatches
// never calls FindPRByBranch for a workspace whose circuit is already open
// — the end-to-end behavior AC-9 requires for the PR-monitor loop.
func TestCheckPRWatches_OpenCircuitSkipsSearchEntirely(t *testing.T) {
	poller, _, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	watch := withTestWorkspace(&PRWatch{TaskID: "t1", Owner: "o", Repo: "r", Branch: "feat"})
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	poller.circuits.recordOutcome(testWorkspaceID, authcircuit.FailureClassAuth, time.Now().UTC())

	poller.checkPRWatches(ctx)

	if got := mockClient.FindPRByBranchCallCount(); got != 0 {
		t.Fatalf("FindPRByBranch calls = %d, want 0 while the workspace circuit is open", got)
	}
}

// TestCheckPRWatches_ClosedCircuitStillSearches is the control for the test
// above: with no recorded failures, the same watch is searched normally.
func TestCheckPRWatches_ClosedCircuitStillSearches(t *testing.T) {
	poller, _, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	watch := withTestWorkspace(&PRWatch{TaskID: "t1", Owner: "o", Repo: "r", Branch: "feat"})
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	poller.checkPRWatches(ctx)

	if got := mockClient.FindPRByBranchCallCount(); got != 1 {
		t.Fatalf("FindPRByBranch calls = %d, want 1 with a closed circuit", got)
	}
}
