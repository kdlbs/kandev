package github

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPRBaseIdentityConversionsRetainBaseCommitOID(t *testing.T) {
	const want = "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name        string
		pr          *PR
		wantBaseSHA string
	}{
		{
			name:        "REST",
			wantBaseSHA: want,
			pr: func() *PR {
				var raw patPR
				if err := json.Unmarshal([]byte(`{"number":42,"head":{"ref":"feature","sha":"head","repo":{"full_name":"fork/widgets","clone_url":"https://github.com/fork/widgets.git","owner":{"login":"fork"},"name":"widgets"}},"base":{"ref":"release","sha":"`+want+`","repo":{"full_name":"upstream/widgets","clone_url":"https://github.com/upstream/widgets.git","owner":{"login":"upstream"},"name":"widgets"}}}`), &raw); err != nil {
					t.Fatalf("decode REST pull request: %v", err)
				}
				return convertPatPR(&raw, "upstream", "widgets")
			}(),
		},
		{
			name: "GH CLI",
			pr: func() *PR {
				var raw ghPR
				if err := json.Unmarshal([]byte(`{"number":42,"headRefName":"feature","headRefOid":"head","baseRefName":"release","headRepository":{"name":"widgets","nameWithOwner":"fork/widgets","url":"https://github.com/fork/widgets"},"headRepositoryOwner":{"login":"fork"}}`), &raw); err != nil {
					t.Fatalf("decode GH pull request: %v", err)
				}
				return convertGHPR(&raw, "upstream", "widgets")
			}(),
		},
		{
			name:        "GraphQL",
			wantBaseSHA: want,
			pr: func() *PR {
				var raw batchedPRResult
				if err := json.Unmarshal([]byte(`{"state":"OPEN","headRefName":"feature","headRefOid":"head","baseRefName":"release","baseRefOid":"`+want+`","headRepository":{"name":"widgets","nameWithOwner":"fork/widgets","url":"https://github.com/fork/widgets"},"headRepositoryOwner":{"login":"fork"}}`), &raw); err != nil {
					t.Fatalf("decode GraphQL pull request: %v", err)
				}
				return convertBatchedPRResult(&raw, "upstream", "widgets", 42).PR
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var values map[string]any
			encoded, err := json.Marshal(tt.pr)
			if err != nil {
				t.Fatalf("marshal PR: %v", err)
			}
			if err := json.Unmarshal(encoded, &values); err != nil {
				t.Fatalf("decode marshaled PR: %v", err)
			}
			if got, _ := values["base_sha"].(string); got != tt.wantBaseSHA {
				t.Fatalf("base_sha = %q, want %q", got, tt.wantBaseSHA)
			}
		})
	}
}

func TestPRBaseGraphQLQueryRequestsBaseRefOID(t *testing.T) {
	if !strings.Contains(prFieldsBlock(), "baseRefOid") {
		t.Fatalf("prFieldsBlock() = %q, want baseRefOid", prFieldsBlock())
	}
}
