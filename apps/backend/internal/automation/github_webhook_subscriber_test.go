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
				WorkspaceIDs: []string{"ws-1"},
				Owner:        "acme",
				Name:         "repo",
				Branch:       branch,
				SHA:          sha,
				PusherLogin:  "alice",
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
		if evt.DedupKey != "push:acme/repo@sha-2" {
			t.Fatalf("dedup key = %q", evt.DedupKey)
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

func TestGitHubWebhookSubscriberStopDrainsCleanly(t *testing.T) {
	svc := newTestService(t)
	sub := newTestSubscriber(t, svc)
	sub.Stop()
	// Calling Stop twice (once here, once via t.Cleanup) must be safe.
}
