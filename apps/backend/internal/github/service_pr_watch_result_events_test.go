package github

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.7
func TestPRWatchResultEventsPublishAfterCommit(t *testing.T) {
	t.Run("status update", func(t *testing.T) {
		_, service, _, store := setupPollerTest(t)
		ctx := context.Background()
		seedTask(t, store, "task-status-event", false)
		watch := createResultEventWatch(t, service, "task-status-event", 42, "status-event")
		observed := make(chan error, 1)
		subscription, err := service.eventBus.Subscribe(events.GitHubTaskPRUpdated, func(readCtx context.Context, event *bus.Event) error {
			readCtx, cancel := context.WithTimeout(readCtx, 100*time.Millisecond)
			defer cancel()
			data, ok := event.Data.(*TaskPR)
			if !ok {
				observed <- errUnexpectedTaskPREventData
				return nil
			}
			stored, readErr := store.GetTaskPRByRepoAndNumber(readCtx, data.TaskID, data.RepositoryID, data.PRNumber)
			if readErr == nil && (stored == nil || stored.PRTitle != data.PRTitle) {
				readErr = errTaskPRSnapshotNotCommitted
			}
			observed <- readErr
			return readErr
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = subscription.Unsubscribe() })

		status := resultEventStatus(watch, "committed title", time.Now().UTC())
		_, _, err = service.applyPRWatchStatusResult(ctx, watch, status, time.Now().UTC(), false)
		require.NoError(t, err)
		select {
		case eventErr := <-observed:
			require.NoError(t, eventErr, "synchronous subscribers must read the committed TaskPR")
		case <-time.After(time.Second):
			t.Fatal("TaskPR update event was not published")
		}
	})

	t.Run("discovery association", func(t *testing.T) {
		_, service, _, store := setupPollerTest(t)
		ctx := context.Background()
		seedTask(t, store, "task-discovery-event", false)
		watch := createResultEventWatch(t, service, "task-discovery-event", 0, "discovery-event")
		observed := make(chan error, 1)
		subscription, err := service.eventBus.Subscribe(events.GitHubTaskPRUpdated, func(readCtx context.Context, event *bus.Event) error {
			readCtx, cancel := context.WithTimeout(readCtx, 100*time.Millisecond)
			defer cancel()
			data, ok := event.Data.(*TaskPR)
			if !ok {
				observed <- errUnexpectedTaskPREventData
				return nil
			}
			stored, readErr := store.GetTaskPRByRepoAndNumber(readCtx, data.TaskID, data.RepositoryID, data.PRNumber)
			if readErr == nil && (stored == nil || stored.PRTitle != data.PRTitle) {
				readErr = errTaskPRSnapshotNotCommitted
			}
			observed <- readErr
			return readErr
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = subscription.Unsubscribe() })

		pr := resultEventStatus(watch, "discovered title", time.Now().UTC()).PR
		_, err = service.ApplyPRWatchDiscoveryResult(ctx, watch, pr)
		require.NoError(t, err)
		select {
		case eventErr := <-observed:
			require.NoError(t, eventErr, "association subscribers must read the committed TaskPR")
		case <-time.After(time.Second):
			t.Fatal("TaskPR association event was not published")
		}
	})
}

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.7
func TestPRWatchResultCommitFailurePublishesNoEvents(t *testing.T) {
	_, service, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-event-rollback", false)
	watch := createResultEventWatch(t, service, "task-event-rollback", 42, "rollback-event")

	_, err := store.db.Exec(`PRAGMA foreign_keys = ON`)
	require.NoError(t, err)
	_, err = store.db.Exec(`
		CREATE TABLE event_commit_parent (id INTEGER PRIMARY KEY);
		CREATE TABLE event_commit_child (
			parent_id INTEGER REFERENCES event_commit_parent(id) DEFERRABLE INITIALLY DEFERRED
		);
		CREATE TRIGGER fail_task_pr_result_commit AFTER UPDATE ON github_task_prs
		BEGIN
			INSERT INTO event_commit_child(parent_id) VALUES (999);
		END;
	`)
	require.NoError(t, err)

	var published atomic.Int32
	for _, subject := range []string{events.GitHubTaskPRUpdated, events.GitHubPRFeedback} {
		subscription, subscribeErr := service.eventBus.Subscribe(subject, func(context.Context, *bus.Event) error {
			published.Add(1)
			return nil
		})
		require.NoError(t, subscribeErr)
		t.Cleanup(func() { _ = subscription.Unsubscribe() })
	}

	_, _, err = service.applyPRWatchStatusResult(
		ctx, watch, resultEventStatus(watch, "must roll back", time.Now().UTC()), time.Now().UTC(), true,
	)
	require.Error(t, err, "deferred foreign key must fail the result commit")
	require.Zero(t, published.Load(), "events must not escape a rolled-back result")
	rows, err := store.ListTaskPRsByTask(ctx, watch.TaskID)
	require.NoError(t, err)
	require.Empty(t, rows)
}

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.7
func TestPRWatchRetirementWaitsForCommittedResultPublication(t *testing.T) {
	_, service, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-event-order", false)
	service.SetTaskIssueStore(&fakeTaskIssueStore{
		task:  &taskmodels.Task{ID: "task-event-order", WorkspaceID: testWorkspaceID},
		repos: []*taskmodels.TaskRepository{{RepositoryID: "repo-1"}},
		entities: map[string]*taskmodels.Repository{
			"repo-1": {ID: "repo-1", Provider: "github", ProviderOwner: "acme", ProviderName: "widget"},
		},
	})
	watch := createResultEventWatch(t, service, "task-event-order", 42, "event-order")

	publicationStarted := make(chan struct{})
	continuePublication := make(chan struct{})
	var published atomic.Int32
	subscription, err := service.eventBus.Subscribe(events.GitHubPRFeedback, func(context.Context, *bus.Event) error {
		if published.Add(1) == 1 {
			close(publicationStarted)
			<-continuePublication
		}
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })

	resultDone := make(chan error, 1)
	go func() {
		_, _, applyErr := service.applyPRWatchStatusResult(
			ctx, watch, resultEventStatus(watch, "ordered update", time.Now().UTC()), time.Now().UTC(), true,
		)
		resultDone <- applyErr
	}()
	select {
	case <-publicationStarted:
	case <-time.After(time.Second):
		t.Fatal("result publication did not start")
	}

	retirementDone := make(chan error, 1)
	retirementStarted := make(chan struct{})
	go func() {
		close(retirementStarted)
		retirementDone <- service.DeletePRWatch(ctx, watch.ID)
	}()
	<-retirementStarted
	select {
	case err := <-retirementDone:
		t.Fatalf("retirement returned before result delivery completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(continuePublication)
	require.NoError(t, <-resultDone)
	require.NoError(t, <-retirementDone)

	resumed, err := service.ResumePRWatch(ctx, watch.ID, testWorkspaceID)
	require.NoError(t, err)
	require.NotEqual(t, watch.ID, resumed.ID)
	accepted, changed, err := service.applyPRWatchStatusResult(
		ctx, watch, resultEventStatus(watch, "stale update", time.Now().UTC()), time.Now().UTC(), true,
	)
	require.NoError(t, err)
	require.False(t, accepted)
	require.False(t, changed)
	require.EqualValues(t, 1, published.Load(), "the old lifetime must not publish after resume")
}

func createResultEventWatch(t *testing.T, service *Service, taskID string, prNumber int, branch string) *PRWatch {
	t.Helper()
	watch, err := service.CreatePRWatchForWorkspace(
		context.Background(), testWorkspaceID, "session-"+taskID, taskID, "repo-1", "acme", "widget", prNumber, branch,
	)
	require.NoError(t, err)
	return watch
}

func resultEventStatus(watch *PRWatch, title string, updatedAt time.Time) *PRStatus {
	requiredReviews := 1
	return &PRStatus{
		PR: &PR{
			Number: 42, Title: title, State: "open", RepoOwner: "acme", RepoName: "widget",
			HTMLURL: "https://github.com/acme/widget/pull/42", HeadBranch: watch.Branch, BaseBranch: "main",
			HeadSHA: "event-head", AuthorLogin: "reviewer", CreatedAt: updatedAt.Add(-time.Hour), UpdatedAt: updatedAt,
		},
		ChecksState: "success", ReviewState: "approved", RequiredReviews: &requiredReviews,
	}
}

var (
	errUnexpectedTaskPREventData  = errors.New("unexpected TaskPR event data")
	errTaskPRSnapshotNotCommitted = errors.New("TaskPR event snapshot was not committed")
)
