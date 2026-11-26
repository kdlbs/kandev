package executor

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestPersistRuntimeModelMetadataPreservesConcurrentCursorCatalog(t *testing.T) {
	repo := &runtimeMetadataCASRepo{mockRepository: newMockRepository()}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	session := &models.TaskSession{
		ID: "session", TaskID: "task",
		Metadata: map[string]any{
			models.SessionMetaKeyRuntimeConfig: models.SessionRuntimeConfig{
				Model: "opus", ConfigOptions: map[string]string{"fast": "false"},
			},
		},
	}
	repo.sessions[session.ID] = cloneMockTaskSession(session)
	repo.interleave = func(ctx context.Context) error {
		return repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyRuntimeConfig, models.SessionRuntimeConfig{
			Model: "sonnet", ConfigOptions: map[string]string{"reasoning_effort": "high"},
		})
	}

	exec.persistRuntimeModelMetadata(t.Context(), session.ID, session, "sonnet")

	stored, err := repo.GetTaskSession(t.Context(), session.ID)
	require.NoError(t, err)
	cfg, ok := models.LoadSessionRuntimeConfig(stored.Metadata)
	require.True(t, ok)
	require.Equal(t, "sonnet", cfg.Model)
	require.Equal(t, map[string]string{"reasoning_effort": "high"}, cfg.ConfigOptions)
}

type runtimeMetadataCASRepo struct {
	*mockRepository
	interleave func(context.Context) error
}

func (r *runtimeMetadataCASRepo) SetSessionMetadataKeyIfAbsent(ctx context.Context, id, key string, value any) (bool, error) {
	return r.SetSessionMetadataKeyIfJSONValue(ctx, id, key, nil, value)
}

func (r *runtimeMetadataCASRepo) SetSessionMetadataKeyIfJSONValue(ctx context.Context, id, key string, expected, value any) (bool, error) {
	if r.interleave != nil {
		interleave := r.interleave
		r.interleave = nil
		if err := interleave(ctx); err != nil {
			return false, err
		}
	}
	row, err := r.GetTaskSession(ctx, id)
	if err != nil || !reflect.DeepEqual(row.Metadata[key], expected) {
		return false, err
	}
	return true, r.SetSessionMetadataKey(ctx, id, key, value)
}
