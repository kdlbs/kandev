package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/stretchr/testify/require"
)

func TestPromptWritesPublishInvalidation(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	publisher, ok := any(svc).(interface {
		SetEventBus(bus.EventBus, *logger.Logger)
	})
	require.True(t, ok, "saved prompt mutations must publish invalidation")
	publisher.SetEventBus(eventBus, log)
	var received []*bus.Event
	_, err = eventBus.Subscribe("prompts.changed", func(_ context.Context, event *bus.Event) error {
		received = append(received, event)
		return nil
	})
	require.NoError(t, err)
	ctx := context.Background()
	prompt, err := svc.CreatePromptForAgent(ctx, "live", "Private content")
	require.NoError(t, err)
	require.Len(t, received, 1)
	require.Empty(t, received[0].Data)
	_, err = svc.CreatePromptForAgent(ctx, "live", "duplicate")
	require.ErrorIs(t, err, ErrPromptAlreadyExists)
	require.Len(t, received, 1)
	_, err = svc.UpdatePromptContentForAgent(ctx, "live", "New content")
	require.NoError(t, err)
	require.Len(t, received, 2)
	allow := false
	_, err = svc.UpdatePromptWithPermission(ctx, prompt.ID, nil, nil, &allow)
	require.NoError(t, err)
	require.Len(t, received, 3)
	_, err = svc.UpdatePromptContentForAgent(ctx, "live", "Rejected")
	require.Error(t, err)
	require.Len(t, received, 3)
	require.NoError(t, svc.DeletePrompt(ctx, prompt.ID))
	require.Len(t, received, 4)
}

func TestPromptCreateTranslatesPostgresNameRace(t *testing.T) {
	svc := NewService(&raceRepo{createErr: &pgconn.PgError{Code: "23505", ConstraintName: "custom_prompts_name_key"}})
	_, err := svc.CreatePromptForAgent(context.Background(), "duplicate", "content")
	require.ErrorIs(t, err, ErrPromptAlreadyExists)
	internal := errors.New("database unavailable")
	svc = NewService(&raceRepo{createErr: internal})
	_, err = svc.CreatePromptForAgent(context.Background(), "prompt", "content")
	require.ErrorIs(t, err, internal)
}
