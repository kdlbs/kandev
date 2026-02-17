package models

import (
	"fmt"
	"maps"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	ExportVersion = 1
	ExportType    = "kandev_workflow"
)

// WorkflowExport is the portable JSON format for sharing workflows.
type WorkflowExport struct {
	Version   int                `json:"version"`
	Type      string             `json:"type"`
	Workflows []WorkflowPortable `json:"workflows"`
}

// WorkflowPortable is a workflow without instance-specific fields (IDs, timestamps).
type WorkflowPortable struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Steps       []StepPortable `json:"steps"`
}

// StepPortable is a workflow step without instance-specific fields.
type StepPortable struct {
	Name                  string     `json:"name"`
	Position              int        `json:"position"`
	Color                 string     `json:"color"`
	Prompt                string     `json:"prompt,omitempty"`
	Events                StepEvents `json:"events"`
	IsStartStep           bool       `json:"is_start_step"`
	AllowManualMove       bool       `json:"allow_manual_move"`
	AutoArchiveAfterHours int        `json:"auto_archive_after_hours,omitempty"`
}

// BuildWorkflowExport builds a portable WorkflowExport from domain models.
// stepsByWorkflow maps workflow ID → its steps (ordered by position).
func BuildWorkflowExport(workflows []*taskmodels.Workflow, stepsByWorkflow map[string][]*WorkflowStep) *WorkflowExport {
	portable := make([]WorkflowPortable, 0, len(workflows))
	for _, wf := range workflows {
		steps := stepsByWorkflow[wf.ID]
		portable = append(portable, buildWorkflowPortable(wf, steps))
	}
	return &WorkflowExport{
		Version:   ExportVersion,
		Type:      ExportType,
		Workflows: portable,
	}
}

func buildWorkflowPortable(wf *taskmodels.Workflow, steps []*WorkflowStep) WorkflowPortable {
	portableSteps := make([]StepPortable, 0, len(steps))
	// Build step ID → position map for converting move_to_step references.
	idToPos := make(map[string]int, len(steps))
	for _, s := range steps {
		idToPos[s.ID] = s.Position
	}
	for _, s := range steps {
		portableSteps = append(portableSteps, StepPortable{
			Name:                  s.Name,
			Position:              s.Position,
			Color:                 s.Color,
			Prompt:                s.Prompt,
			Events:                convertStepIDToPosition(s.Events, idToPos),
			IsStartStep:           s.IsStartStep,
			AllowManualMove:       s.AllowManualMove,
			AutoArchiveAfterHours: s.AutoArchiveAfterHours,
		})
	}
	return WorkflowPortable{
		Name:        wf.Name,
		Description: wf.Description,
		Steps:       portableSteps,
	}
}

// Validate checks that the export data is well-formed.
func (e *WorkflowExport) Validate() error {
	if e.Version != ExportVersion {
		return fmt.Errorf("unsupported export version: %d (expected %d)", e.Version, ExportVersion)
	}
	if e.Type != ExportType {
		return fmt.Errorf("unsupported export type: %q (expected %q)", e.Type, ExportType)
	}
	if len(e.Workflows) == 0 {
		return fmt.Errorf("export contains no workflows")
	}
	for i, wf := range e.Workflows {
		if wf.Name == "" {
			return fmt.Errorf("workflow %d: name is required", i)
		}
		positions := make(map[int]bool, len(wf.Steps))
		for j, step := range wf.Steps {
			if step.Name == "" {
				return fmt.Errorf("workflow %d step %d: name is required", i, j)
			}
			if positions[step.Position] {
				return fmt.Errorf("workflow %d: duplicate step position %d", i, step.Position)
			}
			positions[step.Position] = true
		}
		// Validate that move_to_step references point to valid positions.
		if err := validateStepPositionRefs(wf.Steps, positions); err != nil {
			return fmt.Errorf("workflow %d: %w", i, err)
		}
	}
	return nil
}

