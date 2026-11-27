package backendapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-LABELS-002.5
func TestOfficeTaskLabelRemovalScope(t *testing.T) {
	h := newOfficeScopeHarness(t)
	for _, tc := range []struct {
		name, owner, workspace, task string
		agent, allowed               bool
	}{
		{"browser owner", officeScopeUserA, officeScopeUserA, officeScopeUserA, false, true},
		{"browser foreign task", officeScopeUserB, officeScopeUserB, officeScopeUserA, false, false},
		{"browser owned workspace mismatch", officeScopeUserA, officeScopeUserASecond, officeScopeUserA, false, false},
		{"browser missing task", officeScopeUserA, officeScopeUserA, "missing", false, false},
		{"agent owner", officeScopeUserA, officeScopeUserA, officeScopeUserA, true, true},
		{"agent foreign workspace", officeScopeUserA, officeScopeUserB, officeScopeUserB, true, false},
		{"agent foreign task with own workspace", officeScopeUserA, officeScopeUserA, officeScopeUserB, true, false},
		{"agent missing task", officeScopeUserA, officeScopeUserA, "missing", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reached string
			engine := officeScopeEngine(h, tc.owner, &reached)
			token := ""
			if tc.agent {
				engine, token = officeAgentEngine(t, h, tc.owner, &reached)
			}
			path := "/api/v1/office/workspaces/" + h.workspaces[tc.workspace] +
				"/tasks/task-" + tc.task + "/labels?name=area%2Fbackend"
			req := httptest.NewRequest(http.MethodDelete, path, nil)
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if tc.allowed {
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, "/api/v1/office/workspaces/:wsId/tasks/:taskId/labels", reached)
			} else {
				require.NotEqual(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Empty(t, reached, "scope denial must happen before handler dispatch")
			}
		})
	}
}
