package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type metadataMergeOutcome struct {
	task *models.Task
	err  error
}

func runMetadataMergePair(t *testing.T, first int, title bool) {
	t.Helper()
	services, buses, gates := taskFieldServicePair(t)
	repo := gates[0].Repository
	seedFieldTask(t, repo, map[string]interface{}{"unrelated": "keep"})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	human := "Current human title"
	ops := [2]func() (*models.Task, error){
		func() (*models.Task, error) {
			return services[0].UpdateTaskMetadata(ctx, "field-task", map[string]interface{}{"alpha": "accepted"})
		},
		func() (*models.Task, error) {
			return services[1].UpdateTaskMetadata(ctx, "field-task", map[string]interface{}{models.MetaKeyPortForwardingEnabled: true})
		},
	}
	if title {
		ops[0] = func() (*models.Task, error) {
			return services[0].UpdateTask(ctx, "field-task", &UpdateTaskRequest{Title: &human})
		}
	}
	results := [2]chan metadataMergeOutcome{make(chan metadataMergeOutcome, 1), make(chan metadataMergeOutcome, 1)}
	for i := range services {
		buses[i].ClearEvents()
		gates[i].armed.Store(true)
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			task, err := ops[i]()
			results[i] <- metadataMergeOutcome{task, err}
		}(i)
	}
	for _, gate := range gates {
		select {
		case <-gate.arrived:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for _, i := range []int{first, 1 - first} {
		close(gates[i].release)
		select {
		case result := <-results[i]:
			require.NoError(t, result.err)
			require.NotNil(t, result.task)
			published := buses[i].GetPublishedEvents()
			require.Len(t, published, 1)
			require.Equal(t, events.TaskUpdated, published[0].Type)
			data := published[0].Data.(map[string]interface{})
			require.Equal(t, result.task.Title, data["title"])
			require.Equal(t, result.task.Metadata, data["metadata"])
			require.Equal(t, result.task.UpdatedAt.Format(time.RFC3339Nano), data["updated_at"])
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	stored, err := repo.GetTask(ctx, "field-task")
	require.NoError(t, err)
	require.Equal(t, "Original description", stored.Description)
	require.Equal(t, "medium", stored.Priority)
	require.Equal(t, "keep", stored.Metadata["unrelated"])
	require.Equal(t, true, stored.Metadata[models.MetaKeyPortForwardingEnabled])
	if title {
		require.Equal(t, human, stored.Title)
	} else {
		require.Equal(t, "accepted", stored.Metadata["alpha"])
	}
}

// @covers AC-TASKS-FIELD-UPDATES-001.8
func TestTaskMetadataMergeConcurrentSQLite(t *testing.T) {
	for first := range 2 {
		t.Run(fmt.Sprintf("first_%d", first), func(t *testing.T) { runMetadataMergePair(t, first, false) })
	}
	t.Run("sequential_same_key", func(t *testing.T) {
		services, _, gates := taskFieldServicePair(t)
		seedFieldTask(t, gates[0].Repository, map[string]interface{}{"unrelated": "keep"})
		for i, patch := range []map[string]interface{}{{"alpha": "old"}, {"alpha": "later", models.MetaKeyPortForwardingEnabled: true}} {
			_, err := services[i].UpdateTaskMetadata(context.Background(), "field-task", patch)
			require.NoError(t, err)
		}
		stored, err := gates[0].Repository.GetTask(context.Background(), "field-task")
		require.NoError(t, err)
		require.Equal(t, "later", stored.Metadata["alpha"])
		require.Equal(t, "keep", stored.Metadata["unrelated"])
		require.Equal(t, true, stored.Metadata[models.MetaKeyPortForwardingEnabled])
	})
}

// @covers AC-TASKS-FIELD-UPDATES-001.9, AC-TASKS-FIELD-UPDATES-001.12
func TestTaskMetadataMergeOrdinaryFieldInteraction(t *testing.T) {
	for first := range 2 {
		t.Run(fmt.Sprintf("title_first_%d", first), func(t *testing.T) { runMetadataMergePair(t, first, true) })
	}
}
