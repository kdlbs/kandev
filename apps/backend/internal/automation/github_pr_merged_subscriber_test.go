package automation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
)

// fakeTaskOriginLookup is a configurable stub for TaskOriginLookup.
type fakeTaskOriginLookup struct {
	results map[string]fakeOriginResult
}

type fakeOriginResult struct {
	workspaceID     string
	isAutomationRun bool
	ok              bool
}

func (f *fakeTaskOriginLookup) TaskWorkspaceAndAutomationOrigin(_ context.Context, taskID string) (string, bool, bool) {
	if r, ok := f.results[taskID]; ok {
		return r.workspaceID, r.isAutomationRun, r.ok
	}
	return "", false, false
}

func newPRMergedTestService(t *testing.T) *Service {
	t.Helper()
	store := setupTestStore(t)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	eb := bus.NewMemoryEventBus(log)
	return NewService(store, eb, log)
}

func newPRMergedSubscriber(t *testing.T, svc *Service, lookup TaskOriginLookup) *GitHubPRMergedSubscriber {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if lookup != nil {
		svc.SetTaskOriginLookup(lookup)
	}
	sub := NewGitHubPRMergedSubscriber(svc, svc.eventBus, log)
	sub.Start(context.Background())
	t.Cleanup(sub.Stop)
	return sub
}

func publishPRUpdated(t *testing.T, svc *Service, pr *github.TaskPR) {
	t.Helper()
	if err := svc.eventBus.Publish(context.Background(), events.GitHubTaskPRUpdated, bus.NewEvent(
		events.GitHubTaskPRUpdated, "test", pr,
	)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
}

func mergedPR() *github.TaskPR {
	ts := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	return &github.TaskPR{
		TaskID:     "t_abc123",
		Owner:      "acme",
		Repo:       "api",
		PRNumber:   7,
		PRURL:      "https://github.com/acme/api/pull/7",
		BaseBranch: "main",
		State:      "merged",
		MergedAt:   &ts,
	}
}

// TestGitHubPRMergedSubscriberFiresHappyPath checks the full happy path:
// creates an automation with an all-repos trigger, publishes a merged PR event,
// and verifies the AutomationTriggered event fires with the expected data.
func TestGitHubPRMergedSubscriberFiresHappyPath(t *testing.T) {
	svc := newPRMergedTestService(t)
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", isAutomationRun: false, ok: true},
		},
	}
	newPRMergedSubscriber(t, svc, lookup)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a := createTestAutomation(t, svc, "ws-1")
	trig := addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{
		AllRepos: true,
	})

	pr := mergedPR()
	publishPRUpdated(t, svc, pr)

	select {
	case evt := <-fired:
		if evt.AutomationID != a.ID || evt.TriggerID != trig.ID {
			t.Fatalf("unexpected evt ids: %+v", evt)
		}
		if evt.TriggerType != TriggerTypeGitHubPRMerged {
			t.Fatalf("wrong TriggerType: %s", evt.TriggerType)
		}
		want := "pr_merged:t_abc123:acme/api#7"
		if evt.DedupKey != want {
			t.Fatalf("dedup key = %q, want %q", evt.DedupKey, want)
		}
		var data map[string]interface{}
		if err := json.Unmarshal(evt.TriggerData, &data); err != nil {
			t.Fatalf("unmarshal trigger data: %v", err)
		}
		checks := map[string]string{
			"task_id":     "t_abc123",
			"repo":        "acme/api",
			"pr_url":      "https://github.com/acme/api/pull/7",
			"base_branch": "main",
			"merged_at":   "2026-03-08T12:00:00Z",
		}
		for k, wantV := range checks {
			if got, ok := data[k]; !ok || got != wantV {
				t.Errorf("data[%q] = %v, want %q", k, got, wantV)
			}
		}
		if num, ok := data["pr_number"].(float64); !ok || int(num) != 7 {
			t.Errorf("data[pr_number] = %v, want 7", data["pr_number"])
		}
		// Security contract: exactly 6 keys, no prose fields.
		// Spec lines 1320-1326: "no more and no fewer" and "what fails if someone widens the map."
		if len(data) != 6 {
			t.Errorf("trigger data has %d keys, want exactly 6: %v", len(data), data)
		}
		for _, forbidden := range []string{"pr_title", "body", "author_login", "head_branch"} {
			if _, exists := data[forbidden]; exists {
				t.Errorf("trigger data must not contain %q", forbidden)
			}
		}
	default:
		t.Fatal("expected AutomationTriggered event")
	}
}

