package service

import (
	"context"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func mergeAfterRealRead(t *testing.T, change func(context.Context, *Service, *sqliterepo.Repository), initial map[string]interface{}, patch map[string]interface{}) *models.Task {
	t.Helper()
	services, _, gates := taskFieldServicePair(t)
	seedFieldTask(t, gates[0].Repository, initial)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	// The construction-supplied gate returns a genuine old snapshot after an owner commit.
	gates[0].armed.Store(true)
	result := make(chan metadataMergeOutcome, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		task, err := services[0].UpdateTaskMetadata(ctx, "field-task", patch)
		result <- metadataMergeOutcome{task, err}
	}()
	defer func() {
		cancel()
		select {
		case <-gates[0].release:
		default:
			close(gates[0].release)
		}
		<-done
	}()
	select {
	case <-gates[0].arrived:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	change(ctx, services[1], gates[1].Repository)
	close(gates[0].release)
	var got metadataMergeOutcome
	select {
	case got = <-result:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.NoError(t, got.err)
	return got.task
}

// @covers AC-TASKS-FIELD-UPDATES-001.9, AC-TASKS-FIELD-UPDATES-001.10
func TestTaskMetadataMergeReplacementControls(t *testing.T) {
	for _, later := range []string{"replacement", "snapshot"} {
		t.Run(later, func(t *testing.T) {
			services, _, gates := taskFieldServicePair(t)
			repo := gates[0].Repository
			seedFieldTask(t, repo, map[string]interface{}{"keep": "old"})
			ctx := context.Background()
			stale, err := repo.GetTask(ctx, "field-task")
			require.NoError(t, err)
			_, err = services[0].UpdateTaskMetadata(ctx, "field-task", map[string]interface{}{"alpha": "merged"})
			require.NoError(t, err)
			if later == "replacement" {
				_, err = services[1].UpdateTask(ctx, "field-task", &UpdateTaskRequest{Metadata: map[string]interface{}{"replacement": "current"}})
			} else {
				err = repo.UpdateTaskPreservingDeferredLaunch(ctx, stale)
			}
			require.NoError(t, err)
			current, err := repo.GetTask(ctx, "field-task")
			require.NoError(t, err)
			require.NotContains(t, current.Metadata, "alpha")
			_, err = services[0].UpdateTaskMetadata(ctx, "field-task", map[string]interface{}{"alpha": "later"})
			require.NoError(t, err)
			current, err = repo.GetTask(ctx, "field-task")
			require.NoError(t, err)
			require.Equal(t, "later", current.Metadata["alpha"])
			if later == "replacement" {
				require.Equal(t, "current", current.Metadata["replacement"])
			}
			_, err = services[1].UpdateTask(ctx, "field-task", &UpdateTaskRequest{Metadata: map[string]interface{}{}})
			require.NoError(t, err)
			current, err = repo.GetTask(ctx, "field-task")
			require.NoError(t, err)
			require.Empty(t, current.Metadata)
		})
	}
}

// @covers AC-TASKS-FIELD-UPDATES-001.10
func TestTaskMetadataMergeOwnersAndValues(t *testing.T) {
	t.Run("native_owners", func(t *testing.T) {
		input := map[string]interface{}{"ordinary": "allowed", models.MetaKeyDeferredLaunch: "forged", models.MetaKeyHandoffs: "forged", models.MetaKeyStepHandoffCarry: "forged", models.MetaKeyHandoffSource: "forged", models.MetaKeyOfficeCarrierCausationDepth: 0, models.MetaKeyAgentTitlePending: false, models.MetaKeyAgentTitleOwnerSessionID: "forged"}
		current := mergeAfterRealRead(t, func(ctx context.Context, _ *Service, r *sqliterepo.Repository) {
			stored, lost, err := r.SetTaskDeferredLaunchIfUnchanged(ctx, "field-task", sqliterepo.AbsentDeferredLaunch(), map[string]interface{}{"prompt": "current"})
			require.NoError(t, err)
			require.True(t, stored)
			require.False(t, lost)
			stored, _, err = r.SetTaskHandoffsIfUnchanged(ctx, "field-task", "", `[{"task_id":"current-child"}]`)
			require.NoError(t, err)
			require.True(t, stored)
			for key, value := range map[string]interface{}{models.MetaKeyStepHandoffCarry: "current-carry", models.MetaKeyHandoffSource: "current-source", models.MetaKeyOfficeCarrierCausationDepth: 3} {
				require.NoError(t, r.SetTaskMetadataKey(ctx, "field-task", key, value))
			}
			claimed, _, err := r.ClaimTaskTitleSession(ctx, "field-task", "owner")
			require.NoError(t, err)
			require.True(t, claimed)
		}, map[string]interface{}{models.MetaKeyAgentTitlePending: true, "nullable": nil}, input)
		require.Equal(t, "current", current.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})["prompt"])
		require.Equal(t, "current-child", current.Metadata[models.MetaKeyHandoffs].([]interface{})[0].(map[string]interface{})["task_id"])
		require.Equal(t, "current-carry", current.Metadata[models.MetaKeyStepHandoffCarry])
		require.Equal(t, "current-source", current.Metadata[models.MetaKeyHandoffSource])
		require.Equal(t, float64(3), current.Metadata[models.MetaKeyOfficeCarrierCausationDepth])
		require.True(t, models.IsAgentTitleOwner(current.Metadata, "owner"))
		require.Contains(t, current.Metadata, "nullable")
		require.Equal(t, "forged", input[models.MetaKeyDeferredLaunch])
		require.Equal(t, false, input[models.MetaKeyAgentTitlePending])
	})
	t.Run("removed_and_generated", func(t *testing.T) {
		current := mergeAfterRealRead(t, func(ctx context.Context, _ *Service, r *sqliterepo.Repository) {
			_, err := r.RemoveTaskMetadataKey(ctx, "field-task", models.MetaKeyDeferredLaunch)
			require.NoError(t, err)
			claimed, _, err := r.ClaimTaskTitleSession(ctx, "field-task", "owner")
			require.NoError(t, err)
			require.True(t, claimed)
			accepted, err := r.SetTaskTitleIfPending(ctx, "field-task", "owner", "Agent title")
			require.NoError(t, err)
			require.True(t, accepted)
		}, map[string]interface{}{models.MetaKeyAgentTitlePending: true, models.MetaKeyDeferredLaunch: map[string]interface{}{"prompt": "old"}}, map[string]interface{}{"ordinary": true, models.MetaKeyDeferredLaunch: "replay", models.MetaKeyAgentTitlePending: true})
		require.Equal(t, "Agent title", current.Title)
		require.NotContains(t, current.Metadata, models.MetaKeyDeferredLaunch)
		require.False(t, models.IsAgentTitlePending(current.Metadata))
	})
	t.Run("human_title_after_merge", func(t *testing.T) {
		services, _, gates := taskFieldServicePair(t)
		r := gates[0].Repository
		ctx := context.Background()
		seedFieldTask(t, r, map[string]interface{}{models.MetaKeyAgentTitlePending: true})
		claimed, _, err := r.ClaimTaskTitleSession(ctx, "field-task", "owner")
		require.NoError(t, err)
		require.True(t, claimed)
		_, err = services[0].UpdateTaskMetadata(ctx, "field-task", map[string]interface{}{"ordinary": true})
		require.NoError(t, err)
		title := "Human title"
		_, err = services[1].UpdateTask(ctx, "field-task", &UpdateTaskRequest{Title: &title})
		require.NoError(t, err)
		accepted, err := r.SetTaskTitleIfPending(ctx, "field-task", "owner", "Late generated title")
		require.NoError(t, err)
		require.False(t, accepted)
		current, err := r.GetTask(ctx, "field-task")
		require.NoError(t, err)
		require.Equal(t, title, current.Title)
		require.Equal(t, true, current.Metadata["ordinary"])
		require.False(t, models.IsAgentTitlePending(current.Metadata))
	})
	t.Run("scalar_and_workspace", func(t *testing.T) {
		patch := map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent", "group_id": "forged", "ordinary": true}}
		current := mergeAfterRealRead(t, func(ctx context.Context, s *Service, r *sqliterepo.Repository) {
			priority, description, position, assignee := "high", "Current", 23, "user-current"
			_, err := s.UpdateTask(ctx, "field-task", &UpdateTaskRequest{Priority: &priority, Description: &description, Position: &position, AssigneeUserID: &assignee})
			require.NoError(t, err)
			require.NoError(t, r.UpdateTaskState(ctx, "field-task", v1.TaskStateInProgress))
			require.NoError(t, r.SetTaskMetadataKey(ctx, "field-task", "workspace", map[string]interface{}{"mode": "shared_group", "group_id": "current"}))
		}, nil, patch)
		require.Equal(t, "high", current.Priority)
		require.Equal(t, "Current", current.Description)
		require.Equal(t, 23, current.Position)
		require.Equal(t, "user-current", current.AssigneeUserID)
		require.Equal(t, v1.TaskStateInProgress, current.State)
		require.Equal(t, "current", current.Metadata["workspace"].(map[string]interface{})["group_id"])
		require.Equal(t, "forged", patch["workspace"].(map[string]interface{})["group_id"])
	})
}
