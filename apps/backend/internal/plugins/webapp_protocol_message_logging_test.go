package plugins

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type failingMessageTaskSource struct {
	taskDataSource
	err error
}

func (f failingMessageTaskSource) GetTask(context.Context, string) (*taskmodels.Task, error) {
	return nil, f.err
}

func TestSendWebAppMessage_InternalFailureLogging(t *testing.T) {
	const secret = "sensitive-prompt-and-error"
	for _, tc := range []struct {
		name       string
		phase      string
		code       codes.Code
		wantStatus int
		wantLog    bool
	}{
		{name: "scope unavailable", phase: "scope", code: codes.Unavailable, wantStatus: http.StatusInternalServerError, wantLog: true},
		{name: "send aborted", phase: "send", code: codes.Aborted, wantStatus: http.StatusInternalServerError, wantLog: true},
		{name: "send denied", phase: "send", code: codes.PermissionDenied, wantStatus: http.StatusForbidden},
		{name: "scope missing", phase: "scope", code: codes.NotFound, wantStatus: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log, observed := newObservedLogger(t)
			d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}, APIWrite: []string{"messages"}})
			d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": {ID: "task-1", WorkspaceID: "ws-1"}}
			failure := status.Error(tc.code, secret)
			if tc.phase == "scope" {
				d.host.taskData = failingMessageTaskSource{taskDataSource: d.tasks, err: failure}
			} else {
				d.messenger.err = failure
			}
			svc := &Service{log: log}
			binding := webapp.CapabilityBinding{ScopeKind: instances.ScopeWorkspace, WorkspaceID: "ws-1"}
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"text":"`+secret+`"}`))
			recorder := httptest.NewRecorder()

			svc.sendWebAppMessage(context.Background(), recorder, req, d.host, binding, "task-1")

			require.Equal(t, tc.wantStatus, recorder.Code)
			require.NotContains(t, recorder.Body.String(), secret)
			if !tc.wantLog {
				require.Empty(t, observed.All())
				return
			}
			entries := observed.All()
			require.Len(t, entries, 1)
			fields := entries[0].ContextMap()
			require.Equal(t, map[string]any{"phase": tc.phase, "grpc_code": tc.code.String()}, fields)
			require.NotContains(t, entries[0].Message, secret)
		})
	}
}

func TestSendWebAppMessage_InternalFailureWithoutLogger(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}, APIWrite: []string{"messages"}})
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": {ID: "task-1", WorkspaceID: "ws-1"}}
	d.messenger.err = status.Error(codes.Aborted, "private")
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"text":"hello"}`))
	svc := &Service{}
	svc.sendWebAppMessage(context.Background(), recorder, req, d.host, webapp.CapabilityBinding{ScopeKind: instances.ScopeWorkspace, WorkspaceID: "ws-1"}, "task-1")
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
}
