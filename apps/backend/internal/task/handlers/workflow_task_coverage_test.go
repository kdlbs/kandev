package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestWorkflowSnapshotTaskCoverage(t *testing.T) {
	for _, test := range []struct {
		name, suffix    string
		count, returned int
		complete        bool
	}{
		{name: "empty", complete: true},
		{name: "complete", count: 2, returned: 2, complete: true},
		{name: "truncated", count: 2, returned: 1, suffix: "?task_limit=1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := workflowFixture()
			for index := range test.count {
				repo.snapshotTasks = append(repo.snapshotTasks, &models.Task{
					ID: []string{"one", "two"}[index], WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Task",
				})
			}
			h := newWorkflowHandlers(t, repo, &stubStepLister{})
			c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workflows/wf-1/snapshot"+test.suffix, "")
			c.Params = gin.Params{{Key: "id", Value: "wf-1"}}
			h.httpGetWorkflowSnapshot(c)
			require.Equal(t, http.StatusOK, rec.Code)
			snapshot := decodeWorkflowSnapshot(t, rec.Body.Bytes())
			require.NotNil(t, snapshot.TaskCoverage)
			require.Equal(t, "ws-1", snapshot.TaskCoverage.WorkspaceID)
			require.Equal(t, "wf-1", snapshot.TaskCoverage.WorkflowID)
			require.Equal(t, "active", snapshot.TaskCoverage.Membership)
			require.Equal(t, "server_only", snapshot.TaskCoverage.OrderingProfile)
			require.Equal(t, test.count, snapshot.TaskCoverage.Total)
			require.Equal(t, test.complete, snapshot.TaskCoverage.Complete)
			require.Len(t, snapshot.Tasks, test.returned)
		})
	}
}
