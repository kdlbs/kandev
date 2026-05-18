package models_test

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/office/models"
)

// TestAdapterPhaseAlignment fails to compile (referenced constant removed) or
// fails at runtime (value drift) if the office-tier AdapterPhase set drifts
// from routingerr.Phase. The duplication in office/models/enums.go is
// intentional — it keeps the office tier free of a runtime-tier import in
// production code — but this *_test.go anchor catches drift on every test
// run without imposing the dependency on production builds.
func TestAdapterPhaseAlignment(t *testing.T) {
	pairs := []struct {
		office  models.AdapterPhase
		runtime routingerr.Phase
	}{
		{models.AdapterPhaseAuthCheck, routingerr.PhaseAuthCheck},
		{models.AdapterPhaseProcessStart, routingerr.PhaseProcessStart},
		{models.AdapterPhaseSessionInit, routingerr.PhaseSessionInit},
		{models.AdapterPhasePromptSend, routingerr.PhasePromptSend},
		{models.AdapterPhaseStreaming, routingerr.PhaseStreaming},
		{models.AdapterPhaseToolExecution, routingerr.PhaseToolExecution},
		{models.AdapterPhaseShutdown, routingerr.PhaseShutdown},
	}
	for _, p := range pairs {
		if string(p.office) != string(p.runtime) {
			t.Errorf("AdapterPhase drift: office=%q runtime=%q", p.office, p.runtime)
		}
	}
}