// TestGitHubPRMergedSubscriberGates verifies that each per-event gate blocks
// firing when its condition is violated.
func TestGitHubPRMergedSubscriberGates(t *testing.T) {
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", isAutomationRun: false, ok: true},
		},
	}

	tests := []struct {
		name   string
		mutate func(*github.TaskPR)
	}{
		{"empty task id", func(p *github.TaskPR) { p.TaskID = "" }},
		{"state open", func(p *github.TaskPR) { p.State = "open" }},
		{"state closed", func(p *github.TaskPR) { p.State = "closed" }},
		{"empty owner", func(p *github.TaskPR) { p.Owner = "" }},
		{"empty repo", func(p *github.TaskPR) { p.Repo = "" }},
		{"zero pr_number", func(p *github.TaskPR) { p.PRNumber = 0 }},
		{"negative pr_number", func(p *github.TaskPR) { p.PRNumber = -1 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newPRMergedTestService(t)
			newPRMergedSubscriber(t, svc, lookup)
			fired := subscribeAutomationTriggered(t, svc.eventBus)

			a := createTestAutomation(t, svc, "ws-1")
			addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})

			pr := mergedPR()
			tt.mutate(pr)
			publishPRUpdated(t, svc, pr)

			select {
			case evt := <-fired:
				t.Fatalf("expected no fire, got %+v", evt)
			default:
			}
		})
	}
}

// TestGitHubPRMergedSubscriberWrongPayloadType verifies gate 1: publishing
// the wrong payload type is silently ignored.
func TestGitHubPRMergedSubscriberWrongPayloadType(t *testing.T) {
	svc := newPRMergedTestService(t)
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", ok: true},
		},
	}
	newPRMergedSubscriber(t, svc, lookup)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a := createTestAutomation(t, svc, "ws-1")
	addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})

	// Publish a string instead of *github.TaskPR.
	if err := svc.eventBus.Publish(context.Background(), events.GitHubTaskPRUpdated, bus.NewEvent(
		events.GitHubTaskPRUpdated, "test", "not-a-task-pr",
	)); err != nil {
		t.Fatal(err)
	}

	select {
	case evt := <-fired:
		t.Fatalf("expected no fire, got %+v", evt)
	default:
	}
}

// TestGitHubPRMergedSubscriberStateVariants verifies gate 3 allows
// case-insensitive "Merged" / " merged " variants.
func TestGitHubPRMergedSubscriberStateVariants(t *testing.T) {
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", ok: true},
		},
	}
	cases := []string{"Merged", "MERGED", " merged "}
	for _, state := range cases {
		t.Run(state, func(t *testing.T) {
			svc := newPRMergedTestService(t)
			newPRMergedSubscriber(t, svc, lookup)
			fired := subscribeAutomationTriggered(t, svc.eventBus)

			a := createTestAutomation(t, svc, "ws-1")
			addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})

			pr := mergedPR()
			pr.State = state
			publishPRUpdated(t, svc, pr)

			select {
			case <-fired:
			default:
				t.Fatalf("expected fire for state %q", state)
			}
		})
	}
}

// TestGitHubPRMergedSubscriberLoopGuard verifies gate 7: tasks created by
// an automation run do not trigger further automation runs.
func TestGitHubPRMergedSubscriberLoopGuard(t *testing.T) {
	svc := newPRMergedTestService(t)
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_auto": {workspaceID: "ws-1", isAutomationRun: true, ok: true},
		},
	}
	newPRMergedSubscriber(t, svc, lookup)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a := createTestAutomation(t, svc, "ws-1")
	addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})

	pr := mergedPR()
	pr.TaskID = "t_auto"
	publishPRUpdated(t, svc, pr)

	select {
	case evt := <-fired:
		t.Fatalf("loop guard failed: automation fired for automation-run task %+v", evt)
	default:
	}
}

