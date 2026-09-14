package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicRemoteStatusErrorDoesNotExposeProviderDetails(t *testing.T) {
	require.Empty(t, publicRemoteStatusError(""))
	require.Equal(
		t,
		remoteStatusUnavailable,
		publicRemoteStatusError("request failed at /home/operator/.kube/config?token=secret"),
	)
}
