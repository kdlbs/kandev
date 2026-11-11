package backendapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type conversationStore struct{}

func (conversationStore) ConversationSearchReady(context.Context) bool { return true }
func (conversationStore) BackfillConversationSearch(context.Context, int) (bool, error) {
	return true, nil
}
func (conversationStore) ProcessConversationSearchBatch(context.Context, bool) (int, error) {
	return 0, nil
}
func TestConversationSearchRegistersRuntimeCleanup(t *testing.T) {
	var stop func() error
	startConversationSearch(t.Context(), conversationStore{}, nil, func(cleanup func() error) { stop = cleanup })
	require.NotNil(t, stop)
	require.NoError(t, stop())
	require.NoError(t, stop())
}
