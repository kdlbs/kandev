package workflows

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/workflow/models"
	"gopkg.in/yaml.v3"
)

// templateYAML is the YAML-friendly representation of a workflow template.
type templateYAML struct {
	ID          string        `yaml:"id"`
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Steps       []stepDefYAML `yaml:"steps"`
}

// stepDefYAML is the YAML-friendly representation of a step definition.
type stepDefYAML struct {
	ID                    string         `yaml:"id"`
	Name                  string         `yaml:"name"`
	Position              int            `yaml:"position"`
	Color                 string         `yaml:"color"`
	Prompt                string         `yaml:"prompt,omitempty"`
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
}

// actionYAML is the YAML-friendly representation of a step action.
type actionYAML struct {
	Type   string         `yaml:"type"`
	Config map[string]any `yaml:"config,omitempty"`
}

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
	}, nil
}

// Valid action types for each event trigger.
var (
	validOnEnter = map[string]bool{
		string(models.OnEnterEnablePlanMode):    true,
		string(models.OnEnterAutoStartAgent):    true,
		string(models.OnEnterResetAgentContext): true,
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
	return events, nil
}
