package process

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestProviderReplacementControlFencesConfigurationAndStop(t *testing.T) {
	mgr := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	startRestartManager(t, mgr)
	control, ok := any(mgr).(interface {
		ProviderReplacementSource() (types.ProviderReplacementSource, error)
		RetireProviderReplacement(context.Context, types.ProviderReplacementSource) error
	})
	require.True(t, ok, "replacement control must validate the complete source tuple")
	source, err := control.ProviderReplacementSource()
	require.NoError(t, err)
	mgr.startMu.Lock()
	mgr.configRevision++
	mgr.startMu.Unlock()
	require.Error(t, control.RetireProviderReplacement(context.Background(), source))
	require.Equal(t, StatusRunning, mgr.Status(), "a settings/configuration race must leave the source running")
	source, err = control.ProviderReplacementSource()
	require.NoError(t, err)
	require.NoError(t, control.RetireProviderReplacement(context.Background(), source))
	require.Equal(t, StatusStopped, mgr.Status())
	require.Error(t, control.RetireProviderReplacement(context.Background(), source), "the control operation must reconcile its receipt before repeating a stop")
}

func TestProviderTerminationRequiresCompletedReaping(t *testing.T) {
	mgr := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	mgr.status.Store(StatusStopped)
	mgr.mainReapPending.Store(true)
	require.False(t, mgr.ProviderTerminationConfirmed(), "stopped status alone cannot prove that the process group exited")
	mgr.mainReapPending.Store(false)
	require.True(t, mgr.ProviderTerminationConfirmed())
}
