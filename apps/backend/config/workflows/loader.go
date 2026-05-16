package workflows

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/workflow/models"
	"gopkg.in/yaml.v3"
)

// templateYAML is the YAML-friendly representation of a workflow template.
type templateYAML struct {
	ID          string        `yaml:"id"`
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Hidden      bool          `yaml:"hidden,omitempty"`
	Steps       []stepDefYAML `yaml:"steps"`
}

// stepDefYAML is the YAML-friendly representation of a step definition.
type stepDefYAML struct {
	ID                    string         `yaml:"id"`
	Name                  string         `yaml:"name"`
	Position              int            `yaml:"position"`
	Color                 string         `yaml:"color"`
	Prompt                string         `yaml:"prompt,omitempty"`
	StageType             string         `yaml:"stage_type,omitempty"`
	IsStartStep           bool           `yaml:"is_start_step,omitempty"`
	ShowInCommandPanel    bool           `yaml:"show_in_command_panel,omitempty"`
	AllowManualMove       bool           `yaml:"allow_manual_move,omitempty"`
	AutoArchiveAfterHours int            `yaml:"auto_archive_after_hours,omitempty"`
	Events                stepEventsYAML `yaml:"events,omitempty"`
}

// stepEventsYAML is the YAML-friendly representation of step events.
type stepEventsYAML struct {
	OnEnter        []actionYAML `yaml:"on_enter,omitempty"`
	OnTurnStart    []actionYAML `yaml:"on_turn_start,omitempty"`
	OnTurnComplete []actionYAML `yaml:"on_turn_complete,omitempty"`
	OnExit         []actionYAML `yaml:"on_exit,omitempty"`

	// Phase 2 (ADR-0004) event-driven triggers. Actions on these triggers
	// are GenericActions (queue_run, clear_decisions,
	// queue_run_for_each_participant) — the YAML keys are the same.
	OnComment           []actionYAML `yaml:"on_comment,omitempty"`
	OnBlockerResolved   []actionYAML `yaml:"on_blocker_resolved,omitempty"`
	OnChildrenCompleted []actionYAML `yaml:"on_children_completed,omitempty"`
	OnApprovalResolved  []actionYAML `yaml:"on_approval_resolved,omitempty"`
	OnHeartbeat         []actionYAML `yaml:"on_heartbeat,omitempty"`
	OnBudgetAlert       []actionYAML `yaml:"on_budget_alert,omitempty"`
	OnAgentError        []actionYAML `yaml:"on_agent_error,omitempty"`
}

// actionYAML is the YAML-friendly representation of a step action.
type actionYAML struct {
	Type   string         `yaml:"type"`
	Config map[string]any `yaml:"config,omitempty"`
}

// HiddenTemplateIDs returns the set of template IDs marked `hidden: true`
// in their embedded YAML. Hidden templates are system-only flows
// (e.g. improve-kandev) that must not appear in management UI or pickers.
//
// The result is cached for the lifetime of the binary; the embedded YAML is
// static, and ListTemplates (the only caller) is on a hot path served on
// every picker / settings page load.
func HiddenTemplateIDs() (map[string]bool, error) {
	hiddenOnce.Do(func() {
		templates, err := LoadTemplates()
		if err != nil {
			hiddenErr = err
			return
		}
		m := make(map[string]bool, len(templates))
		for _, t := range templates {
			if t.Hidden {
				m[t.ID] = true
			}
		}
		hiddenIDs = m
	})
	return hiddenIDs, hiddenErr
}

var (
	hiddenOnce sync.Once
	hiddenIDs  map[string]bool
	hiddenErr  error
)

// LoadTemplates parses all embedded YAML files and returns workflow templates.
func LoadTemplates() ([]*models.WorkflowTemplate, error) {
	entries, err := embeddedFS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("workflows: read embedded dir: %w", err)
	}

	var templates []*models.WorkflowTemplate
	for _, entry := range entries {
		if entry.IsDir() || !isYAML(entry) {
			continue
		}
		tmpl, err := loadTemplate(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("workflows: load %s: %w", entry.Name(), err)
		}
		templates = append(templates, tmpl)
	}
	return templates, nil
}

func isYAML(entry fs.DirEntry) bool {
	name := entry.Name()
	return strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")
}

