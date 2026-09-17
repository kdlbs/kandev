package channels

import (
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkspaceSetupRequiresHuman(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&ChannelService{})
	for _, handler := range []gin.HandlerFunc{h.setWorkspaceChief, h.enableWorkspaceAgents, h.openConversation} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		c.Set("agent_caller", &models.AgentInstance{ID: "worker"})
		handler(c)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status=%d", w.Code)
		}
	}
}
