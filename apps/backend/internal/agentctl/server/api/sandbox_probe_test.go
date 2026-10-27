package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestHostProbeFailsClosedAndReports(t *testing.T) {
	var reported []error
	report := func(err error) { reported = append(reported, err) }
	require.False(t, runHostProbe(func(context.Context) error { panic("probe bug") }, report))
	require.False(t, runHostProbe(func(context.Context) error { return errors.New("denied") }, report))
	require.Len(t, reported, 2)
	require.True(t, runHostProbe(func(context.Context) error { return nil }, report))
	require.Len(t, reported, 2)
}

func TestHostProbeIsBoundedByFiveSeconds(t *testing.T) {
	var deadline bool
	require.True(t, runHostProbe(func(ctx context.Context) error {
		d, ok := ctx.Deadline()
		deadline = ok && time.Until(d) <= hostProbeTimeout
		return nil
	}, func(error) {}))
	require.True(t, deadline)
	require.Equal(t, 5, int(hostProbeTimeout.Seconds()))
}

func TestExecutionCapabilitiesByHost(t *testing.T) {
	require.Contains(t, executionCapabilities("linux", false), sandbox.LaunchCapability)
	require.NotContains(t, executionCapabilities("darwin", false), sandbox.LaunchCapability)
	require.Contains(t, executionCapabilities("darwin", true), sandbox.LaunchCapability)
	require.NotContains(t, executionCapabilities("windows", true), sandbox.LaunchCapability)
	require.Contains(t, executionCapabilities("darwin", false), "agent-survival.v1")
}
