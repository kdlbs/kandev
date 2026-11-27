package repository

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

type runtimeConfigStore interface {
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
	SetSessionMetadataKey(context.Context, string, string, any) error
}

type runtimeConfigCASStore interface {
	SetSessionMetadataKeyIfAbsent(context.Context, string, string, any) (bool, error)
	SetSessionMetadataKeyIfJSONValue(context.Context, string, string, any, any) (bool, error)
}

// UpdateSessionRuntimeConfig applies the change to the current metadata value
// so model-only writes and provider observations preserve each other's fields.
func UpdateSessionRuntimeConfig(
	ctx context.Context,
	store runtimeConfigStore,
	session *models.TaskSession,
	update func(*models.SessionRuntimeConfig),
) (models.SessionRuntimeConfig, string, error) {
	const maxAttempts = 4
	for range maxAttempts {
		config, _ := models.LoadSessionRuntimeConfig(session.Metadata)
		previousModel := config.Model
		update(&config)
		if config.IsZero() {
			return config, previousModel, nil
		}
		written, err := writeSessionRuntimeConfig(ctx, store, session, config)
		if err != nil || written {
			return config, previousModel, err
		}
		session, err = store.GetTaskSession(ctx, session.ID)
		if err != nil {
			return models.SessionRuntimeConfig{}, "", err
		}
		if session == nil {
			return models.SessionRuntimeConfig{}, "", models.ErrTaskSessionNotFound
		}
	}
	return models.SessionRuntimeConfig{}, "", fmt.Errorf("session runtime configuration changed concurrently")
}

func writeSessionRuntimeConfig(
	ctx context.Context,
	store runtimeConfigStore,
	session *models.TaskSession,
	config models.SessionRuntimeConfig,
) (bool, error) {
	key := models.SessionMetaKeyRuntimeConfig
	cas, ok := store.(runtimeConfigCASStore)
	if !ok {
		return true, store.SetSessionMetadataKey(ctx, session.ID, key, config)
	}
	if previous, present := session.Metadata[key]; present {
		return cas.SetSessionMetadataKeyIfJSONValue(ctx, session.ID, key, previous, config)
	}
	return cas.SetSessionMetadataKeyIfAbsent(ctx, session.ID, key, config)
}
