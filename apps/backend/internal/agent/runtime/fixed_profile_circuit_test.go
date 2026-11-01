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
	resolver.SetEnabled(true)
	resolver.RecordFixedProfileFailure(ctx, "fixed", key, failure)
	if suspension, known := resolver.CapturedFixedProfileSuspension("fixed", key, time.Now()); !known || !suspension.Blocked() {
		t.Fatalf("enabled routing ignored circuit: %+v, known=%v", suspension, known)
	}
	resolver.SetEnabled(false)
	if suspension, _ := resolver.CapturedFixedProfileSuspension("fixed", key, time.Now()); suspension.Blocked() {
		t.Fatalf("disabled routing blocked fixed prompt: %+v", suspension)
	}
}
