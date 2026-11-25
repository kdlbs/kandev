package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func projectDeliveryClientRecord(instanceID string) *types.ProjectMCPDelivery {
	return &types.ProjectMCPDelivery{
		Version: types.ProjectMCPDeliveryVersion, Strategy: types.ProjectMCPDeliveryStrategyPi,
		Workspace: "/workspace", TaskID: "task-1", InstanceID: instanceID,
		BindingDigest: strings.Repeat("a", 64), Packages: []string{"core"},
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-003.5
func TestProjectMCPDeliveryRequiresHelperCapability(t *testing.T) {
	for _, tc := range []struct {
		name         string
		capabilities []string
		wantErr      bool
	}{
		{name: "old helper", capabilities: []string{"agent-survival.v1"}, wantErr: true},
		{name: "supported helper", capabilities: []string{types.ProjectMCPDeliveryCapability}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/identity", r.URL.Path)
				_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": tc.capabilities})
			}))
			t.Cleanup(server.Close)
			err := newHTTPOnlyClient(server.URL).RequireCapability(context.Background(), types.ProjectMCPDeliveryCapability)
			if tc.wantErr {
				require.ErrorContains(t, err, "does not support project-mcp-delivery-v1")
				return
			}
			require.NoError(t, err)
		})
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-003.5
func TestProjectMCPDeliveryCapabilityMissingIdentityEndpoint(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	err := newHTTPOnlyClient(server.URL).RequireCapability(context.Background(), types.ProjectMCPDeliveryCapability)
	require.ErrorContains(t, err, "update the execution runtime")
}

// @covers AC-AGENTS-BUILTIN-MCP-003.6
func TestConfigureAgentWithProjectDeliveryRoundTrip(t *testing.T) {
	server, captured := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))
	client := newHTTPOnlyClient(server.URL)
	delivery := projectDeliveryClientRecord("instance-1")

	require.NoError(t, client.ConfigureAgentWithProjectDelivery(
		context.Background(), "pi-acp", []string{"pi-acp"}, nil, "", nil, delivery))

	var sent struct {
		ProjectMCPDelivery *types.ProjectMCPDelivery `json:"project_mcp_delivery"`
	}
	require.NoError(t, json.Unmarshal(captured.Body, &sent))
	require.Equal(t, delivery, sent.ProjectMCPDelivery)

	require.NoError(t, client.ConfigureAgent(context.Background(), "pi-acp", nil, nil, "", nil))
	var absent map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(captured.Body, &absent))
	_, present := absent["project_mcp_delivery"]
	require.False(t, present, "a configure request without delivery evidence must omit the field")
}

func TestProjectMCPDeliveryCloneIsolation(t *testing.T) {
	delivery := projectDeliveryClientRecord("instance-1")
	clone := delivery.Clone()
	clone.Packages[0] = "diagnostics"
	require.Equal(t, []string{"core"}, delivery.Packages)
	require.Nil(t, (*types.ProjectMCPDelivery)(nil).Clone())
}
