package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestService_GetPromptByName_TrimsNameAndMapsMissingRows(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)

	prompt, err := svc.GetPromptByName(context.Background(), "  code-review  ")
	require.NoError(t, err)
	require.NotNil(t, prompt)
	require.Equal(t, "code-review", prompt.Name)

	_, err = svc.GetPromptByName(context.Background(), " missing-prompt ")
	require.ErrorIs(t, err, ErrPromptNotFound)

	_, err = svc.GetPromptByName(context.Background(), " CODE-REVIEW ")
	require.ErrorIs(t, err, ErrPromptNotFound)
}
