package service

import (
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"testing"
)

func TestToolUpdateDoesNotRestoreRemovedPayload(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	svc := &Service{logger: log}
	message := &models.Message{Type: models.MessageTypeToolExecute, Metadata: map[string]any{"payload_retention": map[string]any{"version": 1}, "normalized": map[string]any{"kind": "shell_exec"}}}
	svc.applyToolCallMessageUpdate(message, "complete", "secret", "title", streams.NewGeneric("tool", "secret"))
	require.NotContains(t, message.Metadata, "result")
	require.Equal(t, models.MessageTypeToolExecute, message.Type)
	require.Equal(t, "complete", message.Metadata["status"])
}
