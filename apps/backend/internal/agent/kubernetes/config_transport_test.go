package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func transportExecutorConfigValues(transport string) map[string]string {
	values := map[string]string{
		"auth_mode":               "in_cluster",
		"namespace":               "kandev-agents",
		"request_timeout_seconds": "30",
	}
	if transport != "" {
		values["transport"] = transport
	}
	return values
}

func TestParseExecutorConfigDefaultsTransportToPortForward(t *testing.T) {
	config, err := ParseExecutorConfig(transportExecutorConfigValues(""))
	require.NoError(t, err)
	require.Equal(t, StreamTransport(""), config.Transport)
	require.Equal(t, StreamTransportPortForward, config.EffectiveTransport())
}

func TestParseExecutorConfigAcceptsKnownTransports(t *testing.T) {
	for _, transport := range []StreamTransport{StreamTransportPortForward, StreamTransportExec} {
		config, err := ParseExecutorConfig(transportExecutorConfigValues(" " + string(transport) + " "))
		require.NoError(t, err)
		require.Equal(t, transport, config.EffectiveTransport())
	}
}

func TestParseExecutorConfigRejectsUnknownTransport(t *testing.T) {
	_, err := ParseExecutorConfig(transportExecutorConfigValues("ssh"))
	var fieldErr *FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "config.transport", fieldErr.Path)
}
