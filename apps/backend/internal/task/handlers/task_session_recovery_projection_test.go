package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.22
// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.23
func TestHTTPFreshSessionRecoveryProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, h, repo := newQueuedTaskDTOBuilder(t)
	createQueuedTestTask(t, svc, repo, fakeQueuedPromptCounter{})
	h.repo = repo
	core, observed := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	h.logger = log

	first := getConditionalTaskSessionRecorder(t, h, "s1", "")
	require.Equal(t, http.StatusOK, first.Code)
	require.Contains(t, first.Body.String(), `"session_recovery_blocks":[]`)
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)
	for range 3 {
		unchanged := getConditionalTaskSessionRecorder(t, h, "s1", etag)
		require.Equal(t, http.StatusNotModified, unchanged.Code)
	}
	require.Empty(t, observed.All(), "fresh session reads must not warn about a missing generation")
	_, err = repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	missing := getConditionalTaskSessionRecorder(t, h, "missing", etag)
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.Empty(t, missing.Header().Get("ETag"))

	h.repo = &recoveryProjectionFailureRepo{Repository: repo, err: errors.New("generation read failed")}
	failedProjection := getConditionalTaskSessionRecorder(t, h, "s1", "")
	require.Equal(t, http.StatusOK, failedProjection.Code)
	entries := observed.FilterMessage("get task session recovery blocks failed").All()
	require.Len(t, entries, 1)
	require.Equal(t, "generation read failed", entries[0].ContextMap()["error"])
}

type recoveryProjectionFailureRepo struct {
	*taskrepo.Repository
	err error
}

func (r *recoveryProjectionFailureRepo) GetCurrentHarnessSessionGeneration(
	context.Context, string, string,
) (*models.HarnessSessionGeneration, error) {
	return nil, r.err
}
