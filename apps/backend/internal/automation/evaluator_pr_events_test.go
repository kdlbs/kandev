package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
)

const eventSelectionRepo = `"repos":[{"owner":"acme","name":"widget"}]`
const eventSelectionDedupKey = "pr:acme/widget#17"

type eventSelectionSecrets struct{}

func (eventSelectionSecrets) List(context.Context) ([]*github.SecretListItem, error) { return nil, nil }
func (eventSelectionSecrets) Reveal(_ context.Context, key string) (string, error) {
	if key != github.WorkspacePATSecretKey("ws-1") {
		return "", fmt.Errorf("unexpected fixture credential key: %s", key)
	}
	return "event-fixture-token", nil
}

type eventSelectionTransport struct {
	t        *testing.T
	searches int
	draft    bool
}

func (r *eventSelectionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	require.Equal(r.t, "api.github.com", req.URL.Host)
	require.Equal(r.t, "/search/issues", req.URL.Path)
	query := req.URL.Query().Get("q")
	require.True(r.t, strings.HasPrefix(query, "is:pr is:open repo:"), "query = %q", query)
	repo := strings.TrimPrefix(query, "is:pr is:open repo:")
	owner, name, found := strings.Cut(repo, "/")
	require.True(r.t, found, "query repo = %q", repo)
	require.Contains(r.t, req.Header.Get("Authorization"), "event-fixture-token")
	r.searches++
	body := fmt.Sprintf(`{"items":[{"id":17,"number":17,"title":"Selection fixture","state":"open","draft":%t,"html_url":"https://github.com/%s/%s/pull/17","repository_url":"https://api.github.com/repos/%s/%s","user":{"login":"alice"}}]}`, r.draft, owner, name, owner, name)
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.5
func TestGitHubPRScopeUnion(t *testing.T) {
	f := newEventSelectionFixture(t)
	require.NoError(t, f.githubStore.UpsertWorkspaceSettings(context.Background(), &github.WorkspaceSettings{
		WorkspaceID: "ws-1", RepoScopeMode: github.RepoScopeModeSelected,
		RepoScopeOrgs: []string{"acme"}, RepoScopeRepos: []github.RepoFilter{{Owner: "outside", Name: "tool"}},
	}))
	trigger := f.add(t, `{"events":["opened"],"repos":[{"owner":"acme","name":"widget"},{"owner":"outside","name":"tool"},{"owner":"other","name":"private"}]}`)
	prs, err := f.evaluator.githubService.ListAutomationPRs(context.Background(), "ws-1", "outside", "tool", "is:pr is:open repo:outside/tool")
	require.NoError(t, err)
	require.Len(t, prs, 1)
	require.Equal(t, "outside/tool", prs[0].RepoOwner+"/"+prs[0].RepoName)
	_, err = f.evaluator.githubService.ListAutomationPRs(context.Background(), "ws-1", "other", "private", "is:pr is:open repo:other/private")
	require.Error(t, err, "unselected repository escaped the workspace scope")
	require.Equal(t, 1, f.transport.searches, "denied repository reached GitHub")
	f.transport.searches = 0

	f.evaluator.evaluatePRTriggers(context.Background())
	require.Equal(t, 2, f.transport.searches, "provider searches")
	require.Len(t, f.fired, 1, "only the first run is admitted while the automation is active")
	runs, err := f.svc.Store().ListRuns(context.Background(), f.automation.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 2, "both allowed repositories are evaluated")
	gotRepos := make(map[string]RunStatus, len(runs))
	for _, run := range runs {
		var data map[string]any
		require.NoError(t, json.Unmarshal(run.TriggerData, &data))
		repo, ok := data["repo"].(string)
		require.True(t, ok, "run trigger data repo = %#v", data["repo"])
		gotRepos[repo] = run.Status
		reserved, err := f.svc.Store().HasRunWithDedupKey(context.Background(), f.automation.ID, run.DedupKey)
		require.NoError(t, err)
		require.True(t, reserved, "run for %s must retain its PR dedup reservation", repo)
	}
	require.Contains(t, gotRepos, "acme/widget")
	require.Contains(t, gotRepos, "outside/tool")
	current, err := f.svc.GetAutomation(context.Background(), f.automation.ID)
	require.NoError(t, err)
	require.NotNil(t, current.LastTriggeredAt)
	saved, err := f.svc.GetTrigger(context.Background(), trigger.ID)
	require.NoError(t, err)
	require.NotNil(t, saved.LastEvaluatedAt)
	require.JSONEq(t, string(trigger.Config), string(saved.Config))
	event := <-f.fired
	var eventData map[string]any
	require.NoError(t, json.Unmarshal(event.TriggerData, &eventData))
	require.Equal(t, "acme/widget", eventData["repo"])
}

type eventSelectionFixture struct {
	svc         *Service
	evaluator   *GitHubEvaluator
	githubStore *github.Store
	automation  *Automation
	transport   *eventSelectionTransport
	fired       chan *AutomationTriggeredEvent
}

func newEventSelectionFixture(t *testing.T) *eventSelectionFixture {
	t.Helper()
	svc := newTestService(t)
	t.Cleanup(svc.eventBus.Close)
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	store, err := github.NewStore(db, db)
	require.NoError(t, err)
	require.NoError(t, store.UpsertWorkspaceConnection(context.Background(), &github.WorkspaceConnection{
		WorkspaceID: "ws-1", Source: github.ConnectionSourcePAT, GitHubHost: "github.com",
		Login: "fixture-owner", Status: github.ConnectionStatusActive, CredentialGeneration: 1,
	}))
	transport := &eventSelectionTransport{t: t}
	original := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original })
	githubService := github.NewService(nil, github.AuthMethodPAT, eventSelectionSecrets{}, store, svc.eventBus, svc.logger)
	t.Cleanup(githubService.Stop)
	return &eventSelectionFixture{svc: svc, evaluator: NewGitHubEvaluator(svc, githubService, svc.logger),
		githubStore: store, automation: createTestAutomation(t, svc, "ws-1"), transport: transport,
		fired: subscribeAutomationTriggered(t, svc.eventBus)}
}

