package backendapp

import (
	"errors"
	"reflect"
	"testing"
)

func TestStartOrchestratorAndAutomationConsumersOrder(t *testing.T) {
	var order []string

	err := startOrchestratorAndAutomationConsumers(
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
	want := []string{"orchestrator", "automation", "github"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("start order = %v, want %v", order, want)
	}
}

func TestStartOrchestratorAndAutomationConsumersStopsAfterOrchestratorFailure(t *testing.T) {
	startErr := errors.New("orchestrator unavailable")
	var automationStarted, githubStarted bool

	err := startOrchestratorAndAutomationConsumers(
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