// TestGitHubPRMergedSubscriberLookupNotResolved verifies gate 6: if the
// task lookup returns ok=false, no trigger fires.
func TestGitHubPRMergedSubscriberLookupNotResolved(t *testing.T) {
	svc := newPRMergedTestService(t)
	lookup := &fakeTaskOriginLookup{results: map[string]fakeOriginResult{}}
	newPRMergedSubscriber(t, svc, lookup)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a := createTestAutomation(t, svc, "ws-1")
	addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})

	publishPRUpdated(t, svc, mergedPR())

	select {
	case evt := <-fired:
		t.Fatalf("expected no fire when task not resolved, got %+v", evt)
	default:
	}
}

// TestGitHubPRMergedSubscriberWorkspaceScoping verifies gate 8: the
// automation's workspace must match the resolved workspace from the task
// lookup. TaskPR.WorkspaceID is completely ignored.
func TestGitHubPRMergedSubscriberWorkspaceScoping(t *testing.T) {
	svc := newPRMergedTestService(t)
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			// Task resolves to ws-2, not ws-1 where the automation lives.
			"t_abc123": {workspaceID: "ws-2", ok: true},
		},
	}
	newPRMergedSubscriber(t, svc, lookup)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a := createTestAutomation(t, svc, "ws-1")
	addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})

	// Even though TaskPR.WorkspaceID matches ws-1, the lookup's ws-2 wins.
	pr := mergedPR()
	pr.WorkspaceID = "ws-1"
	publishPRUpdated(t, svc, pr)

	select {
	case evt := <-fired:
		t.Fatalf("workspace scoping failed: fired for wrong workspace %+v", evt)
	default:
	}
}

// TestGitHubPRMergedSubscriberRepoFilter verifies gate 9.
func TestGitHubPRMergedSubscriberRepoFilter(t *testing.T) {
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", ok: true},
		},
	}

	tests := []struct {
		name     string
		cfg      GitHubPRMergedTriggerConfig
		pr       func() *github.TaskPR
		wantFire bool
	}{
		{
			name: "all_repos fires any",
			cfg:  GitHubPRMergedTriggerConfig{AllRepos: true},
			pr: func() *github.TaskPR {
				p := mergedPR()
				p.Owner, p.Repo = "other", "whatever"
				return p
			},
			wantFire: true,
		},
		{
			name: "exact owner+name match",
			cfg: GitHubPRMergedTriggerConfig{
				Repos: []github.RepoFilter{{Owner: "acme", Name: "api"}},
			},
			pr:       mergedPR,
			wantFire: true,
		},
		{
			name: "case-insensitive owner+name",
			cfg: GitHubPRMergedTriggerConfig{
				Repos: []github.RepoFilter{{Owner: "ACME", Name: "API"}},
			},
			pr:       mergedPR,
			wantFire: true,
		},
		{
			name: "org wildcard (empty name)",
			cfg: GitHubPRMergedTriggerConfig{
				Repos: []github.RepoFilter{{Owner: "acme", Name: ""}},
			},
			pr:       mergedPR,
			wantFire: true,
		},
		{
			name: "wrong owner",
			cfg: GitHubPRMergedTriggerConfig{
				Repos: []github.RepoFilter{{Owner: "other", Name: "api"}},
			},
			pr:       mergedPR,
			wantFire: false,
		},
		{
			name: "wrong repo name",
			cfg: GitHubPRMergedTriggerConfig{
				Repos: []github.RepoFilter{{Owner: "acme", Name: "other"}},
			},
			pr:       mergedPR,
			wantFire: false,
		},
		{
			name:     "all_repos=false empty list never matches",
			cfg:      GitHubPRMergedTriggerConfig{AllRepos: false, Repos: nil},
			pr:       mergedPR,
			wantFire: false,
		},
		{
			name: "empty owner entry skipped (no match)",
			cfg: GitHubPRMergedTriggerConfig{
				Repos: []github.RepoFilter{{Owner: "", Name: ""}},
			},
			pr:       mergedPR,
			wantFire: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newPRMergedTestService(t)
			newPRMergedSubscriber(t, svc, lookup)
			fired := subscribeAutomationTriggered(t, svc.eventBus)

			a := createTestAutomation(t, svc, "ws-1")
			addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, tt.cfg)

			publishPRUpdated(t, svc, tt.pr())

			select {
			case <-fired:
				if !tt.wantFire {
					t.Fatal("expected no fire, but got one")
				}
			default:
				if tt.wantFire {
					t.Fatal("expected fire, but got none")
				}
			}
		})
	}
}

