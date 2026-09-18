package runtime

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantMemorySourceOwnerAndRuntimeBoundary(t *testing.T) {
	s, token, run, _ := assistantContextFixture(t)
	human, agent := assistantRouter(s), assistantRuntimeRouter(s)
	path := "/api/v1/orchestration/assistant/memory/source-view"
	saved := runtimeRequest(t, human, "PUT", path, "", "", map[string]any{"key": "preference", "content": "Show short summaries", "scope": "workspace", "source_comment_id": "source", "confirmed": true})
	require.Equal(t, 200, saved.Code, saved.Body.String())
	source := runtimeRequest(t, human, "GET", path+"/source", "", "", nil)
	require.Equal(t, 200, source.Code, source.Body.String())
	require.Contains(t, source.Body.String(), "Inspect my staging setup")
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "GET", path+"/source", "", "", nil).Code)
	require.Equal(t, 403, runtimeRequest(t, agent, "GET", path+"/source", token, run, nil).Code)
	require.Equal(t, 404, runtimeRequest(t, human, "GET", "/api/v1/orchestration/assistant/memory/missing/source", "", "", nil).Code)
}
