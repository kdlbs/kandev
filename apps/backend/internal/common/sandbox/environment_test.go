package sandbox

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSandboxCredentialKeysRequireExplicitCeiling(t *testing.T) {
	grants, err := Canonicalize(Grants{EnvironmentKeys: []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"}}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY"}, grants.EnvironmentKeys)
	require.ErrorIs(t, WithinCeiling(grants, Grants{EnvironmentKeys: []string{"OPENAI_API_KEY"}}), ErrOutsideCeiling)
	require.NoError(t, WithinCeiling(Grants{EnvironmentKeys: []string{"OPENAI_API_KEY"}}, grants))
}

func TestSandboxEnvironmentCannotGrantControlOrInjectionKeys(t *testing.T) {
	for _, key := range []string{"*", "AWS_*", "HOME", "XDG_CONFIG_HOME", "TMPDIR", "PATH", "LD_PRELOAD", "DYLD_INSERT_LIBRARIES", "BASH_ENV", "ENV", "KANDEV_TOKEN", "NODE_OPTIONS", "PYTHONPATH", "GIT_CONFIG_COUNT", "lowercase", "A=B"} {
		_, err := Canonicalize(Grants{EnvironmentKeys: []string{key}}, nil)
		require.ErrorIs(t, err, ErrInvalidPolicy, key)
	}
}
