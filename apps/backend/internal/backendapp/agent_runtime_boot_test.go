package backendapp

import (
	"context"
	"net/http/httptest"
	"testing"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/webapp"
)

func TestBootInitialStateHydratesAgentRuntimeAvailability(t *testing.T) {
	availability := client.NewAvailability(nil, testLogger(t))
	availability.MarkAvailable()

	state := bootInitialState(
		context.Background(),
		nil,
		routeParams{agentRuntimeAvailability: availability},
		webapp.ClassifyRoute("/"),
	)

	runtimeState, ok := state["agentRuntime"].(client.AvailabilitySnapshot)
	if !ok {
		t.Fatalf("agentRuntime state = %#v, want AvailabilitySnapshot", state["agentRuntime"])
	}
	if runtimeState.Status != client.AvailabilityStatusAvailable || runtimeState.Reason != "" {
		t.Fatalf("agentRuntime state = %+v", runtimeState)
	}
}

func TestBootInitialStateDoesNotExposeRuntimeToUnauthenticatedVisitors(t *testing.T) {
	availability := client.NewAvailability(nil, testLogger(t))
	availability.MarkAvailable()
	availability.MarkUnavailable()
	request := httptest.NewRequest("GET", "/", nil)

	state := bootInitialState(
		context.Background(),
		request,
		routeParams{
			authSvc:                  newSSOTestAuthService(t),
			agentRuntimeAvailability: availability,
		},
		webapp.ClassifyRoute("/"),
	)

	if _, ok := state["agentRuntime"]; ok {
		t.Fatalf("unauthenticated boot state exposed agentRuntime: %#v", state)
	}
	if _, ok := state["features"]; !ok {
		t.Fatal("unauthenticated boot state lost features")
	}
	if _, ok := state["auth"]; !ok {
		t.Fatal("unauthenticated boot state lost auth")
	}
}
