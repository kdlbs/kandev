package mcpconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCursorMCPAuthPrefersCredentialBearingSource(t *testing.T) {
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	if err := os.MkdirAll(projects, 0o700); err != nil {
		t.Fatal(err)
	}
	baseTime := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	writeCursorAuth(t, projects, "authenticated", `{"plugin-atlassian-atlassian":{"tokens":{"access_token":"old-access","refresh_token":"old-refresh"},"clientInfo":{"client_id":"old-client"},"unknown":{"kept":true}}}`, baseTime)
	writeCursorAuth(t, projects, "registration-only", `{"plugin-atlassian-atlassian":{"clientInfo":{"client_id":"new-client"}},"independent":{"clientInfo":{"client_id":"independent"}}}`, baseTime.Add(time.Minute))

	if err := AggregateCursorMCPAuth(cursorHome); err != nil {
		t.Fatalf("AggregateCursorMCPAuth: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode published auth: %v", err)
	}
	var selected map[string]any
	if err := json.Unmarshal(got["plugin-atlassian-atlassian"], &selected); err != nil {
		t.Fatalf("decode selected credential object: %v", err)
	}
	if selected["clientInfo"].(map[string]any)["client_id"] != "old-client" {
		t.Fatalf("selected client identity = %#v, want older credential-bearing object", selected["clientInfo"])
	}
	if _, exists := selected["tokens"]; !exists {
		t.Fatalf("selected credential object has no tokens: %#v", selected)
	}
	if _, exists := selected["unknown"]; !exists {
		t.Fatalf("selected credential object lost its unknown fields: %#v", selected)
	}
	if _, exists := got["independent"]; !exists {
		t.Fatalf("independent server was lost from aggregate: %s", data)
	}
}

// @covers AC-AGENTS-CURSOR-AUTH-001.2, AC-AGENTS-CURSOR-AUTH-003.1
func TestAggregateCursorMCPAuthCredentialRanking(t *testing.T) {
	baseTime := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		sources    []authSelectionFixture
		wantClient string
		wantToken  string
	}{
		{
			name: "access token beats newer registration-only object",
			sources: []authSelectionFixture{
				{project: "older", modified: baseTime, client: "access-client", access: `"access-only"`},
				{project: "newer", modified: baseTime.Add(time.Minute), client: "registration-client"},
			},
			wantClient: "access-client", wantToken: "access-only",
		},
		{
			name: "refresh token beats newer registration-only object",
			sources: []authSelectionFixture{
				{project: "older", modified: baseTime, client: "refresh-client", refresh: `"refresh-only"`},
				{project: "newer", modified: baseTime.Add(time.Minute), client: "registration-client"},
			},
			wantClient: "refresh-client", wantToken: "refresh-only",
		},
		{
			name: "whitespace and malformed token values remain registration-only",
			sources: []authSelectionFixture{
				{project: "older", modified: baseTime, client: "credential-client", access: `"access"`},
				{project: "newer", modified: baseTime.Add(time.Minute), client: "malformed-client", access: `"  \t "`, refresh: `17`},
			},
			wantClient: "credential-client", wantToken: "access",
		},
		{
			name: "newest registration-only object wins when all tokens are empty",
			sources: []authSelectionFixture{
				{project: "older", modified: baseTime, client: "older-registration", access: `""`, refresh: `null`},
				{project: "newer", modified: baseTime.Add(time.Minute), client: "newer-registration", access: `" "`, refresh: `""`},
			},
			wantClient: "newer-registration",
		},
		{
			name: "newest authenticated source wins among authenticated peers",
			sources: []authSelectionFixture{
				{project: "older", modified: baseTime, client: "older-authenticated", access: `"old"`},
				{project: "newer", modified: baseTime.Add(time.Minute), client: "newer-authenticated", refresh: `"new"`},
			},
			wantClient: "newer-authenticated", wantToken: "new",
		},
		{
			name: "path order breaks authenticated timestamp ties",
			sources: []authSelectionFixture{
				{project: "z-source", modified: baseTime, client: "z-authenticated", access: `"z"`},
				{project: "a-source", modified: baseTime, client: "a-authenticated", access: `"a"`},
			},
			wantClient: "a-authenticated", wantToken: "a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursorHome := t.TempDir()
			projects := filepath.Join(cursorHome, "projects")
			if err := os.MkdirAll(projects, 0o700); err != nil {
				t.Fatal(err)
			}
			for _, source := range tt.sources {
				writeCursorAuth(t, projects, source.project, source.data(), source.modified)
			}
			if err := AggregateCursorMCPAuth(cursorHome); err != nil {
				t.Fatalf("AggregateCursorMCPAuth: %v", err)
			}
			var got map[string]json.RawMessage
			data, err := os.ReadFile(filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("decode published auth: %v", err)
			}
			var selected struct {
				Tokens struct {
					AccessToken  string `json:"access_token"`
					RefreshToken string `json:"refresh_token"`
				} `json:"tokens"`
				ClientInfo struct {
					ClientID string `json:"client_id"`
				} `json:"clientInfo"`
			}
			if err := json.Unmarshal(got["plugin-atlassian-atlassian"], &selected); err != nil {
				t.Fatalf("decode selected credential object: %v", err)
			}
			if selected.ClientInfo.ClientID != tt.wantClient {
				t.Fatalf("selected client identity = %q, want %q", selected.ClientInfo.ClientID, tt.wantClient)
			}
			gotToken := selected.Tokens.AccessToken + selected.Tokens.RefreshToken
			if tt.wantToken != "" && gotToken != tt.wantToken {
				t.Fatalf("selected token = %q, want %q", gotToken, tt.wantToken)
			}
		})
	}
}

type authSelectionFixture struct {
	project  string
	modified time.Time
	client   string
	access   string
	refresh  string
}

func (f authSelectionFixture) data() string {
	tokens := make(map[string]json.RawMessage)
	if f.access != "" {
		tokens["access_token"] = json.RawMessage(f.access)
	}
	if f.refresh != "" {
		tokens["refresh_token"] = json.RawMessage(f.refresh)
	}
	data, _ := json.Marshal(map[string]any{
		"plugin-atlassian-atlassian": map[string]any{
			"tokens":     tokens,
			"clientInfo": map[string]string{"client_id": f.client},
			"opaque":     map[string]bool{"preserved": true},
		},
	})
	return string(data)
}
