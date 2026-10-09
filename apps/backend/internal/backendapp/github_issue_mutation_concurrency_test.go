package backendapp

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.2, AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.3
func TestGitHubIssueMutationConcurrentSQLite(t *testing.T) {
	for _, number := range []int{42, 0} {
		for _, order := range []string{"merge_first_delayed", "issue_first", "sequential_merge_first"} {
			t.Run(fmt.Sprintf("issue_%d_%s", number, order), func(t *testing.T) { runIssueMutationConcurrent(t, number, order) })
		}
	}
}
func runIssueMutationConcurrent(t *testing.T, number int, order string) {
	t.Helper()
	var mergeGate *issueMutationTaskHook
	h := newIssueMutationHarness(t, func(i int, r *sqliterepo.Repository) repository.TaskRepository {
		if i == 0 {
			return r
		}
		mergeGate = &issueMutationTaskHook{TaskRepository: r, arrived: make(chan struct{}), release: make(chan struct{})}
		return mergeGate
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	task, err := h.repos[0].GetTask(ctx, "issue-task")
	require.NoError(t, err)
	task.Metadata = issueMutationMetadata(7)
	require.NoError(t, h.repos[0].UpdateTask(ctx, task))
	baseline, err := h.repos[0].GetTask(ctx, task.ID)
	require.NoError(t, err)
	gh, gate := issueMutationGitHub(t, h.services[0])
	merge := func() {
		updated, mergeErr := h.services[1].UpdateTaskMetadata(ctx, task.ID, map[string]interface{}{"alpha": "accepted", models.MetaKeyPortForwardingEnabled: true})
		require.NoError(t, mergeErr)
		require.Equal(t, "accepted", updated.Metadata["alpha"])
		require.Equal(t, true, updated.Metadata[models.MetaKeyPortForwardingEnabled])
	}
	switch order {
	case "merge_first_delayed":
		gate.armed.Store(true)
		result := make(chan error, 1)
		workers.Add(1)
		go func() { defer workers.Done(); result <- runIssueMutation(ctx, gh, number) }()
		select {
		case <-gate.arrived:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		merge()
		close(gate.release)
		select {
		case err = <-result:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	case "issue_first":
		mergeGate.armed.Store(true)
		result := make(chan error, 1)
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := h.services[1].UpdateTaskMetadata(ctx, task.ID, map[string]interface{}{"alpha": "accepted", models.MetaKeyPortForwardingEnabled: true})
			result <- err
		}()
		select {
		case <-mergeGate.arrived:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		require.NoError(t, runIssueMutation(ctx, gh, number))
		close(mergeGate.release)
		require.NoError(t, <-result)
	case "sequential_merge_first":
		merge()
		require.NoError(t, runIssueMutation(ctx, gh, number))
	}
	stored, err := h.repos[1].GetTask(ctx, task.ID)
	require.NoError(t, err)
	assertIssueMutationIdentity(t, stored.Metadata, number)
	require.Equal(t, "untouched", stored.Metadata["keep"])
	require.Equal(t, "accepted", stored.Metadata["alpha"])
	require.Equal(t, true, stored.Metadata[models.MetaKeyPortForwardingEnabled])
	for _, eb := range h.buses {
		published := eb.snapshot()
		require.Len(t, published, 1)
		require.Equal(t, events.TaskUpdated, published[0].Type)
		data := published[0].Data.(map[string]interface{})
		require.Equal(t, task.ID, data["task_id"])
	}
	baseline.Metadata = stored.Metadata
	baseline.UpdatedAt = stored.UpdatedAt
	require.Equal(t, baseline, stored)
}
