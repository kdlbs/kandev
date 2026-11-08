package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator/mcpcontract"
)

func callWith(args map[string]interface{}) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

// Every proposal tool forwards note_ids untouched, whatever its type, so the
// coordinator service alone judges it.
func TestProposalToolsForwardNoteIDs(t *testing.T) {
	notes := []interface{}{"11111111-1111-1111-1111-111111111111"}
	cases := map[string]struct {
		action  string
		handler func(s *Server) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		args    map[string]interface{}
	}{
		"task": {mcpcontract.ActionProposeTask, func(s *Server) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.proposeTaskHandler()
		}, map[string]interface{}{"title": "t", "description": "d", "rationale": "r", "workflow_id": "w"}},
		"resume": {mcpcontract.ActionProposeResume, func(s *Server) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.proposeKindHandler(mcpcontract.ActionProposeResume, "task_id")
		}, map[string]interface{}{"task_id": "x"}},
		"answer": {mcpcontract.ActionProposeAnswer, func(s *Server) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.proposeAnswerHandler()
		}, map[string]interface{}{"task_id": "x"}},
		"merge": {mcpcontract.ActionProposeMerge, func(s *Server) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.proposeMergeHandler()
		}, map[string]interface{}{"task_id": "x"}},
		"improvement": {mcpcontract.ActionProposeImprovement, func(s *Server) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.proposeImprovementHandler()
		}, map[string]interface{}{"title": "t"}},
	}
	for name, tc := range cases {
		for label, value := range map[string]interface{}{"list": notes, "scalar": "oops"} {
			backend := &testBackend{response: map[string]interface{}{}}
			s := &Server{backend: backend}
			args := map[string]interface{}{noteIDsArg: value}
			for k, v := range tc.args {
				args[k] = v
			}
			_, err := tc.handler(s)(context.Background(), callWith(args))
			require.NoError(t, err, name)
			assert.Equal(t, tc.action, backend.lastAction, name)
			payload, ok := backend.lastPayload.(map[string]interface{})
			require.True(t, ok, name)
			assert.Equal(t, value, payload[noteIDsArg], name+"/"+label)
		}
	}
}

func TestProposalToolsOmitNoteIDsWhenAbsent(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{}}
	s := &Server{backend: backend}
	_, err := s.proposeKindHandler(mcpcontract.ActionProposeMove, "task_id")(context.Background(), callWith(map[string]interface{}{"task_id": "x"}))
	require.NoError(t, err)
	_, present := backend.lastPayload.(map[string]interface{})[noteIDsArg]
	assert.False(t, present)
}
