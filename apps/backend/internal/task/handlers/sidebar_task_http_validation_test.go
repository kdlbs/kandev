package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.17
func TestHTTPQuerySidebarTasksStructuredValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body, reason string
		index              *int
		limit              int
	}{
		{name: "malformed", body: `{"unknown":"private value"}`, reason: "malformed_query"},
		{name: "dimension", body: `{"filters":[{"dimension":"private value","op":"is","value":"private value"}]}`, reason: "invalid_clause", index: new(int)},
		{name: "scalar", body: `{"filters":[{"dimension":"repository","op":"is","value":"` + strings.Repeat("s", 257) + `"}]}`, reason: "scalar_length", index: new(int), limit: 256},
		{name: "list", body: `{"filters":[{"dimension":"workflow","op":"in","value":[` + strings.Repeat(`"w",`, 1000) + `"w"]}]}`, reason: "list_count", index: new(int), limit: 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &httpTaskRepo{}
			c, rec := taskRequestAs(t, "", http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", "ws-b")
			c.Request.Body = io.NopCloser(strings.NewReader(tc.body))
			newHTTPTaskHandlers(t, repo).httpQuerySidebarTasks(c)
			require.Equal(t, 400, rec.Code)
			var body struct {
				Error   string `json:"error"`
				Code    string `json:"error_code"`
				Details struct {
					Reason string `json:"reason"`
					Index  *int   `json:"filter_index"`
					Limit  int    `json:"limit"`
				} `json:"details"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, "sidebar_query_invalid", body.Code)
			require.Equal(t, tc.reason, body.Details.Reason)
			require.Equal(t, tc.index, body.Details.Index)
			require.Equal(t, tc.limit, body.Details.Limit)
			require.NotContains(t, rec.Body.String(), "private value")
			require.Empty(t, repo.sidebarWorkspace)
		})
	}
}
