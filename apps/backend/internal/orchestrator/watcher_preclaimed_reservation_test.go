package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type reservationTaskCreator struct{ repo *sqliterepo.Repository }

func (c reservationTaskCreator) CreateIssueTask(ctx context.Context, req *IssueTaskRequest) (*models.Task, error) {
	task := &models.Task{
		ID:          fmt.Sprintf("item-%v", req.Metadata["azure_devops_work_item_id"]),
		WorkspaceID: req.WorkspaceID, WorkflowID: req.WorkflowID, WorkflowStepID: req.WorkflowStepID,
		Title: req.Title, Metadata: req.Metadata, State: v1.TaskStateInProgress,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	return task, c.repo.CreateTask(ctx, task)
}

func TestDispatchWatcherEvent_PreclaimedItemsReturnAfterCapacityDrains(t *testing.T) {
	for _, terminal := range []string{"completed", "archived"} {
		t.Run(terminal, func(t *testing.T) {
			ctx := t.Context()
			repo := setupTestRepo(t)
			seedSession(t, repo, "seed", "seed-session", "step-1")
			database := sqlx.NewDb(repo.DB(), "sqlite3")
			store, err := azuredevops.NewStore(database, database)
			require.NoError(t, err)
			watch := &azuredevops.WorkItemWatch{ID: "watch", WorkspaceID: "ws1", ProjectID: "project", WIQL: "SELECT"}
			require.NoError(t, store.CreateWorkItemWatch(ctx, watch))
			source := NewAzureDevOpsWorkItemWatcherSource(store, nopLogger(t))
			creator := reservationTaskCreator{repo: repo}
			svc := &Service{logger: nopLogger(t), watcherTaskCount: repo, issueTaskCreator: creator}
			svc.watcherCoordinator = newTestCoordinator(t, nil, false, nil)
			svc.watcherCoordinator.SetTaskCreator(creator)
			poll := func() {
				for id := 1; id <= 5; id++ {
					claimed, reserveErr := store.ReserveWorkItemWatchTask(ctx, watch.ID, watch.Generation, watch.ProjectID, id, "item")
					require.NoError(t, reserveErr)
					if !claimed {
						continue
					}
					svc.dispatchWatcherEvent(ctx, source, &azuredevops.WorkItemWatchEvent{
						WatchID: watch.ID, WatchGeneration: watch.Generation, ProjectID: watch.ProjectID,
						WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-1",
						ReservationClaimed: true, MaxInflightTasks: ptrInt(2), WorkItem: azuredevops.WorkItem{ID: id},
					})
					// Join the complete dispatch, including attachment and slot release.
					require.Eventually(t, func() bool {
						svc.watcherMu.Lock()
						defer svc.watcherMu.Unlock()
						return len(svc.pendingByWatch) == 0
					}, 5*time.Second, time.Millisecond)
				}
			}
			poll()
			count, err := repo.CountOpenWatcherCreatedTasks(ctx, azureDevOpsWorkItemWatchMetadataKey, watch.ID)
			require.NoError(t, err)
			require.Equal(t, 2, count)
			if terminal == "archived" {
				require.NoError(t, repo.ArchiveTask(ctx, "item-1"))
			} else {
				require.NoError(t, repo.UpdateTaskState(ctx, "item-1", v1.TaskStateCompleted))
			}
			poll()
			third, err := repo.GetTask(ctx, "item-3")
			require.NoError(t, err, "the next poll must re-offer the capacity-refused item")
			require.Equal(t, "item-3", third.ID)
			count, err = repo.CountOpenWatcherCreatedTasks(ctx, azureDevOpsWorkItemWatchMetadataKey, watch.ID)
			require.NoError(t, err)
			require.Equal(t, 2, count)
		})
	}
}

func TestDispatchWatcherEvent_DoesNotReleaseUnclaimedReservation(t *testing.T) {
	source := &fakeWatcherSource{name: "linear", watchID: "watch", metadataKey: "linear", maxInflightTasks: ptrInt(2)}
	counter := newFakeWatcherCounter()
	counter.set("linear", "watch", 2)
	svc := nopServiceWithCounter(t, counter)
	svc.issueTaskCreator = &fakeTaskCreator{}
	svc.watcherCoordinator = newTestCoordinator(t, nil, false, nil)
	svc.dispatchWatcherEvent(t.Context(), source, "event")
	require.Zero(t, source.recordedRelease)
}

func TestDispatchWatcherEvent_OnlyReleasesOwnedAzureReservation(t *testing.T) {
	for _, kind := range []string{"work_item", "pull_request"} {
		for _, claimed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/claimed=%v", kind, claimed), func(t *testing.T) {
				ctx := t.Context()
				repo := setupTestRepo(t)
				database := sqlx.NewDb(repo.DB(), "sqlite3")
				store, err := azuredevops.NewStore(database, database)
				require.NoError(t, err)
				var source WatcherSource
				var event any
				var reserve func() (bool, error)
				if kind == "work_item" {
					watch := &azuredevops.WorkItemWatch{ID: "watch", WorkspaceID: "ws", ProjectID: "project", WIQL: "SELECT"}
					require.NoError(t, store.CreateWorkItemWatch(ctx, watch))
					source = NewAzureDevOpsWorkItemWatcherSource(store, nopLogger(t))
					event = &azuredevops.WorkItemWatchEvent{WatchID: watch.ID, WatchGeneration: watch.Generation,
						ProjectID: "project", ReservationClaimed: claimed, MaxInflightTasks: ptrInt(2), WorkItem: azuredevops.WorkItem{ID: 1}}
					reserve = func() (bool, error) {
						return store.ReserveWorkItemWatchTask(ctx, watch.ID, watch.Generation, "project", 1, "item")
					}
				} else {
					watch := &azuredevops.PullRequestWatch{ID: "watch", WorkspaceID: "ws", ProjectID: "project", AzureRepositoryID: "repo"}
					require.NoError(t, store.CreatePullRequestWatch(ctx, watch))
					source = NewAzureDevOpsPullRequestWatcherSource(store, nopLogger(t))
					event = &azuredevops.PullRequestWatchEvent{WatchID: watch.ID, WatchGeneration: watch.Generation,
						ProjectID: "project", AzureRepositoryID: "repo", ReservationClaimed: claimed, MaxInflightTasks: ptrInt(2), PullRequest: azuredevops.PullRequest{ID: 1}}
					reserve = func() (bool, error) {
						return store.ReservePullRequestWatchTask(ctx, watch.ID, watch.Generation, "project", "repo", 1, "pr")
					}
				}
				ok, err := reserve()
				require.NoError(t, err)
				require.True(t, ok)
				counter := newFakeWatcherCounter()
				counter.set(source.WatchMetadataKey(), "watch", 2)
				svc := nopServiceWithCounter(t, counter)
				svc.issueTaskCreator = &fakeTaskCreator{}
				svc.watcherCoordinator = newTestCoordinator(t, nil, false, nil)
				svc.dispatchWatcherEvent(ctx, source, event)
				ok, err = reserve()
				require.NoError(t, err)
				require.Equal(t, claimed, ok, "only an event that owns the refused claim may free it")
			})
		}
	}
}
