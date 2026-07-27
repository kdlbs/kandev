package automation

import (
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
)

func newTestSubscriber(t *testing.T, svc *Service) *GitHubWebhookSubscriber {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	sub := NewGitHubWebhookSubscriber(svc, svc.eventBus, log)
	sub.Start(context.Background())
	t.Cleanup(sub.Stop)
	return sub
}

func createTestAutomation(t *testing.T, svc *Service, workspaceID string) *Automation {
	t.Helper()
	a, err := svc.CreateAutomation(context.Background(), &CreateAutomationRequest{
		WorkspaceID:       workspaceID,
		Name:              "test automation",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		ExecutionMode:     ExecutionModeRun,
	})
	if err != nil {
		t.Fatalf("CreateAutomation() error = %v", err)
	}
	return a
}

func addTestTrigger(t *testing.T, svc *Service, automationID string, triggerType TriggerType, cfg interface{}) *AutomationTrigger {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	trig, err := svc.AddTrigger(context.Background(), &AddTriggerRequest{
		AutomationID: automationID,
		Type:         triggerType,
		Config:       raw,
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("AddTrigger() error = %v", err)
	}
	return trig
}

func subscribeAutomationTriggered(t *testing.T, eventBus bus.EventBus) chan *AutomationTriggeredEvent {
	t.Helper()
	ch := make(chan *AutomationTriggeredEvent, 4)
	if _, err := eventBus.Subscribe(events.AutomationTriggered, func(_ context.Context, e *bus.Event) error {
		if evt, ok := e.Data.(*AutomationTriggeredEvent); ok {
			ch <- evt
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return ch
}

func TestGitHubWebhookSubscriberFiresCheckRunTrigger(t *testing.T) {
	svc := newTestService(t)
	newTestSubscriber(t, svc)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a := createTestAutomation(t, svc, "ws-1")
	trig := addTestTrigger(t, svc, a.ID, TriggerTypeGitHubCI, GitHubCITriggerConfig{
		Repos:       []github.RepoFilter{{Owner: "acme", Name: "repo"}},
		Conclusions: []string{"failure"},
	})

	if err := svc.eventBus.Publish(context.Background(), events.GitHubCheckRunCompleted, bus.NewEvent(
		events.GitHubCheckRunCompleted, "test", &github.GitHubCheckRunEventPayload{
			WorkspaceIDs: []string{"ws-1"},
			Owner:        "acme",
			Name:         "repo",
			Branch:       "main",
			SHA:          "abc1234",
			CheckName:    "build",
			Conclusion:   "failure",
			CheckRunID:   555,
			HTMLURL:      "https://github.com/acme/repo/runs/555",
		},
	)); err != nil {
		t.Fatal(err)
	}

	select {
	case evt := <-fired:
		if evt.AutomationID != a.ID || evt.TriggerID != trig.ID || evt.TriggerType != TriggerTypeGitHubCI {
			t.Fatalf("evt = %+v", evt)
		}
		if evt.DedupKey != "ci:acme/repo#555" {
			t.Fatalf("dedup key = %q", evt.DedupKey)
		}
		// The {{ci.url}} placeholder resolves from data["html_url"]; assert the
		// subscriber populated it so the default CI prompt doesn't render empty.
		var data map[string]interface{}
		if err := json.Unmarshal(evt.TriggerData, &data); err != nil {
			t.Fatal(err)
		}
		if data["html_url"] != "https://github.com/acme/repo/runs/555" {
			t.Fatalf("expected html_url in trigger data, got %+v", data)
		}
	default:
		t.Fatal("expected AutomationTriggered to fire")
	}
}

func TestGitHubWebhookSubscriberCheckRunNegativeCases(t *testing.T) {
	base := github.GitHubCheckRunEventPayload{
		WorkspaceIDs: []string{"ws-1"},
		Owner:        "acme",
		Name:         "repo",
		Branch:       "main",
		SHA:          "abc1234",
		CheckName:    "build",
		Conclusion:   "failure",
		CheckRunID:   555,
	}

	tests := []struct {
		name   string
		mutate func(p *github.GitHubCheckRunEventPayload)
	}{
		{"wrong workspace", func(p *github.GitHubCheckRunEventPayload) { p.WorkspaceIDs = []string{"ws-other"} }},
		{"wrong repo", func(p *github.GitHubCheckRunEventPayload) { p.Name = "other-repo" }},
		{"wrong conclusion", func(p *github.GitHubCheckRunEventPayload) { p.Conclusion = "success" }},
		{"wrong check name", func(p *github.GitHubCheckRunEventPayload) {}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t)
			newTestSubscriber(t, svc)
			fired := subscribeAutomationTriggered(t, svc.eventBus)

			a := createTestAutomation(t, svc, "ws-1")
			cfg := GitHubCITriggerConfig{
				Repos:       []github.RepoFilter{{Owner: "acme", Name: "repo"}},
				Conclusions: []string{"failure"},
			}
			if tt.name == "wrong check name" {
				cfg.CheckNames = []string{"lint"}
			}
			addTestTrigger(t, svc, a.ID, TriggerTypeGitHubCI, cfg)

			payload := base
			tt.mutate(&payload)
			if err := svc.eventBus.Publish(context.Background(), events.GitHubCheckRunCompleted, bus.NewEvent(
				events.GitHubCheckRunCompleted, "test", &payload,
			)); err != nil {
				t.Fatal(err)
			}

			select {
			case evt := <-fired:
				t.Fatalf("expected no fire, got %+v", evt)
			default:
			}
		})
	}
}

func TestGitHubWebhookSubscriberPushTriggerBranchMatch(t *testing.T) {
	svc := newTestService(t)
	newTestSubscriber(t, svc)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a := createTestAutomation(t, svc, "ws-1")
	trig := addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPush, GitHubPushTriggerConfig{
		Repos:    []github.RepoFilter{{Owner: "acme", Name: "repo"}},
		Branches: []string{"main"},
	})

	publishPush := func(branch, sha string) {
		if err := svc.eventBus.Publish(context.Background(), events.GitHubPushReceived, bus.NewEvent(
			events.GitHubPushReceived, "test", &github.GitHubPushEventPayload{
				WorkspaceIDs:  []string{"ws-1"},
				Owner:         "acme",
				Name:          "repo",
				Branch:        branch,
				SHA:           sha,
				PusherLogin:   "alice",
				HeadCommitMsg: "fix: bug",
			},
		)); err != nil {
			t.Fatal(err)
		}
	}

	// Non-matching branch: no fire.
	publishPush("develop", "sha-1")
	select {
	case evt := <-fired:
		t.Fatalf("expected no fire for non-matching branch, got %+v", evt)
	default:
	}

	// Matching branch: fires.
	publishPush("main", "sha-2")
	var dedupKey string
	select {
	case evt := <-fired:
		if evt.AutomationID != a.ID || evt.TriggerID != trig.ID {
			t.Fatalf("evt = %+v", evt)
		}
		if evt.DedupKey != "push:acme/repo@main@sha-2" {
			t.Fatalf("dedup key = %q", evt.DedupKey)
		}
		// {{push.message}} resolves from data["message"]; assert it's populated.
		var data map[string]interface{}
		if err := json.Unmarshal(evt.TriggerData, &data); err != nil {
			t.Fatal(err)
		}
		if data["message"] != "fix: bug" {
			t.Fatalf("expected message in trigger data, got %+v", data)
		}
		dedupKey = evt.DedupKey
	default:
		t.Fatal("expected AutomationTriggered to fire for matching branch")
	}

	// A real deployment records a run row for the fired trigger (the
	// orchestrator does this on AutomationTriggered); do so here so the
	// dedup check in FireTrigger has something to find.
	if err := svc.RecordRun(context.Background(), &AutomationRun{
		AutomationID: a.ID, TriggerID: trig.ID, TriggerType: TriggerTypeGitHubPush,
		Status: RunStatusTaskCreated, DedupKey: dedupKey,
	}); err != nil {
		t.Fatal(err)
	}

	// Same SHA again: dedup key prevents double-fire.
	publishPush("main", "sha-2")
	select {
	case evt := <-fired:
		t.Fatalf("expected dedup to suppress second fire, got %+v", evt)
	default:
	}
}

// TestGitHubWebhookSubscriberPushDedupIsPerBranch pins that the same commit SHA
// pushed to two different (matching) branches fires once per branch, while a
// duplicate delivery on one branch stays suppressed.
func TestGitHubWebhookSubscriberPushDedupIsPerBranch(t *testing.T) {
	svc := newTestService(t)
	newTestSubscriber(t, svc)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a := createTestAutomation(t, svc, "ws-1")
	trig := addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPush, GitHubPushTriggerConfig{
		Repos:    []github.RepoFilter{{Owner: "acme", Name: "repo"}},
		Branches: []string{"main", "develop"},
	})

	publishPush := func(branch, sha string) {
		if err := svc.eventBus.Publish(context.Background(), events.GitHubPushReceived, bus.NewEvent(
			events.GitHubPushReceived, "test", &github.GitHubPushEventPayload{
				WorkspaceIDs: []string{"ws-1"}, Owner: "acme", Name: "repo",
				Branch: branch, SHA: sha,
			},
		)); err != nil {
			t.Fatal(err)
		}
	}
	recordFired := func() {
		select {
		case evt := <-fired:
			// Record as a terminal (succeeded) run: it still carries the dedup
			// key for same-key suppression, but doesn't count toward the
			// concurrency cap, so a distinct-branch push can still fire.
			if err := svc.RecordRun(context.Background(), &AutomationRun{
				AutomationID: a.ID, TriggerID: trig.ID, TriggerType: TriggerTypeGitHubPush,
				Status: RunStatusSucceeded, DedupKey: evt.DedupKey,
			}); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("expected AutomationTriggered to fire")
		}
	}

	// Same SHA on two branches: each fires (distinct dedup keys).
	publishPush("main", "shaX")
	recordFired()
	publishPush("develop", "shaX")
	recordFired()

	// Duplicate delivery on the same branch is suppressed.
	publishPush("main", "shaX")
	select {
	case evt := <-fired:
		t.Fatalf("expected same-branch duplicate to be suppressed, got %+v", evt)
	default:
	}
}

