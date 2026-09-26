package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkflowExportTool_RegisteredOnlyOnConfigurationSurfaces(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	for _, mode := range []string{ModeConfig, ModeExternal} {
		t.Run(mode, func(t *testing.T) {
			s := New(backend, "test-session", "test-task", 10005, log, "", true, mode)
			assert.Contains(t, s.mcpServer.ListTools(), "export_workflow_kandev")
		})
	}
	for _, mode := range []string{ModeTask, ModeOffice} {
		t.Run(mode, func(t *testing.T) {
			s := New(backend, "test-session", "test-task", 10005, log, "", true, mode)
			assert.NotContains(t, s.mcpServer.ListTools(), "export_workflow_kandev")
		})
	}
}
