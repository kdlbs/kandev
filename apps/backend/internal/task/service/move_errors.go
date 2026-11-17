package service

import "errors"

var (
	ErrTaskMoveArchived           = errors.New("archived tasks cannot be moved")
	ErrTaskMoveActiveSession      = errors.New("task has an active session")
	ErrTaskMoveDifferentWorkspace = errors.New("target workflow is in a different workspace")
	ErrTaskMoveWorkflowStep       = errors.New("target workflow step does not belong to target workflow")
)