func loadTemplate(filename string) (*models.WorkflowTemplate, error) {
	data, err := embeddedFS.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var raw templateYAML
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	now := time.Now().UTC()
	tmpl := &models.WorkflowTemplate{
		ID:          raw.ID,
		Name:        raw.Name,
		Description: raw.Description,
		IsSystem:    true,
		Hidden:      raw.Hidden,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	for _, s := range raw.Steps {
		step, err := convertStep(s)
		if err != nil {
			return nil, err
		}
		tmpl.Steps = append(tmpl.Steps, step)
	}
	return tmpl, nil
}

func convertStep(s stepDefYAML) (models.StepDefinition, error) {
	events, err := convertEvents(s.Events)
	if err != nil {
		return models.StepDefinition{}, fmt.Errorf("step %q: %w", s.ID, err)
	}
	stage, err := convertStageType(s.StageType)
	if err != nil {
		return models.StepDefinition{}, fmt.Errorf("step %q: %w", s.ID, err)
	}
	return models.StepDefinition{
		ID:                    s.ID,
		Name:                  s.Name,
		Position:              s.Position,
		Color:                 s.Color,
		Prompt:                strings.TrimSpace(s.Prompt),
		Events:                events,
		AllowManualMove:       s.AllowManualMove,
		IsStartStep:           s.IsStartStep,
		ShowInCommandPanel:    s.ShowInCommandPanel,
		AutoArchiveAfterHours: s.AutoArchiveAfterHours,
		StageType:             stage,
	}, nil
}

// convertStageType validates a YAML stage_type field. Empty strings become
// the schema default ("custom").
func convertStageType(stage string) (models.StageType, error) {
	if stage == "" {
		return models.StageTypeCustom, nil
	}
	switch models.StageType(stage) {
	case models.StageTypeWork, models.StageTypeReview, models.StageTypeApproval, models.StageTypeCustom:
		return models.StageType(stage), nil
	}
	return "", fmt.Errorf("invalid stage_type %q", stage)
}

// validGenericAction is the YAML allow-list for actions appearing under any
// of the Phase 2 (ADR-0004) event-driven triggers. Restricting the set keeps
// templates honest while letting the engine reject malformed actions at
// compile time.
var validGenericAction = map[string]bool{
	string(models.GenericActionQueueRun):                   true,
	string(models.GenericActionClearDecisions):             true,
	string(models.GenericActionQueueRunForEachParticipant): true,
}

// Valid action types for each event trigger.
var (
	validOnEnter = map[string]bool{
		string(models.OnEnterEnablePlanMode):             true,
		string(models.OnEnterAutoStartAgent):             true,
		string(models.OnEnterResetAgentContext):          true,
		string(models.OnEnterClearDecisions):             true,
		string(models.OnEnterQueueRunForEachParticipant): true,
		string(models.OnEnterQueueRun):                   true,
	}
	validOnTurnStart = map[string]bool{
		string(models.OnTurnStartMoveToNext):     true,
		string(models.OnTurnStartMoveToPrevious): true,
		string(models.OnTurnStartMoveToStep):     true,
	}
	validOnTurnComplete = map[string]bool{
		string(models.OnTurnCompleteMoveToNext):      true,
		string(models.OnTurnCompleteMoveToPrevious):  true,
		string(models.OnTurnCompleteMoveToStep):      true,
		string(models.OnTurnCompleteDisablePlanMode): true,
	}
	validOnExit = map[string]bool{
		string(models.OnExitDisablePlanMode): true,
	}
)

func convertEvents(e stepEventsYAML) (models.StepEvents, error) {
	var events models.StepEvents
	for _, a := range e.OnEnter {
		if !validOnEnter[a.Type] {
			return events, fmt.Errorf("invalid on_enter action type %q", a.Type)
		}
		events.OnEnter = append(events.OnEnter, models.OnEnterAction{
			Type:   models.OnEnterActionType(a.Type),
			Config: a.Config,
		})
	}
	for _, a := range e.OnTurnStart {
		if !validOnTurnStart[a.Type] {
			return events, fmt.Errorf("invalid on_turn_start action type %q", a.Type)
		}
		events.OnTurnStart = append(events.OnTurnStart, models.OnTurnStartAction{
			Type:   models.OnTurnStartActionType(a.Type),
			Config: a.Config,
		})
	}
	for _, a := range e.OnTurnComplete {
		if !validOnTurnComplete[a.Type] {
			return events, fmt.Errorf("invalid on_turn_complete action type %q", a.Type)
		}
		events.OnTurnComplete = append(events.OnTurnComplete, models.OnTurnCompleteAction{
			Type:   models.OnTurnCompleteActionType(a.Type),
			Config: a.Config,
		})
	}
	for _, a := range e.OnExit {
		if !validOnExit[a.Type] {
			return events, fmt.Errorf("invalid on_exit action type %q", a.Type)
		}
		events.OnExit = append(events.OnExit, models.OnExitAction{
			Type:   models.OnExitActionType(a.Type),
			Config: a.Config,
		})
	}
	if err := convertGenericTriggers(e, &events); err != nil {
		return events, err
	}
	return events, nil
}

// convertGenericTriggers translates the YAML actions under the seven
// Phase 2 (ADR-0004) event-driven triggers into models.GenericAction lists.
func convertGenericTriggers(e stepEventsYAML, events *models.StepEvents) error {
	mappings := []struct {
		name string
		in   []actionYAML
		out  *[]models.GenericAction
	}{
		{"on_comment", e.OnComment, &events.OnComment},
		{"on_blocker_resolved", e.OnBlockerResolved, &events.OnBlockerResolved},
		{"on_children_completed", e.OnChildrenCompleted, &events.OnChildrenCompleted},
		{"on_approval_resolved", e.OnApprovalResolved, &events.OnApprovalResolved},
		{"on_heartbeat", e.OnHeartbeat, &events.OnHeartbeat},
		{"on_budget_alert", e.OnBudgetAlert, &events.OnBudgetAlert},
		{"on_agent_error", e.OnAgentError, &events.OnAgentError},
	}
	for _, m := range mappings {
		for _, a := range m.in {
			if !validGenericAction[a.Type] {
				return fmt.Errorf("invalid %s action type %q", m.name, a.Type)
			}
			*m.out = append(*m.out, models.GenericAction{
				Type:   models.GenericActionType(a.Type),
				Config: a.Config,
			})
		}
	}
	return nil
}
