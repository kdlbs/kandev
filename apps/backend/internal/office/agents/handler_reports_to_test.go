package agents

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestUpdateAgent_ReportsToValidationReturnsClientErrorCode(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, svc *AgentService) (targetID, reportsTo string)
		wantCode   string
		wantStatus int
	}{
		{
			name: "self",
			setup: func(t *testing.T, svc *AgentService) (string, string) {
				target := createAndGetAgent(t, svc, svc.repo, &models.AgentInstance{
					WorkspaceID: "ws-1",
					Name:        "Worker",
					Role:        models.AgentRoleWorker,
				})
				return target.ID, target.ID
			},
			wantCode:   "agent_reports_to_self",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unknown manager",
			setup: func(t *testing.T, svc *AgentService) (string, string) {
				target := createAndGetAgent(t, svc, svc.repo, &models.AgentInstance{
					WorkspaceID: "ws-1",
					Name:        "Worker",
					Role:        models.AgentRoleWorker,
				})
				return target.ID, "missing-manager"
			},
			wantCode:   "agent_reports_to_invalid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "cycle",
			setup: func(t *testing.T, svc *AgentService) (string, string) {
				ceo := createAndGetAgent(t, svc, svc.repo, &models.AgentInstance{
					WorkspaceID: "ws-1",
					Name:        "CEO",
					Role:        models.AgentRoleCEO,
				})
				manager := createAndGetAgent(t, svc, svc.repo, &models.AgentInstance{
					WorkspaceID: "ws-1",
					Name:        "Manager",
					Role:        models.AgentRoleWorker,
					ReportsTo:   ceo.ID,
				})
				worker := createAndGetAgent(t, svc, svc.repo, &models.AgentInstance{
					WorkspaceID: "ws-1",
					Name:        "Worker",
					Role:        models.AgentRoleWorker,
					ReportsTo:   manager.ID,
				})
				return manager.ID, worker.ID
			},
			wantCode:   "agent_reports_to_cycle",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newTestAgentService(t)
			targetID, reportsTo := tt.setup(t, svc)
			rec := newPatchAgentRecorder(t, svc, targetID, mustJSON(t, map[string]string{
				"reports_to": reportsTo,
			}))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["code"] != tt.wantCode {
				t.Errorf("code = %v, want %q; body=%s", body["code"], tt.wantCode, rec.Body.String())
			}
		})
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(data)
}
