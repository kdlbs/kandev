package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConversationSearchAdmission(t *testing.T) {
	gate := conversationAdmission{active: make(map[string]bool)}
	require.True(t, gate.acquire("one"))
	require.False(t, gate.acquire("one"))
	for _, id := range []string{"two", "three", "four"} {
		require.True(t, gate.acquire(id))
	}
	require.False(t, gate.acquire("five"))
	gate.release("one")
	require.True(t, gate.acquire("five"))
}
func TestConversationSearchAuthorizesWorkspace(t *testing.T) {
	svc, _, _ := createTestService(t)
	_, _, err := svc.SearchTaskConversations(ctxAs("outsider"), "missing-workspace", "needle", 20)
	require.Error(t, err)
}
