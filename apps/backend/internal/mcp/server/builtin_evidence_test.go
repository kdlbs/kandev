package mcp

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinAttachmentEvidenceKeepsPackageIdentity(t *testing.T) {
	s := NewWithProfile(nil, "session", "task", 10005, newTestLogger(t), "", false, packageTestProfile(p.PackageCore, p.PackageDiagnostics))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	events := make(chan streams.MCPAttachmentEvidence, 32)
	s.SetAttachmentReporter(func(e streams.MCPAttachmentEvidence) { events <- e })
	s.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt"})
	require.Empty(t, events, "configuration alone is not connection evidence")
	router := gin.New()
	s.RegisterRoutes(router)
	response := builtinPackageRequest(t, router, "/mcp/packages/diagnostics", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	require.Equal(t, 200, response.Code)
	listed := false
	for len(events) > 0 {
		e := <-events
		require.Equal(t, "kandev-diagnostics", e.ServerName)
		require.Equal(t, "attempt", e.AttemptID)
		if e.Kind == streams.MCPAttachmentEvidenceToolsListObserved {
			listed = true
			require.Len(t, e.Tools, 1)
			require.Equal(t, "get_diagnostic_bundle_kandev", e.Tools[0].Name)
		}
	}
	require.True(t, listed, "the selected package must report its observed catalog")
}