// TestGitHubPRMergedSubscriberBranchFilter verifies gate 10.
func TestGitHubPRMergedSubscriberBranchFilter(t *testing.T) {
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", ok: true},
		},
	}

	tests := []struct {
		name       string
		branches   []string
		baseBranch string
		wantFire   bool
	}{
		{"empty list matches all", nil, "main", true},
		{"all-empty entries matches all", []string{" ", ""}, "main", true},
		{"exact match", []string{"main"}, "main", true},
		{"glob match", []string{"release/*"}, "release/v2", true},
		{"no match", []string{"main", "develop"}, "feature/x", false},
		{"trim whitespace", []string{" main "}, "main", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newPRMergedTestService(t)
			newPRMergedSubscriber(t, svc, lookup)
			fired := subscribeAutomationTriggered(t, svc.eventBus)

			a := createTestAutomation(t, svc, "ws-1")
			addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{
				AllRepos:     true,
				BaseBranches: tt.branches,
			})

			pr := mergedPR()
			pr.BaseBranch = tt.baseBranch
			publishPRUpdated(t, svc, pr)

			select {
			case <-fired:
				if !tt.wantFire {
					t.Fatal("expected no fire")
				}
			default:
				if tt.wantFire {
					t.Fatal("expected fire")
				}
			}
		})
	}
}

// TestGitHubPRMergedSubscriberPerEventFiredKeySet verifies gate 11: when one
// automation has two matching triggers, it fires at most once per event.
func TestGitHubPRMergedSubscriberPerEventFiredKeySet(t *testing.T) {
	svc := newPRMergedTestService(t)
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", ok: true},
		},
	}
	newPRMergedSubscriber(t, svc, lookup)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	// One automation with TWO matching triggers.
	a := createTestAutomation(t, svc, "ws-1")
	addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})
	addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})

	publishPRUpdated(t, svc, mergedPR())

	// Exactly one fire expected.
	select {
	case <-fired:
	default:
		t.Fatal("expected exactly one fire, got none")
	}
	// No second fire.
	select {
	case evt := <-fired:
		t.Fatalf("per-event fired-key set failed: got second fire %+v", evt)
	default:
	}
}

// TestGitHubPRMergedSubscriberTwoAutomationsBothFire verifies that two
// different automations both fire for the same event (keyed per-automation).
func TestGitHubPRMergedSubscriberTwoAutomationsBothFire(t *testing.T) {
	svc := newPRMergedTestService(t)
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", ok: true},
		},
	}
	newPRMergedSubscriber(t, svc, lookup)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a1 := createTestAutomation(t, svc, "ws-1")
	addTestTrigger(t, svc, a1.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})
	a2 := createTestAutomation(t, svc, "ws-1")
	addTestTrigger(t, svc, a2.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})

	publishPRUpdated(t, svc, mergedPR())

	count := 0
	for i := 0; i < 2; i++ {
		select {
		case <-fired:
			count++
		default:
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 fires (one per automation), got %d", count)
	}
}

// TestGitHubPRMergedSubscriberStartIdempotent verifies that calling Start a
// second time is a no-op.
func TestGitHubPRMergedSubscriberStartIdempotent(t *testing.T) {
	svc := newPRMergedTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", ok: true},
		},
	}
	svc.SetTaskOriginLookup(lookup)
	sub := NewGitHubPRMergedSubscriber(svc, svc.eventBus, log)
	t.Cleanup(sub.Stop)

	sub.Start(context.Background())
	sub.Start(context.Background()) // second Start is a no-op

	fired := subscribeAutomationTriggered(t, svc.eventBus)
	a := createTestAutomation(t, svc, "ws-1")
	addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})

	publishPRUpdated(t, svc, mergedPR())

	select {
	case <-fired:
	default:
		t.Fatal("expected fire after idempotent Start")
	}
}

