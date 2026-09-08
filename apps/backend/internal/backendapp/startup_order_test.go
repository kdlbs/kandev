package backendapp

import (
	"errors"
	"reflect"
	"testing"
)

func TestStartOrchestratorAndAutomationConsumersOrder(t *testing.T) {
	var order []string

	err := startOrchestratorAndAutomationConsumers(
		func() error { order = append(order, "bind"); return nil },
		func() error {
			order = append(order, "orchestrator")
			return nil
		},
		func() { order = append(order, "automation") },
		func() { order = append(order, "github") },
	)

	if err != nil {
		t.Fatalf("start sequence returned error: %v", err)
	}
	want := []string{"bind", "orchestrator", "automation", "github"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("start order = %v, want %v", order, want)
	}
}

func TestStartOrchestratorAndAutomationConsumersStopsAfterOrchestratorFailure(t *testing.T) {
	startErr := errors.New("orchestrator unavailable")
	var automationStarted, githubStarted bool

	err := startOrchestratorAndAutomationConsumers(
		func() error { return nil },
		func() error { return startErr },
		func() { automationStarted = true },
		func() { githubStarted = true },
	)

	if !errors.Is(err, startErr) {
		t.Fatalf("start sequence error = %v, want %v", err, startErr)
	}
	if automationStarted || githubStarted {
		t.Fatalf("downstream consumers started after orchestrator failure: automation=%v github=%v", automationStarted, githubStarted)
	}
}

// TestStartOrchestratorAndAutomationConsumersStopsAfterBindFailure pins the
// bind-before-startup ordering invariant (docs/plans/startup-listener-before-
// recovery/task-03-ordering-guard.md): a bind failure must short-circuit
// before the orchestrator, automation, or GitHub poller ever start, exactly
// like an orchestrator-start failure does today.
func TestStartOrchestratorAndAutomationConsumersStopsAfterBindFailure(t *testing.T) {
	bindErr := errors.New("bind unavailable")
	var orchestratorStarted, automationStarted, githubStarted bool

	err := startOrchestratorAndAutomationConsumers(
		func() error { return bindErr },
		func() error { orchestratorStarted = true; return nil },
		func() { automationStarted = true },
		func() { githubStarted = true },
	)

	if !errors.Is(err, bindErr) {
		t.Fatalf("start sequence error = %v, want %v", err, bindErr)
	}
	if orchestratorStarted || automationStarted || githubStarted {
		t.Fatalf("downstream steps started after bind failure: orchestrator=%v automation=%v github=%v",
			orchestratorStarted, automationStarted, githubStarted)
	}
}

// TestPublishReadinessFlipsReadyBeforeSwappingHandler is the regression test
// for R1-2 (docs/specs/health-endpoint-version/spec.md's 2026-08-22
// amendment): startGatewayAndServe must flip the readiness flag before
// swapping in the fully wired router, never the reverse, or a request racing
// the swap could observe the real router with ready still false and get a
// spurious 503 — recreating the exact flap docs/specs/startup-listener-
// before-recovery/spec.md exists to prevent.
func TestPublishReadinessFlipsReadyBeforeSwappingHandler(t *testing.T) {
	var order []string

	publishReadiness(
		func() { order = append(order, "ready") },
		func() { order = append(order, "handler") },
	)

	want := []string{"ready", "handler"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("publishReadiness order = %v, want %v", order, want)
	}
}
