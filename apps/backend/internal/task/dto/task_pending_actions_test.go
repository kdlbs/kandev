package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kandev/kandev/internal/task/models"
)

func TestIsInputCapableSession(t *testing.T) {
	assert.False(t, IsInputCapableSession(nil))
	assert.True(t, IsInputCapableSession(&models.TaskSession{State: models.TaskSessionStateRunning}))
	assert.True(t, IsInputCapableSession(&models.TaskSession{State: models.TaskSessionStateWaitingForInput}))
	assert.False(t, IsInputCapableSession(&models.TaskSession{State: models.TaskSessionStateCompleted}))
}

func TestPendingActionPtr(t *testing.T) {
	actions := map[string]models.TaskPendingAction{"s1": models.TaskPendingActionClarification}

	assert.Nil(t, PendingActionPtr(nil, actions), "nil session ID yields nil")

	other := "s2"
	assert.Nil(t, PendingActionPtr(&other, actions), "session with no entry yields nil")

	s1 := "s1"
	got := PendingActionPtr(&s1, actions)
	if assert.NotNil(t, got) {
		assert.Equal(t, "clarification", *got)
	}
}

func TestTaskPendingActionPtr_NoInputCapableSessions(t *testing.T) {
	sessions := []*models.TaskSession{{ID: "s1", State: models.TaskSessionStateCompleted}}
	actions := map[string]models.TaskPendingAction{"s1": models.TaskPendingActionClarification}
	assert.Nil(t, TaskPendingActionPtr(sessions, actions), "terminal-state sessions never contribute, even with a stale actions entry")
}

func TestTaskPendingActionPtr_ClarificationOnly(t *testing.T) {
	sessions := []*models.TaskSession{{ID: "s1", State: models.TaskSessionStateRunning}}
	actions := map[string]models.TaskPendingAction{"s1": models.TaskPendingActionClarification}
	got := TaskPendingActionPtr(sessions, actions)
	if assert.NotNil(t, got) {
		assert.Equal(t, "clarification", *got)
	}
}

func TestTaskPendingActionPtr_PermissionWinsOverClarification(t *testing.T) {
	sessions := []*models.TaskSession{
		{ID: "s-clar", State: models.TaskSessionStateRunning},
		{ID: "s-perm", State: models.TaskSessionStateWaitingForInput},
	}
	actions := map[string]models.TaskPendingAction{
		"s-clar": models.TaskPendingActionClarification,
		"s-perm": models.TaskPendingActionPermission,
	}
	got := TaskPendingActionPtr(sessions, actions)
	if assert.NotNil(t, got) {
		assert.Equal(t, "permission", *got)
	}
}

func TestTaskPendingActionPtr_IgnoresNonInputCapableSessionEvenWithStaleAction(t *testing.T) {
	sessions := []*models.TaskSession{
		{ID: "running", State: models.TaskSessionStateRunning},
		{ID: "waiting", State: models.TaskSessionStateWaitingForInput},
		{ID: "starting", State: models.TaskSessionStateStarting},
	}
	actions := map[string]models.TaskPendingAction{
		"running":  models.TaskPendingActionClarification,
		"waiting":  models.TaskPendingActionPermission,
		"starting": models.TaskPendingActionPermission,
	}

	got := TaskPendingActionPtr(sessions, actions)
	if assert.NotNil(t, got) {
		assert.Equal(t, "permission", *got, "the starting session's action must not contribute despite the map entry")
	}
}

func TestTaskPendingActionPtr_NoSessionsOrNoActions(t *testing.T) {
	assert.Nil(t, TaskPendingActionPtr(nil, nil))
	sessions := []*models.TaskSession{{ID: "s1", State: models.TaskSessionStateRunning}}
	assert.Nil(t, TaskPendingActionPtr(sessions, nil), "no matching action entry yields nil")
}
