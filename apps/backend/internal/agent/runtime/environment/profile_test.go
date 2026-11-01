package environment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestGenericWrapperProfileEnvironment(t *testing.T) {
	values := []models.ProfileEnvVar{{Key: "TOKEN", SecretID: "secret"}, {Key: "OVERRIDE", Value: "profile"}}
	defaults := map[string]string{"TOKEN": "default", "OVERRIDE": "default", "DEFAULT": "retained"}
	got, err := ResolveProfile(context.Background(), values, defaults, func(_ context.Context, d Definition) (string, error) {
		require.Equal(t, "secret", d.SecretID)
		require.Equal(t, OriginAgentProfile, d.Origin)
		return "resolved", nil
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"TOKEN": "resolved", "OVERRIDE": "profile", "DEFAULT": "retained"}, got)
	_, err = ResolveProfile(context.Background(), values, defaults, nil)
	require.Error(t, err, "unavailable secrets must stop dispatch")
}
