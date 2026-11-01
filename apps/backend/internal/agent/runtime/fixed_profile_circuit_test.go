package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

func TestFixedProfileCircuitUsesDynamicRoutingFlag(t *testing.T) {
	ctx := context.Background()
	engine := dynamic.NewEngine()
	resolver := NewProfileExecutionResolver(nil, engine, false)
	key := dynamic.ResourceKey(dynamic.ScopeProfile, "fixed")
	failure := &routingerr.Error{Code: routingerr.CodeRateLimited, Confidence: routingerr.ConfHigh}
	resolver.RecordFixedProfileFailure(ctx, "fixed", key, failure)
	if got := engine.Circuits().Snapshots(); len(got) != 0 {
		t.Fatalf("disabled routing charged a fixed failure: %+v", got)
	}
	engine.Circuits().OpenFailure(key, dynamic.FailureOpen{Code: failure.Code, Now: time.Now()})
	if got := engine.Circuits().Snapshots(); len(got) != 1 || got[0].Failures != 1 {
		t.Fatalf("test circuit history was not established: %+v", got)
	}
	if resolver.ResetFixedProfileCircuitFailures(ctx, key) {
		t.Fatal("disabled routing reset fixed-profile circuit history")
	}
	if got := engine.Circuits().Snapshots()[0].Failures; got != 1 {
		t.Fatalf("disabled routing changed fixed-profile failure count to %d", got)
	}
	resolver.SetEnabled(true)
	resolver.RecordFixedProfileFailure(ctx, "fixed", key, failure)
	if suspension, known := resolver.CapturedFixedProfileSuspension("fixed", key, time.Now()); !known || !suspension.Blocked() {
		t.Fatalf("enabled routing ignored circuit: %+v, known=%v", suspension, known)
	}
	if got := engine.Circuits().Snapshots()[0].Failures; got != 1 {
		t.Fatalf("enabled routing did not charge fixed failure: %d", got)
	}
	if !resolver.ResetFixedProfileCircuitFailures(ctx, key) {
		t.Fatal("enabled routing did not reset fixed-profile failure count")
	}
	if got := engine.Circuits().Snapshots()[0].Failures; got != 0 {
		t.Fatalf("enabled routing retained fixed-profile failure count %d", got)
	}
	resolver.SetEnabled(false)
	if suspension, _ := resolver.CapturedFixedProfileSuspension("fixed", key, time.Now()); suspension.Blocked() {
		t.Fatalf("disabled routing blocked fixed prompt: %+v", suspension)
	}
}
