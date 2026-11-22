package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestFormatterCaptureDrainsWritesAndResumesAfterCancellation(t *testing.T) {
	svc := &Service{}
	finish, err := svc.BeginFormatterManagement(context.Background())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = svc.CaptureFormatterConfiguration(ctx)
	require.ErrorIs(t, err, context.Canceled)
	finish()
	release, err := svc.CaptureFormatterConfiguration(context.Background())
	require.NoError(t, err)
	_, err = svc.BeginFormatterManagement(context.Background())
	require.ErrorIs(t, err, ErrFormatterCapture)
	release()
	finish, err = svc.BeginFormatterManagement(context.Background())
	require.NoError(t, err)
	finish()
}

func TestFormatterCaptureLeaseIsOwnedAndReleasesTheGate(t *testing.T) {
	svc := &Service{}
	t.Cleanup(svc.CloseFormatterCaptures)
	lease, err := svc.StartFormatterCapture(context.Background(), "alice")
	require.NoError(t, err)
	require.NotEmpty(t, lease.ID)
	_, err = svc.BeginFormatterManagement(context.Background())
	require.ErrorIs(t, err, ErrFormatterCapture)
	require.ErrorIs(t, svc.ReleaseFormatterCapture("bob", lease.ID), ErrFormatterCaptureNotFound)
	require.NoError(t, svc.ReleaseFormatterCapture("alice", lease.ID))
	finish, err := svc.BeginFormatterManagement(context.Background())
	require.NoError(t, err)
	finish()
}

func TestCaptureFormatterRejectsBindingForChangedProviderType(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	repo := &formatterRepositoryFixture{notificationTestRepository: &notificationTestRepository{},
		binding: &models.FormatterBinding{ProviderID: "mail", ProviderType: models.ProviderTypeApprise, PluginID: "formatter"}}
	svc := NewService(repo, nil, nil, log, nil, false)
	svc.SetFormatterRuntime(&formatterRuntimeFixture{})
	require.Nil(t, svc.captureFormatter(t.Context(), "owner", &models.Provider{ID: "mail", Type: models.ProviderTypeLocal}))
}

func TestCaptureFormatterReportsLookupFailureWithoutRawError(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repo := &formatterRepositoryFixture{notificationTestRepository: &notificationTestRepository{}, bindingErr: errors.New("private database details")}
	svc := NewService(repo, nil, nil, log, nil, false)
	svc.SetFormatterRuntime(&formatterRuntimeFixture{})
	require.Nil(t, svc.captureFormatter(t.Context(), "owner", &models.Provider{ID: "mail", Type: models.ProviderTypeApprise}))
	require.Equal(t, 1, logs.Len())
	require.Equal(t, "binding_lookup_failed", logs.All()[0].ContextMap()["reason"])
	require.NotContains(t, logs.All()[0].Message, "private database details")
}
