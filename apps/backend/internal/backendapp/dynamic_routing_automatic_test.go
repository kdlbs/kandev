package backendapp

import (
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator"
)

func TestResolverRouteActionMapsOnlyAutomaticTryNextAndRetry(t *testing.T) {
	cases := []struct {
		request orchestrator.RouteActionRequest
		want    string
	}{
		{orchestrator.RouteActionRequest{Action: orchestrator.RouteActionTryNext, Automatic: true}, agentruntime.RouteActionAutoTryNext},
		{orchestrator.RouteActionRequest{Action: orchestrator.RouteActionTryNext}, "try_next"},
		{orchestrator.RouteActionRequest{Action: orchestrator.RouteActionRetry, Automatic: true}, agentruntime.RouteActionAutoRetry},
		{orchestrator.RouteActionRequest{Action: orchestrator.RouteActionRetry}, "retry"},
		{orchestrator.RouteActionRequest{Action: orchestrator.RouteActionStop, Automatic: true}, "stop"},
		{orchestrator.RouteActionRequest{Action: orchestrator.RouteActionSkip}, "skip"},
	}
	for _, tc := range cases {
		if got := resolverRouteAction(tc.request); got != tc.want {
			t.Fatalf("resolverRouteAction(%#v) = %q, want %q", tc.request, got, tc.want)
		}
	}
}
