package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestProviderReplacementClientChecksOperationIdentity(t *testing.T) {
	source := types.ProviderReplacementSource{ExecutionID: "execution", TaskID: "task", SessionID: "session", HelperID: "helper", ProcessGeneration: 1, ConfigurationRevision: 2, StreamID: "source-stream", HarnessGeneration: 1, NativeSessionID: "native"}
	for _, scenario := range []string{"matching", "foreign-operation", "foreign-source", "unsupported"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer control", r.Header.Get("Authorization"))
				require.Equal(t, "1", r.Header.Get(types.ProviderMCPReplacementHeader))
				require.Equal(t, "/api/v1/agent/provider-replacement", r.URL.Path)
				if scenario == "unsupported" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				var request types.ProviderReplacementRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				require.Equal(t, source, request.Source)
				response := types.ProviderReplacementStatus{OperationID: request.OperationID, Phase: "prepared", Version: 1, Source: source}
				if scenario == "foreign-operation" {
					response.OperationID = "other"
				}
				if scenario == "foreign-source" {
					response.Source.HelperID = "other"
				}
				require.NoError(t, json.NewEncoder(w).Encode(response))
			}))
			defer server.Close()
			host, port := splitTestServerHostPort(t, server)
			client := NewClient(host, port, newTestLogger(), WithAuthToken("control"), WithExecutionID("execution"))
			replacement, ok := any(client).(interface {
				ProviderReplacement(context.Context, types.ProviderReplacementRequest) (types.ProviderReplacementStatus, error)
			})
			require.True(t, ok, "runtime client must expose the fenced control protocol")
			status, err := replacement.ProviderReplacement(context.Background(), types.ProviderReplacementRequest{Action: types.ProviderReplacementActionPrepare, OperationID: "operation", Source: source})
			if scenario == "matching" {
				require.NoError(t, err)
				require.Equal(t, "operation", status.OperationID)
			} else {
				require.Error(t, err)
			}
		})
	}
}
