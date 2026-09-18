package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssistantMaintenancePatchInvalidatesChecks(t *testing.T) {
	s, b, candidate, sandbox, _ := maintenanceFixture(t)
	base, body := prepareMaintenanceFixture(t, s, b, candidate)
	router := assistantRouter(s)
	body["action"], body["operation_id"] = "check", "checked"
	require.Equal(t, 200, runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body).Code)
	body["action"], body["operation_id"] = "patch", "patched"
	require.Equal(t, 200, runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body).Code)
	body["action"], body["operation_id"] = "commit", "must-check-again"
	response := runtimeRequest(t, router, "POST", base+"/maintenance", "", "", body)
	require.Equal(t, 422, response.Code, response.Body.String())
	require.Zero(t, sandbox.commits)
	_, err := s.Repo.MaintenanceValidation(context.Background(), b.ID, candidate.ID)
	require.Error(t, err, "the UI cannot offer a passing receipt for a prior tree")
}
