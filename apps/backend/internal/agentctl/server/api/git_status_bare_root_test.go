package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.26
func TestHandleGitStatusDetailsWaitBareRootDoesNotLogFailure(t *testing.T) {
	server, _ := newMultiRepoStatusServer(t)
	core, observed := observer.New(zapcore.ErrorLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	server.logger = log

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/git/status?fresh=true&details=wait", nil)
	server.Router().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var result GitStatusResult
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.False(t, result.Success)
	require.Equal(t, "unavailable", result.StatusState)
	require.Equal(t, "repository_unavailable", result.ErrorCode)
	require.Empty(t, observed.FilterMessage("git status failed").All())
}
