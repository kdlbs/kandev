package client

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestGenericWrapperRequiresPeerCapability(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(strconv.FormatBool(supported), func(t *testing.T) {
			dispatched := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					capabilities := []string{}
					if supported {
						capabilities = append(capabilities, launchwrapper.Capability)
					}
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"capabilities": capabilities}))
					return
				}
				dispatched++
				require.NoError(t, json.NewEncoder(w).Encode(utility.PromptResponse{Success: true}))
			}))
			defer server.Close()
			host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portText)
			require.NoError(t, err)
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
			require.NoError(t, err)
			client := NewClient(host, port, log)
			cfg := &utility.InferenceConfigDTO{LaunchWrapper: launchwrapper.Profile([]string{"wrapper"})}
			_, err = client.InferencePrompt(context.Background(), &utility.PromptRequest{InferenceConfig: cfg})
			if supported {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			_, err = client.Probe(context.Background(), &utility.ProbeRequest{InferenceConfig: cfg})
			if supported {
				require.NoError(t, err)
				require.Equal(t, 2, dispatched)
			} else {
				require.Error(t, err)
				require.Zero(t, dispatched)
			}
		})
	}
}
