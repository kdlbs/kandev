package planws

import (
	"testing"

	"github.com/kandev/kandev/internal/task/contract"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestPlanPartialReadErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"shape", &contract.PlanReadValidationError{Message: "invalid offset"}, ws.ErrorCodeValidation},
		{"range", &service.PlanSafetyError{Code: service.PlanErrorReadOffsetOutOfRange,
			Message: "out of range", NextAction: "restart"}, ws.ErrorCodeValidation},
		{"version", &service.PlanSafetyError{Code: service.PlanErrorVersionConflict,
			Message: "changed", NextAction: "reconcile", CurrentVersion: "v2"}, ws.ErrorCodeConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := GetError(&ws.Message{ID: "read", Action: ws.ActionMCPGetTaskPlan}, tc.err)
			payload := decode(t, out, err)
			require.Equal(t, tc.code, payload.Code)
			if tc.name == "version" {
				require.Equal(t, "v2", payload.Details["current_version"])
				require.Equal(t, "reconcile", payload.Details["next_action"])
				require.Equal(t, false, payload.Details["write_applied"])
			}
		})
	}
}