// TestGitHubWebhookSubscriberCITriggerBranchFilter pins that a CI trigger scoped
// to a branch only fires for check runs on that branch.
func TestGitHubWebhookSubscriberCITriggerBranchFilter(t *testing.T) {
	svc := newTestService(t)
	newTestSubscriber(t, svc)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a := createTestAutomation(t, svc, "ws-1")
	addTestTrigger(t, svc, a.ID, TriggerTypeGitHubCI, GitHubCITriggerConfig{
		Repos:       []github.RepoFilter{{Owner: "acme", Name: "repo"}},
		Conclusions: []string{"failure"},
		Branches:    []string{"main"},
	})

	publishCI := func(branch string, runID int64) {
		if err := svc.eventBus.Publish(context.Background(), events.GitHubCheckRunCompleted, bus.NewEvent(
			events.GitHubCheckRunCompleted, "test", &github.GitHubCheckRunEventPayload{
				WorkspaceIDs: []string{"ws-1"}, Owner: "acme", Name: "repo",
				Branch: branch, SHA: "abc", CheckName: "build", Conclusion: "failure", CheckRunID: runID,
			},
		)); err != nil {
			t.Fatal(err)
		}
	}

	// Wrong branch: no fire.
	publishCI("develop", 1)
	select {
	case evt := <-fired:
		t.Fatalf("expected no fire for non-matching CI branch, got %+v", evt)
	default:
	}

	// Matching branch: fires.
	publishCI("main", 2)
	select {
	case <-fired:
	default:
		t.Fatal("expected CI trigger to fire for matching branch")
	}
}

func TestGitHubWebhookSubscriberStopDrainsCleanly(t *testing.T) {
	svc := newTestService(t)
	sub := newTestSubscriber(t, svc)
	sub.Stop()
	// Calling Stop twice (once here, once via t.Cleanup) must be safe.
}
