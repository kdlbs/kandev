package api

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type initializeProtocolAdapter struct {
	bareAgentAdapter
	name        string
	version     uint16
	onAgentInfo func()
}

func (a *initializeProtocolAdapter) GetAgentInfo() *adapter.AgentInfo {
	if a.onAgentInfo != nil {
		a.onAgentInfo()
	}
	return &adapter.AgentInfo{Name: a.name}
}

func (a *initializeProtocolAdapter) ACPProtocolVersion() uint16 { return a.version }

// @covers AC-AGENTS-ACP-V2-004.2
func TestWSInitializeReportsProtocolFromInitializedAdapter(t *testing.T) {
	server := newTestServer(t)
	replacement := &initializeProtocolAdapter{name: "replacement", version: 1}
	server.procMgr.SetAdapterForTest(&initializeProtocolAdapter{
		name: "original", version: 2,
		onAgentInfo: func() { server.procMgr.SetAdapterForTest(replacement) },
	})
	request, err := ws.NewRequest("initialize", "agent.initialize", InitializeRequest{ClientName: "test"})
	require.NoError(t, err)

	response := server.handleWSInitialize(t.Context(), request)
	var result InitializeResponse
	require.NoError(t, response.ParsePayload(&result))
	require.True(t, result.Success)
	require.NotNil(t, result.AgentInfo)
	require.Equal(t, "original", result.AgentInfo.Name)
	require.Same(t, replacement, server.procMgr.GetAdapter())
	require.EqualValues(t, 2, result.ACPProtocolVersion)
}