func (f *eventSelectionFixture) add(t *testing.T, raw string) *AutomationTrigger {
	t.Helper()
	trigger, err := f.svc.AddTrigger(context.Background(), &AddTriggerRequest{
		AutomationID: f.automation.ID, Type: TriggerTypeGitHubPR, Enabled: true, Config: json.RawMessage(raw),
	})
	require.NoError(t, err)
	return trigger
}

func (f *eventSelectionFixture) assertOutcome(t *testing.T, trigger *AutomationTrigger, searches, firings int) {
	t.Helper()
	ctx := context.Background()
	require.Equal(t, searches, f.transport.searches, "provider searches")
	require.Len(t, f.fired, firings, "AutomationTriggered publications")
	runs, err := f.svc.Store().ListRuns(ctx, f.automation.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, firings, "admitted or skipped run rows")
	reserved, err := f.svc.Store().HasRunWithDedupKey(ctx, f.automation.ID, eventSelectionDedupKey)
	require.NoError(t, err)
	require.Equal(t, firings > 0, reserved, "PR dedup reservation")
	current, err := f.svc.GetAutomation(ctx, f.automation.ID)
	require.NoError(t, err)
	require.Equal(t, firings > 0, current.LastTriggeredAt != nil)
	saved, err := f.svc.GetTrigger(ctx, trigger.ID)
	require.NoError(t, err)
	if firings == 0 {
		require.Equal(t, trigger.LastEvaluatedAt, saved.LastEvaluatedAt)
	} else {
		require.NotNil(t, saved.LastEvaluatedAt)
	}
	require.JSONEq(t, string(trigger.Config), string(saved.Config))
	if firings > 0 {
		event := <-f.fired
		require.Equal(t, trigger.ID, event.TriggerID)
		require.Equal(t, eventSelectionDedupKey, event.DedupKey)
		require.Equal(t, runs[0].ID, event.RunID)
	}
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.19
func TestGitHubPREventSelection(t *testing.T) {
	cases := []struct {
		name, config string
		firings      int
	}{
		{"cleared", `{"events":[],` + eventSelectionRepo + `}`, 0},
		{"missing", `{` + eventSelectionRepo + `}`, 0},
		{"null events", `{"events":null,` + eventSelectionRepo + `}`, 0},
		{"null config", `null`, 0},
		{"unsupported only", `{"events":["closed"],` + eventSelectionRepo + `}`, 0},
		{"case sensitive", `{"events":["Opened"],` + eventSelectionRepo + `}`, 0},
		{"opened", `{"events":["opened"],` + eventSelectionRepo + `}`, 1},
		{"mixed opened last", `{"events":["closed","opened"],` + eventSelectionRepo + `}`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newEventSelectionFixture(t)
			trigger := f.add(t, tc.config)
			f.evaluator.evaluatePRTriggers(context.Background())
			f.assertOutcome(t, trigger, tc.firings, tc.firings)
		})
	}
}

