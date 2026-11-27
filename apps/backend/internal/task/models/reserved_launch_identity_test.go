package models

import (
	"context"
	"testing"
)

func TestExplicitManualCreationIsBoundToRequestAndTask(t *testing.T) {
	ctx := context.Background()
	if ExplicitManualCreation(ctx, "task") {
		t.Fatal("ordinary request has manual authority")
	}
	manual := WithExplicitManualCreation(ctx, "task")
	if !ExplicitManualCreation(manual, "task") {
		t.Fatal("explicit request lost authority")
	}
	if ExplicitManualCreation(manual, "foreign") {
		t.Fatal("manual authority crossed task ownership")
	}
	if ExplicitManualCreation(WithExplicitManualCreation(manual, ""), "task") {
		t.Fatal("automatic work inherited manual authority")
	}
}

func TestReservedLaunchIdentityContextRequiresCompleteTuple(t *testing.T) {
	identity := ReservedLaunchIdentity{TaskID: "task", SessionID: "session", TurnID: "turn", SubmissionID: "submission"}
	ctx := WithReservedLaunchIdentity(context.Background(), identity)
	got, ok := ReservedLaunchIdentityFromContext(ctx)
	if !ok || got != identity {
		t.Fatalf("lost identity: %#v, %v", got, ok)
	}
	identity.SubmissionID = ""
	if _, ok := ReservedLaunchIdentityFromContext(WithReservedLaunchIdentity(ctx, identity)); ok {
		t.Fatal("incomplete identity admitted")
	}
	if _, ok := ReservedLaunchIdentityFromContext(context.Background()); ok {
		t.Fatal("identity leaked across contexts")
	}
}
