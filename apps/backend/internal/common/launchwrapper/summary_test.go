package launchwrapper

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLaunchWrapperSummaryOmitsOpaqueArgumentsAndPaths(t *testing.T) {
	secret := "private-value-do-not-project"
	spec := Profile([]string{"/private/operator/tools/wrapper", "--token=" + secret, WorkDir, ManagedNPMDir, "{{{kandev.secret}}}"})
	summary, err := spec.Summary()
	require.NoError(t, err)
	require.Equal(t, "wrapper", summary.Command.Executable)
	require.Equal(t, 2, summary.Command.OmittedArguments)
	require.Equal(t, []string{WorkDir, ManagedNPMDir}, summary.Command.References)
	require.Equal(t, SourceProfile, summary.Source)
	require.Len(t, summary.Digest, 64)
	raw, err := json.Marshal(summary)
	require.NoError(t, err)
	require.NotContains(t, string(raw), secret)
	require.NotContains(t, string(raw), "/private/operator")
	require.NotContains(t, string(raw), "kandev.secret")
	again, err := spec.Summary()
	require.NoError(t, err)
	require.Equal(t, summary, again)
	spec.Argv[1] = "changed"
	changed, err := spec.Summary()
	require.NoError(t, err)
	require.NotEqual(t, summary.Digest, changed.Digest)
}

func TestLaunchWrapperSummaryValidatesAndHandlesAbsentWrapper(t *testing.T) {
	absent, err := (Spec{}).Summary()
	require.NoError(t, err)
	require.Nil(t, absent)
	_, err = Profile([]string{"--invalid"}).Summary()
	require.ErrorIs(t, err, ErrInvalid)
	for _, command := range []string{"/private/path/", "unsafe\nidentity", "tool with spaces", "token=value"} {
		require.Empty(t, SummarizeCommand([]string{command, "opaque"}).Executable)
	}
	require.Equal(t, "agent.exe", SummarizeCommand([]string{`C:\private\agent.exe`}).Executable)
	require.Empty(t, SummarizeCommand(nil).Executable)
}
