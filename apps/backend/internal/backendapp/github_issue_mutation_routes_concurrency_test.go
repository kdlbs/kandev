package backendapp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.2, AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
func TestGitHubIssueMutationRegisteredRoutesConcurrent(t *testing.T) {
	for _, unlink := range []bool{false, true} {
		for first := range 2 {
			t.Run(fmt.Sprintf("unlink_%t_first_%d", unlink, first), func(t *testing.T) {
				var mergeGate *issueMutationTaskHook
				h := newIssueMutationHarness(t, func(i int, r *sqliterepo.Repository) repository.TaskRepository {
					if i == 0 {
						return r
					}
					mergeGate = &issueMutationTaskHook{TaskRepository: r, arrived: make(chan struct{}), release: make(chan struct{})}
					return mergeGate
				})
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				task, err := h.repos[0].GetTask(ctx, "issue-task")
				require.NoError(t, err)
				task.Metadata = issueMutationMetadata(7)
				require.NoError(t, h.repos[0].UpdateTask(ctx, task))
				gh, _ := issueMutationAuthenticatedGitHub(t, h, issueMutationRemote)
				issueGate := &issueMutationReadGate{githubTaskIssueStoreAdapter: githubTaskIssueStoreAdapter{svc: h.services[0]}, arrived: make(chan struct{}), release: make(chan struct{})}
				gh.SetTaskIssueStore(issueGate)
				router := issueMutationRouter(t, h, gh)
				issueGate.armed.Store(true)
				mergeGate.armed.Store(true)
				result := [2]chan *httptest.ResponseRecorder{make(chan *httptest.ResponseRecorder, 1), make(chan *httptest.ResponseRecorder, 1)}
				joined := [2]bool{}
				defer func() {
					cancel()
					for i := range result {
						if !joined[i] {
							<-result[i]
						}
					}
				}()
				go func() {
					method, body := http.MethodPut, `{"owner":"acme","repo":"api","number":42}`
					if unlink {
						method, body = http.MethodDelete, ""
					}
					result[0] <- issueMutationHTTP(ctx, router, method, "/api/v1/github/tasks/issue-task/issue", body)
				}()
				go func() {
					result[1] <- issueMutationHTTP(ctx, router, http.MethodPatch, "/api/v1/tasks/issue-task/port-forwarding", `{"enabled":true}`)
				}()
				for _, arrived := range []chan struct{}{issueGate.arrived, mergeGate.arrived} {
					select {
					case <-arrived:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				}
				for _, i := range []int{first, 1 - first} {
					if i == 0 {
						close(issueGate.release)
					} else {
						close(mergeGate.release)
					}
					response := <-result[i]
					joined[i] = true
					require.Equal(t, 200, response.Code, response.Body.String())
				}
				stored, err := h.repos[0].GetTask(ctx, "issue-task")
				require.NoError(t, err)
				number := 42
				if unlink {
					number = 0
				}
				assertIssueMutationIdentity(t, stored.Metadata, number)
				require.Equal(t, true, stored.Metadata["port_forwarding_enabled"])
				require.Equal(t, "before", stored.Metadata["alpha"])
				require.Equal(t, "untouched", stored.Metadata["keep"])
			})
		}
	}
}
