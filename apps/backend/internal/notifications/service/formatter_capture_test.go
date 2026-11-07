package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
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
