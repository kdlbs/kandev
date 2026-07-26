package backendapp

import "testing"

// TestWorkspaceIDFromPath covers the gitlab /workspaces/:id/ path extraction
// used by integrationWorkspaceScopeMiddleware (query params are covered by the
// middleware's ownership check directly).
func TestWorkspaceIDFromPath(t *testing.T) {
	cases := map[string]string{
		"/api/v1/gitlab/workspaces/ws-1/task-mrs":     "ws-1",
		"/api/v1/gitlab/workspaces/ws-2":              "ws-2",
		"/api/v1/gitlab/status":                       "",
		"/api/v1/jira/config":                         "",
		"/api/v1/gitlab/workspaces/ws-3/task-mrs/x/y": "ws-3",
	}
	for path, want := range cases {
		if got := workspaceIDFromPath(path); got != want {
			t.Errorf("workspaceIDFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestHasIntegrationPrefix(t *testing.T) {
	inside := []string{
		"/api/v1/jira/config", "/api/v1/gitlab/status", "/api/v1/github/status",
		"/api/v1/linear/config", "/api/v1/sentry/config", "/api/v1/slack/config",
		"/api/v1/azure-devops/config", "/api/v1/workflow-sync/status",
	}
	outside := []string{
		"/api/v1/tasks", "/api/v1/workspaces", "/api/v1/office/tasks/t1",
		"/health", "/api/v1/jira", // exact prefix without trailing slash is not matched
	}
	for _, p := range inside {
		if !hasIntegrationPrefix(p) {
			t.Errorf("%q should be an integration path", p)
		}
	}
	for _, p := range outside {
		if hasIntegrationPrefix(p) {
			t.Errorf("%q should NOT be an integration path", p)
		}
	}
}
