package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/githubhost"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.5
func TestEnterpriseHostOptionalCapabilitiesDoNotBreakCorePRRead(t *testing.T) {
	host, err := githubhost.Parse("git.example.com")
	if err != nil {
		t.Fatal(err)
	}
	client := NewPATClientForHost("secret", host)
	calls := 0
	client.httpClient.Transport = enterpriseTransportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var request struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		body := `{"data":{"repo0":{"pr0":{"state":"OPEN","title":"core PR","url":"https://git.example.com/acme/widget/pull/7","headRefName":"feature","baseRefName":"main","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z","reviews":{"nodes":[]},"commits":{"nodes":[]}}}}}`
		if strings.Contains(request.Query, "mergeQueueEntry") || strings.Contains(request.Query, "REMOVED_FROM_MERGE_QUEUE_EVENT") {
			body = `{"errors":[{"message":"Field mergeQueueEntry is not defined on PullRequest","type":"GRAPHQL_VALIDATION_FAILED"}]}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	statuses, err := runBatchedPRQuery(context.Background(), client, []graphQLPRRef{{Owner: "acme", Repo: "widget", Number: 7}})
	if err != nil {
		t.Fatalf("optional provider capability broke core read: %v", err)
	}
	if statuses["acme/widget#7"] == nil || statuses["acme/widget#7"].PR.Title != "core PR" {
		t.Fatalf("core PR missing: %+v", statuses)
	}
	if statuses["acme/widget#7"].mergeQueuePopulated || statuses["acme/widget#7"].mergeQueueRecoveryPopulated {
		t.Fatal("unsupported queue fields became authoritative empty observations")
	}
	if calls != 1 {
		t.Fatalf("core read consumed %d queries", calls)
	}
}
