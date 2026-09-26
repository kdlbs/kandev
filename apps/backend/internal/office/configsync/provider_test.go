package configsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestProvide_BuildsAWorkingService(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	_ = store // Provide builds its own Store over the same connection pool.

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)

	svc, cleanup, err := Provide(repo.Writer(), repo.Writer(), repo, nil, nil, log)
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.NotNil(t, cleanup)
	require.NoError(t, cleanup())

	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.Nil(t, cfg)
}
