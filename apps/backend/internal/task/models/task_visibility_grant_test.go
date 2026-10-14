package models

import "testing"

func TestTaskVisibilityGrantBindsExactManualSessionAndWorkspace(t *testing.T) {
	task := &Task{ID: "coordinator-task", WorkspaceID: "workspace-a", Origin: TaskOriginManual}
	grant := map[string]interface{}{
		"version": float64(1), "task_id": task.ID, "session_id": "session-a", "workspace_id": task.WorkspaceID,
	}
	session := &TaskSession{ID: "session-a", TaskID: task.ID, Metadata: map[string]interface{}{
		SessionMetaKeyTaskVisibilityGrant: grant,
	}}
	if !HasTaskVisibilityGrant(task, session) {
		t.Fatal("exact manual task/session/workspace grant was denied")
	}
	for name, change := range map[string]func(*Task, *TaskSession){
		"other session":   func(_ *Task, s *TaskSession) { s.ID = "session-b" },
		"other task":      func(_ *Task, s *TaskSession) { s.TaskID = "other-task" },
		"other workspace": func(task *Task, _ *TaskSession) { task.WorkspaceID = "workspace-b" },
		"office origin":   func(task *Task, _ *TaskSession) { task.Origin = TaskOriginAgentCreated },
		"missing grant":   func(_ *Task, s *TaskSession) { delete(s.Metadata, SessionMetaKeyTaskVisibilityGrant) },
	} {
		t.Run(name, func(t *testing.T) {
			copyTask := *task
			copySession := *session
			copySession.Metadata = map[string]interface{}{SessionMetaKeyTaskVisibilityGrant: grant}
			change(&copyTask, &copySession)
			if HasTaskVisibilityGrant(&copyTask, &copySession) {
				t.Fatal("nonmatching grant was accepted")
			}
		})
	}
}