func TestGitHubPREventSelectionControls(t *testing.T) {
	cases := []struct {
		name, extra       string
		searches, firings int
	}{
		{"eligible", "", 1, 1},
		{"draft excluded", `,"exclude_draft":true`, 1, 0},
		{"author mismatch", `,"authors":["bob"]`, 1, 0},
		{"author match", `,"authors":["alice"]`, 1, 1},
		{"branch mismatch", `,"branches":["release/*"]`, 1, 0},
		{"branch wildcard", `,"branches":["*"]`, 1, 1},
		{"trigger disabled", "", 0, 0},
		{"automation disabled", "", 0, 0},
		{"no explicit repos", "", 0, 0},
		{"foreign repo scope", "", 0, 0},
		{"missing workspace", "", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newEventSelectionFixture(t)
			raw := `{"events":["opened"],` + eventSelectionRepo + tc.extra + `}`
			if tc.name == "no explicit repos" {
				raw = `{"events":["opened"],"all_repos":true}`
			}
			trigger := f.add(t, raw)
			configureEventSelectionControl(t, f, trigger, tc.name)
			f.evaluator.evaluatePRTriggers(context.Background())
			f.assertOutcome(t, trigger, tc.searches, tc.firings)
		})
	}
}

func configureEventSelectionControl(t *testing.T, f *eventSelectionFixture, trigger *AutomationTrigger, name string) {
	t.Helper()
	ctx := context.Background()
	disabled := false
	switch name {
	case "draft excluded":
		f.transport.draft = true
	case "trigger disabled":
		require.NoError(t, f.svc.UpdateTrigger(ctx, trigger.ID, &UpdateTriggerRequest{Enabled: &disabled}))
	case "automation disabled":
		_, err := f.svc.UpdateAutomation(ctx, f.automation.ID, &UpdateAutomationRequest{Enabled: &disabled})
		require.NoError(t, err)
	case "foreign repo scope":
		require.NoError(t, f.githubStore.UpsertWorkspaceSettings(ctx, &github.WorkspaceSettings{
			WorkspaceID: "ws-1", RepoScopeMode: github.RepoScopeModeRepos,
			RepoScopeRepos: []github.RepoFilter{{Owner: "other", Name: "widget"}},
		}))
	case "missing workspace":
		_, err := f.svc.Store().db.ExecContext(ctx, `UPDATE automations SET workspace_id = '' WHERE id = ?`, f.automation.ID)
		require.NoError(t, err)
	}
}

func TestGitHubPREventSelectionPersistenceAndDedup(t *testing.T) {
	f := newEventSelectionFixture(t)
	ctx := context.Background()
	var config map[string]any
	require.NoError(t, json.Unmarshal(GetTriggerTypeInfo(TriggerTypeGitHubPR).DefaultConfig, &config))
	require.Equal(t, []any{"opened"}, config["events"])
	config["repos"] = []github.RepoFilter{{Owner: "acme", Name: "widget"}}
	opened, err := json.Marshal(config)
	require.NoError(t, err)
	trigger := f.add(t, string(opened))
	cleared := json.RawMessage(`{"events":[],` + eventSelectionRepo + `}`)
	require.NoError(t, f.svc.UpdateTrigger(ctx, trigger.ID, &UpdateTriggerRequest{Config: &cleared}))
	trigger, err = f.svc.GetTrigger(ctx, trigger.ID)
	require.NoError(t, err)
	f.evaluator.evaluatePRTriggers(ctx)
	f.assertOutcome(t, trigger, 0, 0)
	selected := json.RawMessage(opened)
	require.NoError(t, f.svc.UpdateTrigger(ctx, trigger.ID, &UpdateTriggerRequest{Config: &selected}))
	trigger, err = f.svc.GetTrigger(ctx, trigger.ID)
	require.NoError(t, err)
	f.evaluator.evaluatePRTriggers(ctx)
	f.assertOutcome(t, trigger, 1, 1)
	f.evaluator.evaluatePRTriggers(ctx)
	require.Equal(t, 2, f.transport.searches)
	require.Empty(t, f.fired, "duplicate PR must not publish again")
	runs, err := f.svc.Store().ListRuns(ctx, f.automation.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, RunStatusTriggered, runs[0].Status)
}