// TestGitHubPRMergedSubscriberStartWithNilLookup verifies that Start with no
// lookup wired sets started=true (subsequent starts are no-ops), emits exactly
// one warn log naming github_pr_merged, and does not subscribe to the bus.
// Spec lines 1368-1371: "emits exactly one log line at warn naming github_pr_merged."
func TestGitHubPRMergedSubscriberStartWithNilLookup(t *testing.T) {
	svc := newPRMergedTestService(t)
	core, obs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	// Do NOT set lookup.
	sub := NewGitHubPRMergedSubscriber(svc, svc.eventBus, log)
	t.Cleanup(sub.Stop)

	sub.Start(context.Background())

	if !sub.started {
		t.Fatal("started should be true even when lookup is nil")
	}
	if sub.sub != nil {
		t.Fatal("sub should not have subscribed when lookup is nil")
	}

	entries := obs.All()
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 log entry on nil-lookup Start, got %d: %v", len(entries), entries)
	}
	if entries[0].Level != zap.WarnLevel {
		t.Errorf("log entry level = %v, want Warn", entries[0].Level)
	}
	if !strings.Contains(entries[0].Message, string(TriggerTypeGitHubPRMerged)) {
		t.Errorf("log message %q does not contain trigger type %q", entries[0].Message, TriggerTypeGitHubPRMerged)
	}
}

// TestGitHubPRMergedSubscriberStartWiredLookupLogsInfo verifies that Start with
// a wired lookup emits exactly one info log and no warn logs.
// Spec lines 1372-1375: "emits a line at info ... no warn line. Both branches are asserted."
func TestGitHubPRMergedSubscriberStartWiredLookupLogsInfo(t *testing.T) {
	svc := newPRMergedTestService(t)
	core, obs := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", ok: true},
		},
	}
	svc.SetTaskOriginLookup(lookup)
	sub := NewGitHubPRMergedSubscriber(svc, svc.eventBus, log)
	t.Cleanup(sub.Stop)

	sub.Start(context.Background())

	var infoCount, warnCount int
	for _, e := range obs.All() {
		switch e.Level {
		case zap.InfoLevel:
			infoCount++
		case zap.WarnLevel:
			warnCount++
		}
	}
	if infoCount == 0 {
		t.Error("expected at least one Info log on wired-lookup Start, got none")
	}
	if warnCount != 0 {
		t.Errorf("expected no Warn logs on wired-lookup Start, got %d", warnCount)
	}
}

// TestGitHubPRMergedSubscriberStopIdempotent verifies Stop can be called
// multiple times without panicking.
func TestGitHubPRMergedSubscriberStopIdempotent(t *testing.T) {
	svc := newPRMergedTestService(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", ok: true},
		},
	}
	svc.SetTaskOriginLookup(lookup)
	sub := NewGitHubPRMergedSubscriber(svc, svc.eventBus, log)
	sub.Start(context.Background())
	sub.Stop()
	sub.Stop() // second Stop is a no-op
}

// TestGitHubPRMergedSubscriberDedupKeyFormat verifies the dedup key is
// lowercase owner/repo to ensure case-normalization.
func TestGitHubPRMergedSubscriberDedupKeyFormat(t *testing.T) {
	svc := newPRMergedTestService(t)
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", ok: true},
		},
	}
	newPRMergedSubscriber(t, svc, lookup)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a := createTestAutomation(t, svc, "ws-1")
	addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})

	pr := mergedPR()
	pr.Owner = "ACME"
	pr.Repo = "API"
	pr.PRNumber = 42
	publishPRUpdated(t, svc, pr)

	select {
	case evt := <-fired:
		want := "pr_merged:t_abc123:acme/api#42"
		if evt.DedupKey != want {
			t.Fatalf("dedup key = %q, want %q", evt.DedupKey, want)
		}
	default:
		t.Fatal("expected fire")
	}
}

