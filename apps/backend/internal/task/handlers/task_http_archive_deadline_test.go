package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/archivecascade"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

type slowHTTPArchiveRepo struct {
	httpArchiveOutcomeRepo
	once     sync.Once
	deadline time.Time
}

type archiveDeadlineWriter struct {
	gin.ResponseWriter
	deadline time.Time
}

func (w *archiveDeadlineWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return http.NewResponseController(w.ResponseWriter).SetWriteDeadline(deadline)
}

func (r *slowHTTPArchiveRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	r.once.Do(func() {
		r.deadline, _ = ctx.Deadline()
		// Model archive work that exceeds the server's ordinary write timeout.
		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	})
	return r.httpArchiveOutcomeRepo.GetTask(ctx, id)
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.42
func TestHTTPArchiveResponseOutlivesServerWriteTimeout(t *testing.T) {
	for _, callerBudget := range []time.Duration{0, 5 * time.Second} {
		t.Run(callerBudget.String(), func(t *testing.T) {
			repo := &slowHTTPArchiveRepo{httpArchiveOutcomeRepo: httpArchiveOutcomeRepo{
				task: &models.Task{ID: "task-1", WorkspaceID: "ws-1"},
			}}
			h := &TaskHandlers{
				handoffSvc: service.NewHandoffService(repo, nil, nil, nil, nil, nil),
				logger:     newTestLogger(t),
			}
			done := make(chan struct{})
			var callerDeadline, responseDeadline time.Time
			router := gin.New()
			router.DELETE("/tasks/:id", func(c *gin.Context) {
				defer close(done)
				writer := &archiveDeadlineWriter{ResponseWriter: c.Writer}
				c.Writer = writer
				if callerBudget > 0 {
					callerDeadline = time.Now().Add(callerBudget)
					ctx, cancel := context.WithDeadline(c.Request.Context(), callerDeadline)
					defer cancel()
					c.Request = c.Request.WithContext(ctx)
				}
				h.httpArchiveTask(c)
				responseDeadline = writer.deadline
			})
			server := httptest.NewUnstartedServer(router)
			server.Config.WriteTimeout = 250 * time.Millisecond
			server.Start()
			t.Cleanup(server.Close)
			client := server.Client()
			client.Timeout = 5 * time.Second
			started := time.Now()
			req, err := http.NewRequest(http.MethodDelete, server.URL+"/tasks/task-1", nil)
			require.NoError(t, err)
			resp, err := client.Do(req)
			require.NoError(t, err, "archive must return its result after the ordinary write timeout")
			defer func() { require.NoError(t, resp.Body.Close()) }()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.JSONEq(t, `{"success":true}`, string(body))
			<-done
			require.True(t, repo.archived)
			require.Equal(t, repo.deadline.Add(5*time.Second), responseDeadline,
				"response writes must have a finite five-second allowance")
			if callerBudget > 0 {
				require.Equal(t, callerDeadline, repo.deadline, "transport allowance must not extend work")
			} else {
				require.WithinDuration(t, started.Add(archivecascade.TaskArchiveTimeout), repo.deadline, time.Second)
			}
		})
	}
}
