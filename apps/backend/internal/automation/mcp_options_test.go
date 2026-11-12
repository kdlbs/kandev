package automation

import (
	"context"
	"encoding/json"
	"errors"
	ws "github.com/kandev/kandev/pkg/websocket"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/stretchr/testify/require"
)

type automationMCPOptionsCatalog struct {
	definitions []*mcpconfig.MCPServerDefinition
}

func (c automationMCPOptionsCatalog) ListMCPServerDefinitions(context.Context, string) ([]*mcpconfig.MCPServerDefinition, error) {
	return c.definitions, nil
}
func (c automationMCPOptionsCatalog) GetMCPServerDefinition(context.Context, string, string) (*mcpconfig.MCPServerDefinition, error) {
	return c.definitions[0], nil
}
func (c automationMCPOptionsCatalog) ListMCPSelections(context.Context, mcpconfig.SelectionScope, string, string) ([]string, error) {
	return []string{"server"}, nil
}

// @covers AC-OFFICE-AUTOMATION-MCP-001.2
func TestAutomationMCPOptionsPreserveOriginsWithoutConfiguration(t *testing.T) {
	svc := newTestService(t)
	svc.SetMCPSelectionReaders(automationMCPOptionsCatalog{definitions: []*mcpconfig.MCPServerDefinition{{ID: "server", WorkspaceID: "workspace", DisplayName: "Reports", Enabled: true, Configuration: mcpconfig.MCPServerConfiguration{URL: "https://private.example.test"}}}}, nil)
	svc.SetRepositoryLookup(&fakeRepositoryLookup{repos: map[string]string{"repo": "workspace", "foreign": "another"}})
	options, err := svc.mcpOptionsWithInheritance(t.Context(), "workspace", "profile", []string{"repo"})
	require.NoError(t, err)
	require.Len(t, options.Inherited, 1)
	require.ElementsMatch(t, []string{"profile", "repository"}, options.Inherited[0].Origins)
	encoded, err := json.Marshal(options)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private.example.test")
	require.Len(t, options.Packages, 5)
	_, err = svc.mcpOptionsWithInheritance(t.Context(), "workspace", "profile", []string{"foreign"})
	require.ErrorIs(t, err, ErrRepositoryNotInWorkspace)
}

func TestAutomationMCPOptionsRejectsOversizedRepositoryList(t *testing.T) {
	svc := newTestService(t)
	svc.SetMCPSelectionReaders(automationMCPOptionsCatalog{}, nil)
	_, err := svc.mcpOptionsWithInheritance(t.Context(), "workspace", "", make([]string, 101))
	require.ErrorContains(t, err, "too many repository IDs")
}

func TestAutomationMCPOptionsErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{{errors.New("database unavailable"), ws.ErrorCodeInternalError}, {ErrRepositoryNotInWorkspace, ws.ErrorCodeBadRequest}, {ErrAgentProfileNotFound, ws.ErrorCodeNotFound}} {
		t.Run(tc.code, func(t *testing.T) {
			svc := newTestService(t)
			svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return tc.err })
			raw, err := json.Marshal(map[string]any{"workspace_id": "workspace"})
			require.NoError(t, err)
			response, err := wsMCPOptions(svc)(t.Context(), &ws.Message{ID: "request", Action: "automation.mcp_options", Payload: raw})
			require.NoError(t, err)
			var payload ws.ErrorPayload
			require.NoError(t, json.Unmarshal(response.Payload, &payload))
			require.Equal(t, tc.code, payload.Code)
		})
	}
}
