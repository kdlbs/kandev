package models

import (
	"encoding/json"
	"errors"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"time"
)

var ErrPromptSourceLocked = taskmodels.ErrPromptSourceLocked
var ErrPromptSourceForbidden = errors.New("prompt source definition write is not permitted")
var ErrPromptSourceConflict = errors.New("prompt source changed; preview and retry")

type PromptSourceTemplate struct {
	ID          string            `json:"id"`
	Revision    int               `json:"revision"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Prompt      string            `json:"prompt,omitempty"`
	Steps       []json.RawMessage `json:"steps"`
}
type PromptSourceInstance struct {
	TemplateID   string            `json:"template_id"`
	WorkspaceID  string            `json:"workspace_id"`
	SourceLocked *bool             `json:"source_locked"`
	Variables    map[string]string `json:"variables"`
}
type PromptSourceImport struct {
	Templates []PromptSourceTemplate `json:"templates"`
	Instances []PromptSourceInstance `json:"instances"`
}
type PromptSourceRecord struct {
	ID              string            `json:"instance_id"`
	TemplateID      string            `json:"template_id"`
	WorkspaceID     string            `json:"workspace_id"`
	WorkflowID      string            `json:"workflow_id"`
	AppliedRevision int               `json:"applied_revision"`
	SourceLocked    bool              `json:"source_locked"`
	Variables       map[string]string `json:"variables"`
	UpdatedAt       time.Time         `json:"updated_at"`
}
type PromptSourceChange struct {
	PromptSourceRecord
	DesiredRevision     int      `json:"desired_revision"`
	DesiredSourceLocked bool     `json:"desired_source_locked"`
	ChangedFields       []string `json:"changed_fields"`
}
type PromptSourceResult struct {
	Preview   bool                 `json:"preview,omitempty"`
	Instances []PromptSourceChange `json:"instances"`
	Changed   int                  `json:"changed"`
}

// PromptSourcePlan is validated by the workflow service. The repository fences
// the complete observed membership and versions before atomically applying it.
type PromptSourcePlan struct {
	Template         PromptSourceTemplate
	Record           PromptSourceRecord
	ExpectedRecord   *PromptSourceRecord
	Workflow         *taskmodels.Workflow
	ExpectedWorkflow *taskmodels.Workflow
	ExistingSteps    []*WorkflowStep
	Steps            []*WorkflowStep
	Removed          []*WorkflowStep
	ChangedFields    []string
}
