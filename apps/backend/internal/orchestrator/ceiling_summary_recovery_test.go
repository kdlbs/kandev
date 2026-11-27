package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/statussummary"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type ceilingSummaryReloadFailure struct {
	*tasksqlite.Repository
	fail bool
}

func (r *ceilingSummaryReloadFailure) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if r.fail {
		return nil, errors.New("transient publication reload failure")
	}
	return r.Repository.GetTask(ctx, id)
}

// The delivery boundary can lose a publication independently of the task
// reload. Successful deliveries exercise the real projector and persistence.
type ceilingSummaryDelivery struct {
	t         *testing.T
	projector *statussummary.Projector
	drop      bool
	updates   int
}

func (p *ceilingSummaryDelivery) PublishTaskUpdated(ctx context.Context, task *models.Task, _ ...string) {
	p.updates++
	if !p.drop {
		if err := p.projector.HandleEvent(ctx, bus.NewEvent(events.TaskUpdated, "test", map[string]interface{}{
			"task_id": task.ID, "workspace_id": task.WorkspaceID,
		})); err != nil {
			p.t.Fatal(err)
		}
	}
}

func (*ceilingSummaryDelivery) PublishTaskStateChanged(context.Context, *models.Task, v1.TaskState) {}
func (*ceilingSummaryDelivery) PublishTaskActivityIfChanged(context.Context, string)                {}

// @covers AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.16
func TestCeilingSummaryRecoveryLostEnqueueAndClear(t *testing.T) {
	for _, cleared := range []bool{false, true} {
		name := "enqueue"
		if cleared {
			name = "clear"
		}
		t.Run(name, func(t *testing.T) {
			svc, repo := newServiceWithRealRepo(t)
			ctx := context.Background()
			queuedAt := time.Now().UTC()
			record := models.CeilingRecordKeys(models.CeilingDeferral{
				Kind: models.CeilingLaunchStart, Origin: "automatic", ReasonCode: ceilingReasonRefused,
				QueuedAt: queuedAt, Payload: map[string]interface{}{},
			})
			task := &models.Task{ID: "recovery", WorkspaceID: "workspace-recovery", Title: "Recovery", State: v1.TaskStateScheduling}
			if cleared {
				task.Metadata = map[string]interface{}{models.MetaKeyDeferredLaunch: record}
			}
			if err := repo.CreateTask(ctx, task); err != nil {
				t.Fatal(err)
			}
			projector := statussummary.NewProjector(statussummary.ProjectorConfig{
				Store:            repo,
				ResolveWorkspace: func(context.Context, string) (string, error) { return task.WorkspaceID, nil },
				LoadLaunchQueue: func(ctx context.Context, id string) (*statussummary.LaunchQueueSummary, error) {
					current, err := repo.GetTask(ctx, id)
					if err != nil || !models.HasCeilingDeferredIntent(current) {
						return nil, err
					}
					return &statussummary.LaunchQueueSummary{
						QueuedAt: queuedAt, Reason: statussummary.LaunchQueueReasonSessionCapacity, Retrying: true,
					}, nil
				},
			})
			delivery := &ceilingSummaryDelivery{t: t, projector: projector}
			svc.SetTaskEventPublisher(delivery)
			svc.publishTaskUpdatedByID(ctx, task.ID)
			var next interface{} = record
			if cleared {
				next = nil
			}
			if err := repo.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyDeferredLaunch, next); err != nil {
				t.Fatal(err)
			}
			authoritative, err := repo.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			failing := &ceilingSummaryReloadFailure{Repository: repo, fail: true}
			svc.repo = failing
			svc.publishTaskUpdatedByID(ctx, task.ID) // original mutation publication is lost
			refresh := func() {
				tasks, listErr := repo.ListTasksWithCeilingDeferred(ctx)
				if listErr != nil {
					t.Fatal(listErr)
				}
				svc.refreshCeilingLaunchQueueSummaries(ctx, tasks)
			}
			assertQueue := func(want bool) {
				t.Helper()
				rows, loadErr := repo.LoadTaskStatusSummaries(ctx, []string{task.ID})
				if loadErr != nil || rows[task.ID] == nil || (rows[task.ID].LaunchQueue != nil) != want {
					t.Fatalf("queue presence want %v: rows=%+v err=%v", want, rows, loadErr)
				}
			}
			refresh() // a second reload failure must leave a durable retry candidate
			assertQueue(cleared)
			failing.fail = false
			delivery.drop = true
			refresh() // transport loss must also retain the recovery candidate
			assertQueue(cleared)
			delivery.drop = false
			refresh()
			assertQueue(!cleared)
			after, err := repo.GetTask(ctx, task.ID)
			if err != nil || after.State != authoritative.State || !reflect.DeepEqual(after.Metadata, authoritative.Metadata) {
				t.Fatalf("summary recovery changed authoritative state or record: %+v, %v", after, err)
			}
			if cleared {
				count := delivery.updates
				svc.drainDeferredCeilingLaunches(ctx)
				if delivery.updates != count {
					t.Fatal("successfully cleared summary remained in the recovery set")
				}
			}
		})
	}
}
