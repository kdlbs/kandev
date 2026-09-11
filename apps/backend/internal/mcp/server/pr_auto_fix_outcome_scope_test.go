package mcp

import (
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportPRAutoFixOutcomeToolDescriptionScopesCurrentTurn(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")

	tool, ok := s.mcpServer.ListTools()["report_pr_auto_fix_outcome_kandev"]
	require.True(t, ok, "report outcome tool must be registered")
	description := tool.Tool.Description

	assert.Contains(t, description, "current Kandev-dispatched auto-fix turn")
	assert.Contains(t, description, "server-owned outcome protocol")
	assert.Contains(t, description, "manual PR fixup")
	assert.Contains(t, description, "sibling review")
	assert.Contains(t, description, "earlier auto-fix turn")
	assert.Contains(t, description, "Tool availability or enabled automation settings alone do not establish an obligation to report")
	assert.Contains(t, description, "exactly once")
}

func TestReportPRAutoFixOutcomeMCPClientPreservesUnmatchedScope(t *testing.T) {
	const explanation = "No matching unresolved GitHub PR auto-fix attempt exists for this turn. Finish ordinary work without retrying this report or enabling auto-fix."
	backend := &testBackend{err: errors.New(explanation)}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "report_pr_auto_fix_outcome_kandev", map[string]interface{}{
		"outcome": "blocked",
		"summary": "provider is unavailable",
	})

	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, explanation)
}
