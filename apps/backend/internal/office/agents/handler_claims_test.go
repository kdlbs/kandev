package agents

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// AC-OFFICE-AGENT-COMMENT-READS-005.5: the caller task identity must come
// from validated JWT claims alone. ClaimsFromContext is the only exported
// reader of those claims outside this package.
func TestClaimsFromContext_ReturnsNilForUIRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	if got := ClaimsFromContext(c); got != nil {
		t.Fatalf("ClaimsFromContext = %+v, want nil for a request with no validated claims", got)
	}
}

func TestClaimsFromContext_ReturnsSetClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	want := &AgentClaims{AgentProfileID: "agent-1", TaskID: "task-42", WorkspaceID: "ws-1"}
	c.Set(ctxKeyAgentClaims, want)

	got := ClaimsFromContext(c)
	if got != want {
		t.Fatalf("ClaimsFromContext = %+v, want %+v", got, want)
	}
}
