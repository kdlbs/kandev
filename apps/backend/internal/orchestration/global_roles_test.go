package orchestration

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestGlobalRoleOwnsIdentityWhileAssignmentsKeepAccountsAndContext(t *testing.T) {
	router, _ := testHandler(t)
	rolePath := "/api/v1/orchestration/roles/chief-of-staff"
	role := map[string]string{"name": "Global chief", "icon": "💼", "instructions": "Delegate substantive work"}
	require.Equal(t, 200, request(t, router, http.MethodPut, rolePath, role).Code)
	var ids []string
	for _, workspace := range []string{"ws", "other"} {
		cfg := configuration{RoleID: "chief-of-staff", ProfileID: "personal", Context: workspace, Name: "Ignored local name", Instructions: "Ignored local policy", Icon: "🌱"}
		if workspace == "other" {
			cfg.ProfileID = "work"
		}
		result := request(t, router, http.MethodPost, "/api/v1/orchestration/workspaces/"+workspace+"/orchestrators", cfg)
		require.Equal(t, 201, result.Code, result.Body.String())
		var data map[string]any
		require.NoError(t, json.Unmarshal(result.Body.Bytes(), &data))
		ids = append(ids, data["id"].(string))
	}
	role["name"], role["icon"], role["instructions"] = "Chief revised", "🧭", "Updated global policy"
	require.Equal(t, 200, request(t, router, http.MethodPut, rolePath, role).Code)
	for i, workspace := range []string{"ws", "other"} {
		result := request(t, router, http.MethodGet, "/api/v1/orchestration/workspaces/"+workspace+"/orchestrators/"+ids[i], nil)
		var data map[string]any
		require.NoError(t, json.Unmarshal(result.Body.Bytes(), &data))
		require.Equal(t, "Chief revised", data["name"])
		require.Equal(t, "🧭", data["icon"])
		require.Equal(t, "Updated global policy", data["instructions"])
		require.Equal(t, workspace, data["context"])
		expected := "personal"
		if workspace == "other" {
			expected = "work"
		}
		require.Equal(t, expected, data["profile_id"])
	}
	role["icon"] = "invalid"
	require.Equal(t, 400, request(t, router, http.MethodPut, rolePath, role).Code)
}