func validateStepPositionRefs(steps []StepPortable, validPositions map[int]bool) error {
	for _, step := range steps {
		for _, a := range step.Events.OnTurnStart {
			if a.Type == OnTurnStartMoveToStep {
				if err := checkPositionRef(a.Config, validPositions); err != nil {
					return fmt.Errorf("step %q on_turn_start: %w", step.Name, err)
				}
			}
		}
		for _, a := range step.Events.OnTurnComplete {
			if a.Type == OnTurnCompleteMoveToStep {
				if err := checkPositionRef(a.Config, validPositions); err != nil {
					return fmt.Errorf("step %q on_turn_complete: %w", step.Name, err)
				}
			}
		}
	}
	return nil
}

func checkPositionRef(config map[string]any, validPositions map[int]bool) error {
	if config == nil {
		return fmt.Errorf("move_to_step action missing config")
	}
	pos, exists := config["step_position"]
	if !exists {
		return fmt.Errorf("move_to_step action missing step_position")
	}
	posInt, ok := toInt(pos)
	if !ok {
		return fmt.Errorf("step_position has unexpected type %T", pos)
	}
	if !validPositions[posInt] {
		return fmt.Errorf("step_position %d does not match any step", posInt)
	}
	return nil
}

// convertStepIDToPosition rewrites move_to_step events: step_id → step_position.
func convertStepIDToPosition(events StepEvents, idToPos map[string]int) StepEvents {
	return remapStepEvents(events, "step_id", "step_position", func(v any) (any, bool) {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		pos, found := idToPos[s]
		return pos, found
	})
}

// ConvertPositionToStepID rewrites move_to_step events: step_position → step_id.
// posToID maps position → new step ID.
func ConvertPositionToStepID(events StepEvents, posToID map[int]string) StepEvents {
	return remapStepEvents(events, "step_position", "step_id", func(v any) (any, bool) {
		pos, ok := toInt(v)
		if !ok {
			return nil, false
		}
		id, found := posToID[pos]
		return id, found
	})
}

// remapStepEvents rewrites move_to_step config in OnTurnStart and OnTurnComplete actions,
// replacing fromKey with toKey using the provided lookup function.
func remapStepEvents(events StepEvents, fromKey, toKey string, lookup func(any) (any, bool)) StepEvents {
	result := StepEvents{
		OnEnter: append([]OnEnterAction{}, events.OnEnter...),
		OnExit:  append([]OnExitAction{}, events.OnExit...),
	}
	for _, a := range events.OnTurnStart {
		if a.Type == OnTurnStartMoveToStep {
			if cfg, ok := remapConfigKey(a.Config, fromKey, toKey, lookup); ok {
				a = OnTurnStartAction{Type: a.Type, Config: cfg}
			}
		}
		result.OnTurnStart = append(result.OnTurnStart, a)
	}
	for _, a := range events.OnTurnComplete {
		if a.Type == OnTurnCompleteMoveToStep {
			if cfg, ok := remapConfigKey(a.Config, fromKey, toKey, lookup); ok {
				a = OnTurnCompleteAction{Type: a.Type, Config: cfg}
			}
		}
		result.OnTurnComplete = append(result.OnTurnComplete, a)
	}
	return result
}

// remapConfigKey copies config, replaces fromKey with toKey using lookup.
func remapConfigKey(config map[string]any, fromKey, toKey string, lookup func(any) (any, bool)) (map[string]any, bool) {
	if config == nil {
		return nil, false
	}
	val, exists := config[fromKey]
	if !exists {
		return nil, false
	}
	newVal, found := lookup(val)
	if !found {
		return nil, false
	}
	cfg := make(map[string]any, len(config))
	maps.Copy(cfg, config)
	delete(cfg, fromKey)
	cfg[toKey] = newVal
	return cfg, true
}

func toInt(v any) (int, bool) {
	switch val := v.(type) {
	case float64:
		return int(val), true
	case int:
		return val, true
	default:
		return 0, false
	}
}