// TestGitHubPRMergedSubscriberNilMergedAt verifies a nil MergedAt produces
// an empty string for merged_at in the trigger data (not a panic).
func TestGitHubPRMergedSubscriberNilMergedAt(t *testing.T) {
	svc := newPRMergedTestService(t)
	lookup := &fakeTaskOriginLookup{
		results: map[string]fakeOriginResult{
			"t_abc123": {workspaceID: "ws-1", ok: true},
		},
	}
	newPRMergedSubscriber(t, svc, lookup)
	fired := subscribeAutomationTriggered(t, svc.eventBus)

	a := createTestAutomation(t, svc, "ws-1")
	addTestTrigger(t, svc, a.ID, TriggerTypeGitHubPRMerged, GitHubPRMergedTriggerConfig{AllRepos: true})

	pr := mergedPR()
	pr.MergedAt = nil
	publishPRUpdated(t, svc, pr)

	select {
	case evt := <-fired:
		var data map[string]interface{}
		if err := json.Unmarshal(evt.TriggerData, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got := data["merged_at"]; got != "" {
			t.Fatalf("merged_at = %v, want empty string", got)
		}
	default:
		t.Fatal("expected fire")
	}
}

// Unit tests for matchesMergedRepo and matchesMergedBranches helpers.

func TestMatchesMergedRepo(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		repo  string
		cfg   GitHubPRMergedTriggerConfig
		want  bool
	}{
		{
			name:  "all_repos=true always matches",
			owner: "x", repo: "y",
			cfg:  GitHubPRMergedTriggerConfig{AllRepos: true},
			want: true,
		},
		{
			name:  "empty list never matches",
			owner: "acme", repo: "api",
			cfg:  GitHubPRMergedTriggerConfig{},
			want: false,
		},
		{
			name:  "exact match",
			owner: "acme", repo: "api",
			cfg:  GitHubPRMergedTriggerConfig{Repos: []github.RepoFilter{{Owner: "acme", Name: "api"}}},
			want: true,
		},
		{
			name:  "case-insensitive owner",
			owner: "ACME", repo: "api",
			cfg:  GitHubPRMergedTriggerConfig{Repos: []github.RepoFilter{{Owner: "acme", Name: "api"}}},
			want: true,
		},
		{
			name:  "case-insensitive name",
			owner: "acme", repo: "API",
			cfg:  GitHubPRMergedTriggerConfig{Repos: []github.RepoFilter{{Owner: "acme", Name: "api"}}},
			want: true,
		},
		{
			name:  "org wildcard matches any name under owner",
			owner: "acme", repo: "anything",
			cfg:  GitHubPRMergedTriggerConfig{Repos: []github.RepoFilter{{Owner: "acme", Name: ""}}},
			want: true,
		},
		{
			name:  "org wildcard wrong owner",
			owner: "other", repo: "anything",
			cfg:  GitHubPRMergedTriggerConfig{Repos: []github.RepoFilter{{Owner: "acme", Name: ""}}},
			want: false,
		},
		{
			name:  "empty owner in filter is skipped",
			owner: "acme", repo: "api",
			cfg:  GitHubPRMergedTriggerConfig{Repos: []github.RepoFilter{{Owner: "", Name: "api"}}},
			want: false,
		},
		{
			name:  "wrong name",
			owner: "acme", repo: "other",
			cfg:  GitHubPRMergedTriggerConfig{Repos: []github.RepoFilter{{Owner: "acme", Name: "api"}}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesMergedRepo(tt.owner, tt.repo, tt.cfg)
			if got != tt.want {
				t.Fatalf("matchesMergedRepo(%q, %q, ...) = %v, want %v", tt.owner, tt.repo, got, tt.want)
			}
		})
	}
}

func TestMatchesMergedBranches(t *testing.T) {
	tests := []struct {
		name       string
		baseBranch string
		filters    []string
		want       bool
	}{
		{"nil filters match all", "main", nil, true},
		{"empty filters match all", "main", []string{}, true},
		{"all-whitespace filters match all", "main", []string{" ", "  "}, true},
		{"exact match", "main", []string{"main"}, true},
		{"glob match", "release/v2", []string{"release/*"}, true},
		{"no match", "feature/x", []string{"main", "develop"}, false},
		{"trim whitespace in filter", "main", []string{" main "}, true},
		{"multiple with one matching", "develop", []string{"main", "develop"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesMergedBranches(tt.baseBranch, tt.filters)
			if got != tt.want {
				t.Fatalf("matchesMergedBranches(%q, %v) = %v, want %v", tt.baseBranch, tt.filters, got, tt.want)
			}
		})
	}
}
