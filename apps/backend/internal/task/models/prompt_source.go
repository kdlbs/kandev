package models

import "errors"

const WorkflowSourcePromptSource = "prompt_source"

var ErrPromptSourceLocked = errors.New("workflow is managed by a prompt source and is read-only")
