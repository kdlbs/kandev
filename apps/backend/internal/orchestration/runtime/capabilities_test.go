package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

type syntheticCapabilities struct {
	generation string
	count      int
	query      models.CapabilityQuery
	identity   string
	err        error
}

func (s *syntheticCapabilities) ReadCapabilities(ctx context.Context, q models.CapabilityQuery) (models.CapabilityPage, error) {
	s.query = q
	identity, _ := authn.IdentityFromContext(ctx)
	s.identity = identity.UserID
	if s.err != nil {
		return models.CapabilityPage{}, s.err
	}
	if q.Generation != "" && q.Generation != s.generation {
		return models.CapabilityPage{}, models.ErrCapabilityGeneration
	}
	p := models.CapabilityPage{Entries: []models.Capability{}, Generation: s.generation}
	for i := 0; i < s.count; i++ {
		id := fmt.Sprintf("native/%03d", i)
		if id <= q.After {
			continue
		}
		if len(p.Entries) == q.Limit {
			p.After = p.Entries[len(p.Entries)-1].ID
			break
		}
		p.Entries = append(p.Entries, models.Capability{ID: id, Kind: "native", Health: "ready", WorkspaceID: q.WorkspaceID})
	}
	return p, nil
}

func TestAssistantCapabilitiesPaginationAndHealth(t *testing.T) {
	s, token, run, _ := assistantContextFixture(t)
	reader := &syntheticCapabilities{generation: "one", count: 205}
	s.Capabilities = reader
	router := assistantRuntimeRouter(s)
	path := "/api/v1/orchestration/runtime/capabilities"
	response := runtimeRequest(t, router, "GET", path, token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var page models.CapabilityPage
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Len(t, page.Entries, 50)
	require.NotEmpty(t, page.NextCursor)
	require.Equal(t, "owner", reader.identity)
	require.Equal(t, "ws", reader.query.WorkspaceID)
	response = runtimeRequest(t, router, "GET", path+"?limit=1000&after="+page.NextCursor, token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Len(t, page.Entries, 100)
	require.Equal(t, "native/050", page.Entries[0].ID)
	for _, query := range []string{"?after=bad", "?limit=0", "?kind=secret", "?kind=plugin&after=" + page.NextCursor} {
		require.Equal(t, 400, runtimeRequest(t, router, "GET", path+query, token, run, nil).Code)
	}
	reader.count = 0
	response = runtimeRequest(t, router, "GET", path, token, run, nil)
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), `"entries":[]`)
}

func TestAssistantCapabilitiesRevocation(t *testing.T) {
	s, _, _, _ := assistantContextFixture(t)
	reader := &syntheticCapabilities{generation: "one", count: 101}
	s.Capabilities = reader
	router := assistantRouter(s)
	path := "/api/v1/orchestration/assistant/capabilities"
	response := runtimeRequest(t, router, "GET", path, "", "", nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var page models.CapabilityPage
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	reader.generation = "two"
	require.Equal(t, 409, runtimeRequest(t, router, "GET", path+"?after="+page.NextCursor, "", "", nil).Code)
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "GET", path, "", "", nil).Code)
	reader.err = fmt.Errorf("SYNTHETIC_CONFIGURATION_SECRET")
	response = runtimeRequest(t, router, "GET", path, "", "", nil)
	require.Equal(t, 503, response.Code)
	require.NotContains(t, response.Body.String(), "SYNTHETIC_CONFIGURATION_SECRET")
}
