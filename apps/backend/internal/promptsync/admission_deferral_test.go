package promptsync

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
)

func TestAutomaticRunDeferredByGitHubAdmissionIsNotAFailure(t *testing.T) {
	svc, gh, _ := newGitHubFixture(t)
	ctx := context.Background()
	gh.listErr = &github.AdmissionDeferredError{Delay: time.Minute, Reason: "test background pacing"}

	svc.SyncIfDue(ctx)

	cfg, err := svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, cfg.ConsecutiveFailures)
	assert.False(t, cfg.PollSuspended)
	assert.Empty(t, cfg.LastError)
	assert.Nil(t, cfg.NextAttemptAt)

	svc.SyncIfDue(ctx)
	assert.Equal(t, 2, gh.lists, "the configuration stays due, so the next tick retries the run")
}

func TestManualRunDeferredByGitHubAdmissionStillRecordsAFailure(t *testing.T) {
	svc, gh, _ := newGitHubFixture(t)
	ctx := context.Background()
	gh.listErr = &github.AdmissionDeferredError{Delay: time.Minute, Reason: "test interactive pacing"}

	_, err := svc.Sync(ctx)
	require.Error(t, err)

	cfg, err := svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, cfg.ConsecutiveFailures)
}
