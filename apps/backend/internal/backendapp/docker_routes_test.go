package backendapp

import (
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

// TestDockerSessionAuthorizerNilTaskServiceIsUntypedNil pins the fail-closed
// half of the Docker route wiring: a partial build with no task service must
// hand the handlers an untyped nil, which they treat as "deny scoped callers",
// rather than a non-nil interface wrapping a nil *Service that panics on call.
func TestDockerSessionAuthorizerNilTaskServiceIsUntypedNil(t *testing.T) {
	if authorizer := dockerSessionAuthorizer(nil); authorizer != nil {
		t.Fatalf("dockerSessionAuthorizer(nil) = %#v, want untyped nil", authorizer)
	}
}

func TestDockerSessionAuthorizerReturnsTaskService(t *testing.T) {
	if authorizer := dockerSessionAuthorizer(&taskservice.Service{}); authorizer == nil {
		t.Fatal("dockerSessionAuthorizer(svc) = nil, want the task service")
	}
}
