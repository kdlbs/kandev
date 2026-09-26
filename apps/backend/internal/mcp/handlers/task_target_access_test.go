package handlers

import (
	"context"
	"testing"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
)

func TestCanDirectParentAccess(t *testing.T) {
	parent := &models.Task{ID: "parent", WorkspaceID: "workspace"}
	for _, tt := range []struct {
		name   string
		target *models.Task
		want   bool
	}{
		{name: "direct child", target: &models.Task{ParentID: "parent", WorkspaceID: "workspace"}, want: true},
		{name: "self", target: parent, want: false},
		{name: "sibling", target: &models.Task{ParentID: "other", WorkspaceID: "workspace"}, want: false},
		{name: "grandchild", target: &models.Task{ParentID: "child", WorkspaceID: "workspace"}, want: false},
		{name: "other workspace", target: &models.Task{ParentID: "parent", WorkspaceID: "other"}, want: false},
		{name: "empty workspace", target: &models.Task{ParentID: "parent"}, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, canDirectParentAccess(parent, tt.target))
		})
	}
}

func TestCanonicalMCPCaller(t *testing.T) {
	scoped := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		CallerTaskID: "actual-task", CallerSessionID: "actual-session",
	})
	for _, tt := range []struct {
		name                        string
		ctx                         context.Context
		payloadTask, payloadSession string
		wantTask, wantSession       string
		wantOK                      bool
	}{
		{name: "unscoped compatibility", ctx: context.Background(), payloadTask: "payload-task", payloadSession: "payload-session", wantTask: "payload-task", wantSession: "payload-session", wantOK: true},
		{name: "scoped matching payload", ctx: scoped, payloadTask: "actual-task", payloadSession: "actual-session", wantTask: "actual-task", wantSession: "actual-session", wantOK: true},
		{name: "scoped omitted payload", ctx: scoped, wantTask: "actual-task", wantSession: "actual-session", wantOK: true},
		{name: "spoofed task", ctx: scoped, payloadTask: "other-task", payloadSession: "actual-session", wantOK: false},
		{name: "spoofed session", ctx: scoped, payloadTask: "actual-task", payloadSession: "other-session", wantOK: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			taskID, sessionID, ok := canonicalMCPCaller(tt.ctx, tt.payloadTask, tt.payloadSession)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantTask, taskID)
			assert.Equal(t, tt.wantSession, sessionID)
		})
	}
}
