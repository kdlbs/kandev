package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestManagerLaunchMarksSessionCreatedThisLifetime pins the own-record side
// of design 02's "own-record-created-this-lifetime vs inherited-record"
// carve-out (Layer 6.7): a session launched fresh through Manager.Launch is
// "created this lifetime", never a session Manager has simply heard of
// through some other path.
func TestManagerLaunchMarksSessionCreatedThisLifetime(t *testing.T) {
	mgr, _ := newEnvironmentExecutionTestManager(t, nil)

	if mgr.wasCreatedThisLifetime("session-launched") {
		t.Fatal("expected session to NOT be marked before Launch is called")
	}

	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:         "task-launched",
		SessionID:      "session-launched",
		AgentProfileID: "profile-launched",
		ExecutorType:   string(models.ExecutorTypeLocal),
		IsEphemeral:    true,
	})
	if err != nil {
		t.Fatalf("Manager.Launch: %v", err)
	}

	if !mgr.wasCreatedThisLifetime("session-launched") {
		t.Fatal("expected session to be marked created-this-lifetime after a successful Launch")
	}
	if mgr.wasCreatedThisLifetime("some-other-session") {
		t.Fatal("expected an unrelated session to remain unmarked")
	}
	if mgr.wasCreatedThisLifetime("") {
		t.Fatal("expected an empty session ID to never be marked")
	}
}

// TestManagerMarkSessionCreatedThisLifetimeNeverReset pins that, unlike
// retrackedSessions, standaloneOwnSessions must survive across a Start()
// call -- it needs to accumulate for the whole process lifetime, not just
// one recovery pass.
func TestManagerMarkSessionCreatedThisLifetimeNeverReset(t *testing.T) {
	mgr, _ := newEnvironmentExecutionTestManager(t, nil)
	mgr.markSessionCreatedThisLifetime("session-own")

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Manager.Start: %v", err)
	}

	if !mgr.wasCreatedThisLifetime("session-own") {
		t.Fatal("expected standaloneOwnSessions to survive a Start() call, unlike retrackedSessions")
	}
}
