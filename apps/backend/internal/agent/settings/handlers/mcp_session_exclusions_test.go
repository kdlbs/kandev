package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
)

// @covers AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.1, AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.6
func TestMCPSessionExclusionRoundTrip(t *testing.T) {
	router, notifications := selectionRouter(t)
	inherited := performCatalogRequest(router, http.MethodPut, "/repositories/r", `{"workspace_id":"w","definition_ids":["docs"]}`, "")
	if inherited.Code != http.StatusOK {
		t.Fatal(inherited.Body.String())
	}
	saved := performCatalogRequest(router, http.MethodPut, "/sessions/s", `{"workspace_id":"w","definition_ids":["other"],"excluded_definition_ids":["docs"],"expected_revision":0}`, "")
	if saved.Code != http.StatusOK {
		t.Fatal(saved.Body.String())
	}
	var body struct {
		Excluded []string `json:"excluded_definition_ids"`
		IDs      []string `json:"definition_ids"`
		Revision int64    `json:"selection_revision"`
	}
	if err := json.Unmarshal(saved.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Excluded) != 1 || body.Excluded[0] != "docs" {
		t.Fatalf("exclusion missing from saved result: %s", saved.Body.String())
	}
	legacy := performCatalogRequest(router, http.MethodPut, "/sessions/s", `{"workspace_id":"w","definition_ids":["other"]}`, "")
	if legacy.Code != http.StatusOK {
		t.Fatal(legacy.Body.String())
	}
	read := performCatalogRequest(router, http.MethodGet, "/sessions/s?workspace_id=w", "", "")
	if err := json.Unmarshal(read.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Excluded) != 1 || body.Revision != 2 {
		t.Fatalf("legacy save lost exclusion: %s", read.Body.String())
	}
	restored := performCatalogRequest(router, http.MethodPut, "/sessions/s", `{"workspace_id":"w","excluded_definition_ids":[],"expected_revision":2}`, "")
	if restored.Code != http.StatusOK {
		t.Fatal(restored.Body.String())
	}
	if err := json.Unmarshal(restored.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Excluded) != 0 || len(body.IDs) != 1 || body.IDs[0] != "other" || body.Revision != 3 {
		t.Fatalf("restore must preserve additions: %s", restored.Body.String())
	}
	sibling := performCatalogRequest(router, http.MethodGet, "/sessions/sibling?workspace_id=w", "", "")
	if err := json.Unmarshal(sibling.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Excluded) != 0 || *notifications != 3 {
		t.Fatalf("session isolation/notification failed: %s count=%d", sibling.Body.String(), *notifications)
	}
}

func TestMCPSessionExclusionInvalidWrite(t *testing.T) {
	for _, tc := range []struct{ path, fields string }{
		{"/sessions/s", `"excluded_definition_ids":["docs"]`},
		{"/sessions/s", `"excluded_definition_ids":null,"expected_revision":0`},
		{"/sessions/s", `"excluded_definition_ids":["other"],"expected_revision":0`},
		{"/sessions/s", `"excluded_definition_ids":["missing"],"expected_revision":0`},
		{"/repositories/r", `"excluded_definition_ids":[],"expected_definition_ids":[]`},
	} {
		t.Run(tc.path+tc.fields, func(t *testing.T) {
			router, notifications := selectionRouter(t)
			response := performCatalogRequest(router, http.MethodPut, tc.path, `{"workspace_id":"w","definition_ids":["docs"],`+tc.fields+`}`, "")
			if response.Code != http.StatusBadRequest || *notifications != 0 {
				t.Fatalf("rejected exclusion mutated state: %d %s notifications=%d", response.Code, response.Body.String(), *notifications)
			}
		})
	}
}
