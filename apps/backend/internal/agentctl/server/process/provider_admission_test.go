package process

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestMCPProviderReplacementAdmissionDoesNotCancelExistingWork(t *testing.T) {
	mgr := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	_, err := mgr.BeginProviderOperation("unreserved")
	require.Error(t, err)
	release, err := mgr.BeginProviderOperation("")
	require.NoError(t, err)
	require.Error(t, mgr.ReserveProviderReplacement("operation", time.Now().Add(time.Minute)))
	release()
	require.NoError(t, mgr.ReserveProviderReplacement("operation", time.Now().Add(time.Minute)))
	_, err = mgr.BeginProviderOperation("")
	require.Error(t, err)
	control, err := mgr.BeginProviderOperation("operation")
	require.NoError(t, err)
	control()
	workspace, err := mgr.admitStart()
	require.NoError(t, err)
	workspace()
	mgr.ReleaseProviderReplacement("foreign")
	_, err = mgr.BeginProviderOperation("")
	require.Error(t, err)
	mgr.ReleaseProviderReplacement("operation")
	release, err = mgr.BeginProviderOperation("")
	require.NoError(t, err)
	release()
}
