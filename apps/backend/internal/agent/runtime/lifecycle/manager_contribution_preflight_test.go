package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

func TestContributionResumePreflight(t *testing.T) {
	tests := []struct {
		name      string
		resumed   bool
		wantError bool
	}{
		{name: "cold resume", resumed: true},
		{name: "initial launch", resumed: false, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/git/push-preflight" {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"success":          false,
						"preflight_reason": "history_update_required",
						"error":            "remote branch contains commits not present locally",
					})
					return
				}
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK)
					return
				}
				http.NotFound(w, r)
			}))
			t.Cleanup(server.Close)

			host, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
			if err != nil {
				t.Fatalf("parse agentctl test URL: %v", err)
			}
			port, err := strconv.Atoi(portText)
			if err != nil {
				t.Fatalf("parse agentctl test port: %v", err)
			}

			mgr := newTestManager(t)
			execution := &AgentExecution{
				ID:               "execution-contribution-preflight",
				SessionID:        "session-contribution-preflight",
				isResumedSession: tt.resumed,
				metadata: map[string]interface{}{
					MetadataKeyRemoteContributions: map[string]models.RemoteContribution{
						"": contributionPreflightTestBinding(),
					},
				},
				agentctl: agentctl.NewClient(host, port, newTestLogger()),
			}

			err = mgr.preflightRemoteContributionPushes(context.Background(), execution)
			if tt.wantError && err == nil {
				t.Fatal("preflight unexpectedly succeeded")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("preflight returned error: %v", err)
			}
		})
	}
}

func contributionPreflightTestBinding() models.RemoteContribution {
	return models.RemoteContribution{
		Version:      models.RemoteContributionVersion,
		Provider:     models.RemoteContributionProviderGitHub,
		Kind:         models.RemoteContributionKindPullRequest,
		CanonicalURL: "https://github.com/acme/widget/pull/7",
		Number:       7,
		State:        models.RemoteContributionStateOpen,
		BaseBranch:   "main",
		HeadBranch:   "feature/remote",
		HeadSHA:      strings.Repeat("a", 40),
		SourceRepository: models.RemoteContributionRepository{
			Host: "github.com", Path: "contributor/widget", RemoteURL: "https://github.com/contributor/widget.git",
		},
		CollaborationAllowed: true,
	}
}
