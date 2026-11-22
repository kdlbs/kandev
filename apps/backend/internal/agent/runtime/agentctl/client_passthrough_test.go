package client

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemotePassthroughClientCapabilityAndSafeErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"old helper", http.StatusNotFound, "private-server-text", ErrPassthroughUnsupported},
		{"future protocol", http.StatusOK, `{"version":2}`, ErrPassthroughUnsupported},
		{"ownership conflict", http.StatusConflict, "private-server-text", ErrPassthroughConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := captureServer(t, jsonResponder(tc.status, tc.body))
			_, err := newHTTPOnlyClient(server.URL).GetPassthrough(context.Background())
			require.True(t, errors.Is(err, tc.want), err)
			require.NotContains(t, err.Error(), "private-server-text")
		})
	}
}
