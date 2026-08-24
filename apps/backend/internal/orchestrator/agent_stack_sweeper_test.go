package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentStackSweeper_JoinsWorkersAndRefusesAfterStop(t *testing.T) {
	sweeper := newAgentStackSweeper()
	require.False(t, sweeper.spawn(func(context.Context) {}), "spawn before start must be refused")

	sweeper.start(context.Background())
	release := make(chan struct{})
	workerEntered := make(chan struct{})
	stopObserved := make(chan struct{})
	require.True(t, sweeper.spawn(func(ctx context.Context) {
		close(workerEntered)
		<-ctx.Done()
		close(stopObserved)
		<-release
	}))
	<-workerEntered

	stopped := make(chan struct{})
	go func() {
		sweeper.stop()
		close(stopped)
	}()
	<-stopObserved

	select {
	case <-stopped:
		t.Fatal("sweeper.stop returned while its worker was still running")
	default:
	}

	close(release)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("sweeper.stop did not join its worker")
	}

	require.False(t, sweeper.spawn(func(context.Context) {}), "spawn after stop must be refused")
}

func TestAgentStackSweeper_CancelsSweepContextOnStop(t *testing.T) {
	sweeper := newAgentStackSweeper()
	sweeper.start(context.Background())

	observed := make(chan error, 1)
	require.True(t, sweeper.spawn(func(ctx context.Context) {
		<-ctx.Done()
		observed <- ctx.Err()
	}))

	sweeper.stop()

	select {
	case err := <-observed:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("sweep context was not cancelled by stop")
	}
}
