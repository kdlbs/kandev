package github

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestPassiveWorkspaceRefreshHonorsAdaptiveSchedule(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	// Keep the persisted watch older than the injected scheduler clock. This
	// isolates task inactivity from the new-watch fast baseline.
	now := time.Now().UTC().Add(48 * time.Hour)
	svc.SetClock(func() time.Time { return now })
	provider := &scheduleActivityProvider{activities: map[string]PRWatchTaskActivity{
		"task-passive-slow": {LastActivityAt: now.Add(-24 * time.Hour)},
	}}
	svc.SetTaskActivityProvider(provider)
	seedTask(t, store, "task-passive-slow", false)
	watch, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-passive", "task-passive-slow", "repo-1", "owner", "repo", 0, "feature/passive")
	if err != nil {
		t.Fatalf("create passive watch: %v", err)
	}
	lastChecked := now.Add(-10 * time.Minute)
	if err := store.UpdatePRWatchTimestamps(ctx, watch.ID, lastChecked, nil, "", ""); err != nil {
		t.Fatalf("stamp passive watch: %v", err)
	}

	if _, err := svc.ListWorkspaceTaskPRs(ctx, testWorkspaceID); err != nil {
		t.Fatalf("passive workspace read: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if len(gh.branchQueries) != 0 {
		t.Fatal("passive refresh checked a slow searching watch before its due interval")
	}

	gh.branchResponses = []string{`{"data":{"b0":{"pullRequests":{"nodes":[]}}}}`}
	started := make(chan struct{}, 1)
	gh.onExecute = func() { started <- struct{}{} }
	svc.SetClock(func() time.Time { return now.Add(30 * time.Minute) })
	if _, err := svc.ListWorkspaceTaskPRs(ctx, testWorkspaceID); err != nil {
		t.Fatalf("due passive workspace read: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("passive refresh did not check the slow searching watch at its due interval")
	}
	svc.Stop()
}

func TestPassiveWorkspaceRefreshCooldownSkipsRepeatedWorkspaceReads(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	svc.SetClock(func() time.Time { return now })
	seedTask(t, store, "task-passive-cooldown", false)
	if _, err := svc.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-passive-cooldown", "task-passive-cooldown", "repo-1",
		"owner", "repo", 0, "feature/passive-cooldown",
	); err != nil {
		t.Fatalf("create passive watch: %v", err)
	}
	gh.branchResponses = []string{
		`{"data":{"b0":{"pullRequests":{"nodes":[]}}}}`,
		`{"data":{"b0":{"pullRequests":{"nodes":[]}}}}`,
	}

	if _, err := svc.ListWorkspaceTaskPRs(ctx, testWorkspaceID); err != nil {
		t.Fatalf("first passive workspace read: %v", err)
	}
	waitForPassiveWorkspaceRefresh(t, svc, testWorkspaceID)
	if got := len(gh.branchQueries); got != 1 {
		t.Fatalf("first passive refresh queries = %d, want 1", got)
	}

	if _, err := svc.ListWorkspaceTaskPRs(ctx, testWorkspaceID); err != nil {
		t.Fatalf("second passive workspace read: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if got := len(gh.branchQueries); got != 1 {
		t.Fatalf("cooldown allowed repeated passive refresh queries = %d, want 1", got)
	}

	svc.SetClock(func() time.Time { return now.Add(2 * searchFastPollInterval) })
	if _, err := svc.ListWorkspaceTaskPRs(ctx, testWorkspaceID); err != nil {
		t.Fatalf("due passive workspace read: %v", err)
	}
	waitForPassiveWorkspaceRefresh(t, svc, testWorkspaceID)
	if got := len(gh.branchQueries); got != 2 {
		t.Fatalf("passive refresh after cooldown queries = %d, want 2", got)
	}
	svc.Stop()
}

func TestExplicitPRRefreshBypassesAdaptiveIdleSchedule(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	svc.SetTaskActivityProvider(&scheduleActivityProvider{activities: map[string]PRWatchTaskActivity{
		"task-explicit-slow": {LastActivityAt: now.Add(-24 * time.Hour)},
	}})
	seedTask(t, store, "task-explicit-slow", false)
	watch, err := svc.CreatePRWatchForWorkspace(ctx, testWorkspaceID, "session-explicit", "task-explicit-slow", "repo-1", "owner", "repo", 0, "feature/explicit")
	if err != nil {
		t.Fatalf("create explicit watch: %v", err)
	}
	if err := store.UpdatePRWatchTimestamps(ctx, watch.ID, now.Add(-10*time.Minute), nil, "", ""); err != nil {
		t.Fatalf("stamp explicit watch: %v", err)
	}
	gh.branchResponses = []string{`{"data":{"b0":{"pullRequests":{"nodes":[]}}}}`}
	if _, err := svc.TriggerPRSyncAll(ctx, "task-explicit-slow"); err != nil {
		t.Fatalf("explicit PR refresh: %v", err)
	}
	if len(gh.branchQueries) != 1 {
		t.Fatalf("explicit refresh branch queries = %d, want 1", len(gh.branchQueries))
	}
}

func TestWSSyncTaskPRUsesPassiveAdmissionAndExplicitRefresh(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	now := time.Now().UTC().Add(48 * time.Hour)
	svc.SetClock(func() time.Time { return now })
	svc.SetTaskActivityProvider(&scheduleActivityProvider{activities: map[string]PRWatchTaskActivity{
		"task-ws-passive": {LastActivityAt: now.Add(-24 * time.Hour)},
	}})
	seedTask(t, store, "task-ws-passive", false)
	watch, err := svc.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-ws-passive", "task-ws-passive", "repo-1",
		"owner", "repo", 0, "feature/ws-passive",
	)
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	if err := store.UpdatePRWatchTimestamps(ctx, watch.ID, now.Add(-10*time.Minute), nil, "", ""); err != nil {
		t.Fatalf("stamp watch: %v", err)
	}

	passive, err := ws.NewRequest("passive", ws.ActionGitHubTaskPRSync, map[string]any{
		"task_id": "task-ws-passive",
	})
	if err != nil {
		t.Fatalf("build passive request: %v", err)
	}
	if _, err := wsSyncTaskPR(svc, nil)(ctx, passive); err != nil {
		t.Fatalf("passive sync request: %v", err)
	}
	if len(gh.branchQueries) != 0 {
		t.Fatalf("passive idle branch queries = %d, want 0", len(gh.branchQueries))
	}

	gh.branchResponses = []string{`{"data":{"b0":{"pullRequests":{"nodes":[]}}}}`}
	explicit, err := ws.NewRequest("explicit", ws.ActionGitHubTaskPRSync, map[string]any{
		"task_id":          "task-ws-passive",
		"explicit_refresh": true,
	})
	if err != nil {
		t.Fatalf("build explicit request: %v", err)
	}
	if _, err := wsSyncTaskPR(svc, nil)(ctx, explicit); err != nil {
		t.Fatalf("explicit sync request: %v", err)
	}
	if len(gh.branchQueries) != 1 {
		t.Fatalf("explicit branch queries = %d, want 1", len(gh.branchQueries))
	}
	svc.Stop()
}

func TestWSSyncTaskPRExplicitRefreshSupersedesInFlightPassiveBatch(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	defer svc.Stop()
	ctx := context.Background()
	seedTask(t, store, "task-ws-inflight", false)
	_, err := svc.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-ws-inflight", "task-ws-inflight", "repo-1",
		"owner", "repo", 42, "feature/ws-inflight",
	)
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-ws-inflight", Owner: "owner", Repo: "repo", PRNumber: 42,
		PRURL: "https://github.com/owner/repo/pull/42", PRTitle: "PR 42",
		HeadBranch: "feature/ws-inflight", BaseBranch: "main", State: "open", HeadSHA: "initial",
	}); err != nil {
		t.Fatalf("create task PR: %v", err)
	}
	gh.prResponses = []string{
		`{"data":{"repo0":{"pr0":{"state":"OPEN","title":"PR 42","url":"https://github.com/owner/repo/pull/42","isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","headRefName":"feature/ws-inflight","baseRefName":"main","headRefOid":"passive-head","author":{"login":"alice"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z","reviews":{"nodes":[]},"reviewRequests":{"totalCount":0},"commits":{"nodes":[]}}}}}`,
		`{"data":{"repo0":{"pr0":{"state":"OPEN","title":"PR 42","url":"https://github.com/owner/repo/pull/42","isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","headRefName":"feature/ws-inflight","baseRefName":"main","headRefOid":"explicit-head","author":{"login":"alice"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z","reviews":{"nodes":[]},"reviewRequests":{"totalCount":0},"commits":{"nodes":[]}}}}}`,
	}
	started := make(chan struct{})
	secondStarted := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	var executeCount atomic.Int32
	gh.onExecute = func() {
		switch executeCount.Add(1) {
		case 1:
			close(started)
			<-releaseFirst
		case 2:
			secondStarted <- struct{}{}
		}
	}

	passive, err := ws.NewRequest("passive-inflight", ws.ActionGitHubTaskPRSync, map[string]any{
		"task_id": "task-ws-inflight",
	})
	if err != nil {
		t.Fatalf("build passive request: %v", err)
	}
	passiveDone := make(chan error, 1)
	go func() {
		_, requestErr := wsSyncTaskPR(svc, nil)(ctx, passive)
		passiveDone <- requestErr
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("passive sync did not enter the provider call")
	}

	explicit, err := ws.NewRequest("explicit-inflight", ws.ActionGitHubTaskPRSync, map[string]any{
		"task_id":          "task-ws-inflight",
		"explicit_refresh": true,
	})
	if err != nil {
		t.Fatalf("build explicit request: %v", err)
	}
	explicitDone := make(chan error, 1)
	go func() {
		_, requestErr := wsSyncTaskPR(svc, nil)(ctx, explicit)
		explicitDone <- requestErr
	}()
	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		close(releaseFirst)
		<-passiveDone
		<-explicitDone
		t.Fatal("explicit refresh joined the in-flight passive provider call")
	}
	close(releaseFirst)
	select {
	case err := <-passiveDone:
		if err != nil {
			t.Fatalf("passive sync: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("passive sync did not finish")
	}
	select {
	case err := <-explicitDone:
		if err != nil {
			t.Fatalf("explicit sync: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("explicit sync did not finish")
	}
	if got := len(gh.prQueries); got != 2 {
		t.Fatalf("provider batch calls = %d, want two independent reads", got)
	}
}

func TestWSSyncTaskPRPassiveKeepsActionsCache(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-ws-actions", false)
	_, err := svc.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-ws-actions", "task-ws-actions", "repo-1",
		"owner", "repo", 42, "feature/ws-actions",
	)
	if err != nil {
		t.Fatalf("create numbered watch: %v", err)
	}
	scope := testAutomationScope(t, svc, testWorkspaceID)
	key := workflowRunsCacheKey(scope, "owner", "repo", "head-ws-actions")
	svc.workflowRunsCache.setWithTTL(key, []WorkflowRun{{
		ID: 1, RunAttempt: 1, Status: workflowStatusCompleted, Conclusion: "success",
	}}, workflowAttentionLongTTL)
	gh.ReplaceWorkflowRuns("owner", "repo", "head-ws-actions", []WorkflowRun{{
		ID: 2, RunAttempt: 1, Status: workflowStatusCompleted, Conclusion: "success",
	}})
	response := `{"data":{"repo0":{"pr0":{"state":"OPEN","title":"PR","url":"https://x/42","isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","headRefName":"feature/ws-actions","baseRefName":"main","headRefOid":"head-ws-actions","author":{"login":"alice"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z","reviews":{"nodes":[]},"reviewRequests":{"totalCount":0},"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS"}}}]}}}}}`
	gh.prResponses = []string{response, response}

	passive, err := ws.NewRequest("passive-actions", ws.ActionGitHubTaskPRSync, map[string]any{
		"task_id": "task-ws-actions",
	})
	if err != nil {
		t.Fatalf("build passive actions request: %v", err)
	}
	if _, err := wsSyncTaskPR(svc, nil)(ctx, passive); err != nil {
		t.Fatalf("passive actions sync: %v", err)
	}
	passiveValue, ok := svc.workflowRunsCache.get(key)
	if !ok || passiveValue.([]WorkflowRun)[0].ID != 1 {
		t.Fatal("passive task sync invalidated completed Actions cache")
	}

	explicit, err := ws.NewRequest("explicit-actions", ws.ActionGitHubTaskPRSync, map[string]any{
		"task_id":          "task-ws-actions",
		"explicit_refresh": true,
	})
	if err != nil {
		t.Fatalf("build explicit actions request: %v", err)
	}
	if _, err := wsSyncTaskPR(svc, nil)(ctx, explicit); err != nil {
		t.Fatalf("explicit actions sync: %v", err)
	}
	explicitValue, ok := svc.workflowRunsCache.get(key)
	if !ok || explicitValue.([]WorkflowRun)[0].ID != 2 {
		t.Fatalf("explicit task refresh did not replace the Actions cache: %#v", explicitValue)
	}
	svc.Stop()
}
