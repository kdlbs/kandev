package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"strings"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"gopkg.in/yaml.v3"
)

const (
	LegacyExportVersion         = 1
	ExportVersion               = 2
	ProfileOptionsExportVersion = 3
	ExportType                  = "kandev_workflow"
)

// WorkflowExport is the portable format for sharing workflows.
type WorkflowExport struct {
	Version   int                `json:"version" yaml:"version"`
	Type      string             `json:"type" yaml:"type"`
	Workflows []WorkflowPortable `json:"workflows" yaml:"workflows"`
}

// AgentProfilePortable stores enough agent profile info for cross-workspace matching.
type AgentProfilePortable struct {
	AgentName      string             `json:"agent_name" yaml:"agent_name"`
	Model          string             `json:"model,omitempty" yaml:"model,omitempty"`
	Mode           string             `json:"mode,omitempty" yaml:"mode,omitempty"`
	ConfigOptions  *map[string]string `json:"config_options,omitempty" yaml:"config_options,omitempty"`
	optionsPresent bool               `json:"-" yaml:"-"`
}

func (p *AgentProfilePortable) UnmarshalJSON(data []byte) error {
	type plain AgentProfilePortable
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var result plain
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	*p = AgentProfilePortable(result)
	_, p.optionsPresent = fields["config_options"]
	return nil
}

func (p *AgentProfilePortable) UnmarshalYAML(node *yaml.Node) error {
	type plain AgentProfilePortable
	var result plain
	if err := node.Decode(&result); err != nil {
		return err
	}
	*p = AgentProfilePortable(result)
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != "config_options" {
			continue
		}
		p.optionsPresent = true
		value := node.Content[i+1]
		if value.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(value.Content); j += 2 {
				if value.Content[j].Tag != "!!str" || value.Content[j+1].Tag != "!!str" {
					return fmt.Errorf("config_options must contain string keys and values")
				}
			}
		}
	}
	return nil
}

func validatePortableProfile(p *AgentProfilePortable, version int) error {
	if p == nil {
		return nil
	}
	if version < ProfileOptionsExportVersion {
		if p.optionsPresent || p.ConfigOptions != nil {
			return fmt.Errorf("config_options requires export version 3")
		}
		return nil
	}
	if p.ConfigOptions == nil {
		return fmt.Errorf("config_options must be an object")
	}
	for key, value := range *p.ConfigOptions {
		if key == "" || value == "" || strings.TrimSpace(key) != key || strings.TrimSpace(value) != value || key == "model" || key == "mode" {
			return fmt.Errorf("config_options contains invalid key or value")
		}
	}
	return nil
}

// AgentProfileResolver resolves an agent profile ID to its portable representation.
type AgentProfileResolver func(profileID string) *AgentProfilePortable

// AgentProfileMatcher finds a matching agent profile ID by agent name, model,
// and mode. currentID, when non-empty, is the profile already bound (e.g.
// during workflow-sync reconciliation); implementations should keep it when
// it still matches the descriptor even if the profile was since disabled -
// disabling only hides a profile from new selection, it doesn't touch
// existing bindings (docs/specs/agents/requirements/profile-disable.md).
type AgentProfileMatcher func(agentName, model, mode, currentID string) string

// AgentProfileLookup reports candidate ambiguity and storage failure to sync.
type AgentProfileLookup func(profile *AgentProfilePortable, currentID string) ProfileMatch

type ProfileMatch struct {
	ID     string
	Reason string
}

// WorkflowSessionTargetPortable stores a target with a position instead of a
// workflow-local step ID so an export can be imported into another workflow.
type WorkflowSessionTargetPortable struct {
	Kind         WorkflowSessionTargetKind `json:"kind" yaml:"kind"`
	StepPosition *int                      `json:"step_position,omitempty" yaml:"step_position,omitempty"`
}

// WorkflowPortable is a workflow without instance-specific fields (IDs, timestamps).
type WorkflowPortable struct {
	Name         string                `json:"name" yaml:"name"`
	Description  string                `json:"description,omitempty" yaml:"description,omitempty"`
	Prompt       string                `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	AgentProfile *AgentProfilePortable `json:"agent_profile,omitempty" yaml:"agent_profile,omitempty"`
	Steps        []StepPortable        `json:"steps" yaml:"steps"`
}

// StepPortable is a workflow step without instance-specific fields.
type StepPortable struct {
	Name                         string                                       `json:"name" yaml:"name"`
	Position                     int                                          `json:"position" yaml:"position"`
	Color                        string                                       `json:"color" yaml:"color"`
	Prompt                       string                                       `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Events                       StepEvents                                   `json:"events" yaml:"events"`
	IsStartStep                  bool                                         `json:"is_start_step" yaml:"is_start_step"`
	ShowInCommandPanel           bool                                         `json:"show_in_command_panel" yaml:"show_in_command_panel"`
	AllowManualMove              bool                                         `json:"allow_manual_move" yaml:"allow_manual_move"`
	AutoArchiveAfterHours        int                                          `json:"auto_archive_after_hours,omitempty" yaml:"auto_archive_after_hours,omitempty"`
	AgentProfile                 *AgentProfilePortable                        `json:"agent_profile,omitempty" yaml:"agent_profile,omitempty"`
	ProfileSessionStartPolicy    taskmodels.WorkflowProfileSessionStartPolicy `json:"profile_session_start_policy,omitempty" yaml:"profile_session_start_policy,omitempty"`
	ProfileSessionEndPolicy      taskmodels.WorkflowProfileSessionEndPolicy   `json:"profile_session_end_policy,omitempty" yaml:"profile_session_end_policy,omitempty"`
	DisableUnclassifiedFallback  bool                                         `json:"disable_unclassified_fallback" yaml:"disable_unclassified_fallback"`
	AutoAdvanceRequiresSignal    bool                                         `json:"auto_advance_requires_signal" yaml:"auto_advance_requires_signal"`
	CancelTriggersTurnComplete   bool                                         `json:"cancel_triggers_turn_complete" yaml:"cancel_triggers_turn_complete"`
	CompleteTaskOnEnter          bool                                         `json:"complete_task_on_enter" yaml:"complete_task_on_enter"`
	WIPLimit                     int                                          `json:"wip_limit,omitempty" yaml:"wip_limit,omitempty"`
	PullFromStepPosition         *int                                         `json:"pull_from_step_position,omitempty" yaml:"pull_from_step_position,omitempty"`
	completionTaskOnEnterDecoded bool                                         `json:"-" yaml:"-"`
	completionTaskOnEnterPresent bool                                         `json:"-" yaml:"-"`
	SessionTarget                *WorkflowSessionTargetPortable               `json:"session_target,omitempty" yaml:"session_target,omitempty"`
}

// UnmarshalJSON records whether a decoded document explicitly supplied the
// version-2 completion field. The domain field remains a bool so callers that
// build exports in Go keep the same ergonomic API.
func (s *StepPortable) UnmarshalJSON(data []byte) error {
	type plainStepPortable StepPortable
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, ok := fields["complete_task_on_enter"]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("complete_task_on_enter must be a boolean, not null")
	}
	if raw, ok := fields["disable_unclassified_fallback"]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("disable_unclassified_fallback must be a boolean, not null")
	}
	var decoded plainStepPortable
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = StepPortable(decoded)
	_, s.completionTaskOnEnterPresent = fields["complete_task_on_enter"]
	s.completionTaskOnEnterDecoded = true
	return nil
}

// UnmarshalYAML records whether a decoded document explicitly supplied the
// version-2 completion field.
func (s *StepPortable) UnmarshalYAML(node *yaml.Node) error {
	type plainStepPortable StepPortable
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			field := node.Content[i].Value
			if field != "complete_task_on_enter" && field != "disable_unclassified_fallback" {
				continue
			}
			valueNode := node.Content[i+1]
			if valueNode.Tag == "!!null" || valueNode.ShortTag() == "!!null" {
				return fmt.Errorf("%s must be a boolean, not null", field)
			}
		}
	}
	var decoded plainStepPortable
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*s = StepPortable(decoded)
	present := false
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "complete_task_on_enter" {
				present = true
				break
			}
		}
	}
	s.completionTaskOnEnterPresent = present
	s.completionTaskOnEnterDecoded = true
	return nil
}

func (s StepPortable) completionTaskOnEnterIsExplicit() bool {
	if !s.completionTaskOnEnterDecoded {
		return true
	}
	return s.completionTaskOnEnterPresent
}

func (s *StepPortable) markCompletionTaskOnEnterExplicit() {
	s.completionTaskOnEnterPresent = true
}

// BuildWorkflowExport builds a portable WorkflowExport from domain models.
// stepsByWorkflow maps workflow ID → its steps (ordered by position).
// resolveProfile converts agent profile IDs to portable form (may be nil).
// Invalid target references return nil for compatibility with older callers;
// service callers should use BuildWorkflowExportWithError to surface the
// configuration error to the user.
func BuildWorkflowExport(workflows []*taskmodels.Workflow, stepsByWorkflow map[string][]*WorkflowStep, resolveProfile AgentProfileResolver) *WorkflowExport {
	export, err := BuildWorkflowExportWithError(workflows, stepsByWorkflow, resolveProfile)
	if err != nil {
		return nil
	}
	return export
}

// BuildWorkflowExportWithError builds a portable export and rejects unresolved
// step targets before they can produce an import-invalid document.
func BuildWorkflowExportWithError(workflows []*taskmodels.Workflow, stepsByWorkflow map[string][]*WorkflowStep, resolveProfile AgentProfileResolver) (*WorkflowExport, error) {
	portable := make([]WorkflowPortable, 0, len(workflows))
	version := LegacyExportVersion
	for _, wf := range workflows {
		steps := stepsByWorkflow[wf.ID]
		for _, step := range steps {
			if version < ExportVersion && step != nil && (step.SessionTarget != nil || step.CompleteTaskOnEnter) {
				version = ExportVersion
			}
		}
		workflow, err := buildWorkflowPortable(wf, steps, resolveProfile)
		if err != nil {
			return nil, err
		}
		if workflowHasProfiles(&workflow) {
			version = ProfileOptionsExportVersion
		}
		portable = append(portable, workflow)
	}
	return &WorkflowExport{
		Version:   version,
		Type:      ExportType,
		Workflows: portable,
	}, nil
}

func workflowHasProfiles(workflow *WorkflowPortable) bool {
	if workflow.AgentProfile != nil {
		return true
	}
	for _, step := range workflow.Steps {
		if step.AgentProfile != nil {
			return true
		}
		for _, action := range step.Events.OnEnter {
			if action.Type == OnEnterRunCodeReview && action.Config[ReviewAgentProfilePortableKey] != nil {
				return true
			}
		}
	}
	return false
}

func exportProfileOptions(profile *AgentProfilePortable) *AgentProfilePortable {
	if profile != nil && profile.ConfigOptions == nil {
		copy := *profile
		options := map[string]string{}
		copy.ConfigOptions = &options
		return &copy
	}
	return profile
}

func buildWorkflowPortable(wf *taskmodels.Workflow, steps []*WorkflowStep, resolveProfile AgentProfileResolver) (WorkflowPortable, error) {
	portableSteps := make([]StepPortable, 0, len(steps))
	// Build step ID → position map for converting move_to_step references.
	idToPos := make(map[string]int, len(steps))
	for _, s := range steps {
		idToPos[s.ID] = s.Position
	}
	for _, s := range steps {
		sessionTarget, err := buildPortableSessionTarget(s.SessionTarget, idToPos)
		if err != nil {
			return WorkflowPortable{}, fmt.Errorf("workflow %q step %q: %w", wf.Name, s.Name, err)
		}
		sp := StepPortable{
			Name:                        s.Name,
			Position:                    s.Position,
			Color:                       s.Color,
			Prompt:                      s.Prompt,
			Events:                      ConvertReviewProfileToPortable(convertStepIDToPosition(s.Events, idToPos), resolveProfile),
			IsStartStep:                 s.IsStartStep,
			ShowInCommandPanel:          s.ShowInCommandPanel,
			AllowManualMove:             s.AllowManualMove,
			AutoArchiveAfterHours:       s.AutoArchiveAfterHours,
			ProfileSessionStartPolicy:   taskmodels.NormalizeWorkflowProfileSessionStartPolicy(string(s.ProfileSessionStartPolicy)),
			ProfileSessionEndPolicy:     taskmodels.NormalizeWorkflowProfileSessionEndPolicy(string(s.ProfileSessionEndPolicy)),
			DisableUnclassifiedFallback: s.DisableUnclassifiedFallback,
			SessionTarget:               sessionTarget,
			AutoAdvanceRequiresSignal:   s.AutoAdvanceRequiresSignal,
			CancelTriggersTurnComplete:  s.CancelTriggersTurnComplete,
			CompleteTaskOnEnter:         s.CompleteTaskOnEnter,
			WIPLimit:                    s.WIPLimit,
		}
		if pos, ok := idToPos[s.PullFromStepID]; ok {
			sp.PullFromStepPosition = &pos
		}
		if resolveProfile != nil && s.AgentProfileID != "" {
			sp.AgentProfile = exportProfileOptions(resolveProfile(s.AgentProfileID))
		}
		portableSteps = append(portableSteps, sp)
	}

	wp := WorkflowPortable{
		Name:        wf.Name,
		Description: wf.Description,
		Prompt:      wf.Prompt,
		Steps:       portableSteps,
	}
	if resolveProfile != nil && wf.AgentProfileID != "" {
		wp.AgentProfile = exportProfileOptions(resolveProfile(wf.AgentProfileID))
	}
	return wp, nil
}

func buildPortableSessionTarget(target *WorkflowSessionTarget, idToPos map[string]int) (*WorkflowSessionTargetPortable, error) {
	if target == nil {
		return nil, nil
	}
	if err := ValidateWorkflowSessionTarget(target); err != nil {
		return nil, err
	}
	portable := &WorkflowSessionTargetPortable{Kind: target.Kind}
	if target.Kind == WorkflowSessionTargetStep {
		if position, ok := idToPos[target.StepID]; ok {
			portable.StepPosition = &position
		} else {
			return nil, fmt.Errorf("session target source step %q was not found", target.StepID)
		}
	}
	return portable, nil
}

// Validate checks that the export data is well-formed.
func (e *WorkflowExport) Validate() error {
	if e.Version != LegacyExportVersion && e.Version != ExportVersion && e.Version != ProfileOptionsExportVersion {
		return fmt.Errorf("unsupported export version: %d (expected 1, 2 or 3)", e.Version)
	}
	if e.Type != ExportType {
		return fmt.Errorf("unsupported export type: %q (expected %q)", e.Type, ExportType)
	}
	if err := e.NormalizeCompletionPolicy(); err != nil {
		return err
	}
	if len(e.Workflows) == 0 {
		return fmt.Errorf("export contains no workflows")
	}
	for i, wf := range e.Workflows {
		if wf.Name == "" {
			return fmt.Errorf("workflow %d: name is required", i)
		}
		if err := validatePortableProfile(wf.AgentProfile, e.Version); err != nil {
			return fmt.Errorf("workflow %d agent profile: %w", i, err)
		}
		positions := make(map[int]bool, len(wf.Steps))
		for j, step := range wf.Steps {
			if err := validatePortableProfile(step.AgentProfile, e.Version); err != nil {
				return fmt.Errorf("workflow %d step %d agent profile: %w", i, j, err)
			}
			for _, action := range step.Events.OnEnter {
				if action.Type != OnEnterRunCodeReview {
					continue
				}
				value, ok := action.Config[ReviewAgentProfilePortableKey]
				if !ok {
					continue
				}
				descriptor, err := ParsePortableProfile(value)
				if err != nil {
					return fmt.Errorf("workflow %d step %d review profile: %w", i, j, err)
				}
				if err := validatePortableProfile(descriptor, e.Version); err != nil {
					return fmt.Errorf("workflow %d step %d review profile: %w", i, j, err)
				}
			}
			if step.Name == "" {
				return fmt.Errorf("workflow %d step %d: name is required", i, j)
			}
			if positions[step.Position] {
				return fmt.Errorf("workflow %d: duplicate step position %d", i, step.Position)
			}
			positions[step.Position] = true
			if e.Version >= ExportVersion && !step.completionTaskOnEnterIsExplicit() {
				return fmt.Errorf("workflow %d step %d: complete_task_on_enter must be explicitly set", i, j)
			}
			if err := validateOnEnterActions(step); err != nil {
				return fmt.Errorf("workflow %d step %d: %w", i, j, err)
			}
			if step.WIPLimit < 0 {
				return fmt.Errorf("workflow %d step %d: wip_limit must be non-negative", i, j)
			}
		}
		for j, step := range wf.Steps {
			if err := validatePortableSessionTarget(e.Version, step, positions); err != nil {
				return fmt.Errorf("workflow %d step %d: %w", i, j, err)
			}
		}
		// Validate that move_to_step references point to valid positions.
		if err := validateStepPositionRefs(wf.Steps, positions); err != nil {
			return fmt.Errorf("workflow %d: %w", i, err)
		}
		if err := validatePullSourceRefs(wf.Steps, positions); err != nil {
			return fmt.Errorf("workflow %d: %w", i, err)
		}
		if err := validatePortableSessionTargets(wf.Steps); err != nil {
			return fmt.Errorf("workflow %d: %w", i, err)
		}
	}
	return nil
}

func validatePortableSessionTarget(version int, step StepPortable, validPositions map[int]bool) error {
	if step.SessionTarget == nil {
		return nil
	}
	if version == LegacyExportVersion {
		return fmt.Errorf("session_target requires export version %d", ExportVersion)
	}
	switch step.SessionTarget.Kind {
	case WorkflowSessionTargetInitial:
		if step.SessionTarget.StepPosition != nil {
			return fmt.Errorf("initial session target must not include step_position")
		}
	case WorkflowSessionTargetStep:
		if step.SessionTarget.StepPosition == nil {
			return fmt.Errorf("step session target requires step_position")
		}
		position := *step.SessionTarget.StepPosition
		if !validPositions[position] {
			return fmt.Errorf("session target step_position %d does not match any step", position)
		}
		if position >= step.Position {
			return fmt.Errorf("session target step_position %d must reference an earlier step", position)
		}
		if step.AgentProfile != nil {
			return fmt.Errorf("session_target cannot be combined with an agent_profile")
		}
	default:
		return fmt.Errorf("unknown session target kind %q", step.SessionTarget.Kind)
	}
	return nil
}

func validatePortableSessionTargets(steps []StepPortable) error {
	byPosition := make(map[int]StepPortable, len(steps))
	for _, step := range steps {
		byPosition[step.Position] = step
	}
	for _, step := range steps {
		if step.SessionTarget == nil || step.SessionTarget.Kind != WorkflowSessionTargetStep {
			continue
		}
		if step.SessionTarget.StepPosition == nil {
			return fmt.Errorf("step %q session target requires step_position", step.Name)
		}
		target := byPosition[*step.SessionTarget.StepPosition]
		if target.AgentProfile == nil || target.SessionTarget != nil {
			return fmt.Errorf("step %q session target must reference a direct-profile step", step.Name)
		}
	}
	return nil
}

// NormalizeCompletionPolicy converts version-1 exports at the import/sync
// boundary. Version 1 had no explicit completion field, so only its final
// step inherits the legacy terminal-name convention. Version-2 values are
// never inferred or changed.
func (e *WorkflowExport) NormalizeCompletionPolicy() error {
	if e.Version != LegacyExportVersion {
		return nil
	}
	for workflowIndex := range e.Workflows {
		workflow := &e.Workflows[workflowIndex]
		if len(workflow.Steps) == 0 {
			continue
		}
		maxPosition := workflow.Steps[0].Position
		for _, step := range workflow.Steps[1:] {
			if step.Position > maxPosition {
				maxPosition = step.Position
			}
		}
		for stepIndex := range workflow.Steps {
			step := &workflow.Steps[stepIndex]
			if step.completionTaskOnEnterDecoded && step.completionTaskOnEnterPresent {
				continue
			}
			step.CompleteTaskOnEnter = step.Position == maxPosition && IsTerminalStepName(step.Name)
			step.markCompletionTaskOnEnterExplicit()
		}
	}
	return nil
}

func validatePullSourceRefs(steps []StepPortable, validPositions map[int]bool) error {
	pullSources := make(map[int]int, len(steps))
	for _, step := range steps {
		if step.PullFromStepPosition == nil {
			continue
		}
		pos := *step.PullFromStepPosition
		if pos == step.Position {
			return fmt.Errorf("step %q: pull_from_step_position cannot reference itself", step.Name)
		}
		if !validPositions[pos] {
			return fmt.Errorf("step %q: pull_from_step_position %d does not match any step", step.Name, pos)
		}
		pullSources[step.Position] = pos
	}
	if hasPullSourceCycle(pullSources) {
		return fmt.Errorf("pull_from_step_position cannot create a pull cycle")
	}
	return nil
}

func hasPullSourceCycle(pullSources map[int]int) bool {
	const (
		visiting = 1
		visited  = 2
	)

	states := make(map[int]int, len(pullSources))
	for start := range pullSources {
		if states[start] == visited {
			continue
		}
		current := start
		path := make([]int, 0)
		for states[current] != visited {
			if states[current] == visiting {
				return true
			}
			states[current] = visiting
			path = append(path, current)
			next, ok := pullSources[current]
			if !ok {
				break
			}
			current = next
		}
		for _, position := range path {
			states[position] = visited
		}
	}
	return false
}

// PullFromStepID maps the portable pull-source position to a workflow-step ID.
func (s StepPortable) PullFromStepID(posToID map[int]string) string {
	if s.PullFromStepPosition == nil {
		return ""
	}
	return posToID[*s.PullFromStepPosition]
}

// validateOnEnterActions rejects malformed on_enter actions in a portable step.
// Currently this guards set_session_mode, which requires a non-empty string
// "mode" config — without it the action is silently dropped at compile time, so
// an import would "succeed" with an inert action. See issue #1183. This mirrors
// the embedded-YAML loader's allow-list check.
func validateOnEnterActions(step StepPortable) error {
	if err := ValidateStepEventsWithRouting(step.Events, step.AgentProfile != nil, step.SessionTarget != nil); err != nil {
		return fmt.Errorf("step %q on_enter: %w", step.Name, err)
	}
	return nil
}

// WorkflowSessionTarget maps the portable position to a workflow-local step ID.
func (s StepPortable) WorkflowSessionTarget(posToID map[int]string) *WorkflowSessionTarget {
	if s.SessionTarget == nil {
		return nil
	}
	target := &WorkflowSessionTarget{Kind: s.SessionTarget.Kind}
	if s.SessionTarget.StepPosition != nil {
		target.StepID = posToID[*s.SessionTarget.StepPosition]
	}
	return target
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
		if err := validateGenericStepPositionRefs(step, validPositions); err != nil {
			return err
		}
	}
	return nil
}

// validateGenericStepPositionRefs checks move_to_step position refs across
// every GenericAction trigger.
func validateGenericStepPositionRefs(step StepPortable, validPositions map[int]bool) error {
	triggers := map[string][]GenericAction{
		"on_comment":            step.Events.OnComment,
		"on_blocker_resolved":   step.Events.OnBlockerResolved,
		"on_children_completed": step.Events.OnChildrenCompleted,
		"on_approval_resolved":  step.Events.OnApprovalResolved,
		"on_heartbeat":          step.Events.OnHeartbeat,
		"on_budget_alert":       step.Events.OnBudgetAlert,
		"on_agent_error":        step.Events.OnAgentError,
		"on_pr_merged":          step.Events.OnPRMerged,
		"on_pr_closed":          step.Events.OnPRClosed,
	}
	for _, trigger := range genericTriggerOrder {
		for _, a := range triggers[trigger] {
			if a.Type == GenericActionMoveToStep {
				if err := checkPositionRef(a.Config, validPositions); err != nil {
					return fmt.Errorf("step %q %s: %w", step.Name, trigger, err)
				}
			}
		}
	}
	return nil
}

// genericTriggerOrder fixes iteration order for validateGenericStepPositionRefs
// so error messages are deterministic.
var genericTriggerOrder = []string{
	"on_comment", "on_blocker_resolved", "on_children_completed",
	"on_approval_resolved", "on_heartbeat", "on_budget_alert", "on_agent_error",
	"on_pr_merged", "on_pr_closed",
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

// ReviewAgentProfilePortableKey is the on_enter config key that carries a
// run_code_review action's reviewing profile in portable form. The instance
// `agent_profile_id` is meaningless in another workspace, so export swaps it for
// the {agent_name, model, mode} triple and import matches it back.
const ReviewAgentProfilePortableKey = "agent_profile"

// ConvertReviewProfileToPortable rewrites run_code_review on_enter actions,
// replacing `agent_profile_id` with a portable `agent_profile` descriptor.
// An unresolvable profile has its key dropped, so the imported action falls back
// to the code-review utility agent instead of carrying a dangling ID.
func ConvertReviewProfileToPortable(events StepEvents, resolveProfile AgentProfileResolver) StepEvents {
	return remapReviewProfile(events, ReviewAgentProfileConfigKey, ReviewAgentProfilePortableKey, func(v any) (any, bool) {
		profileID, ok := v.(string)
		if !ok || profileID == "" || resolveProfile == nil {
			return nil, false
		}
		portable := exportProfileOptions(resolveProfile(profileID))
		if portable == nil {
			return nil, false
		}
		return map[string]any{
			"agent_name":     portable.AgentName,
			"model":          portable.Model,
			"mode":           portable.Mode,
			"config_options": *portable.ConfigOptions,
		}, true
	})
}

func ParsePortableProfile(value any) (*AgentProfilePortable, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("invalid profile descriptor")
	}
	var profile AgentProfilePortable
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("invalid profile descriptor")
	}
	return &profile, nil
}

func ConvertReviewProfileToIDWithLookup(events StepEvents, match func(*AgentProfilePortable) string) StepEvents {
	return remapReviewProfile(events, ReviewAgentProfilePortableKey, ReviewAgentProfileConfigKey, func(v any) (any, bool) {
		profile, err := ParsePortableProfile(v)
		if err != nil || match == nil {
			return nil, false
		}
		id := match(profile)
		return id, id != ""
	})
}

// ConvertReviewProfileToID rewrites run_code_review on_enter actions, replacing
// a portable `agent_profile` descriptor with a local `agent_profile_id`.
func ConvertReviewProfileToID(events StepEvents, matchProfile AgentProfileMatcher) StepEvents {
	return remapReviewProfile(events, ReviewAgentProfilePortableKey, ReviewAgentProfileConfigKey, func(v any) (any, bool) {
		descriptor, ok := v.(map[string]any)
		if !ok || matchProfile == nil {
			return nil, false
		}
		agentName, _ := descriptor["agent_name"].(string)
		model, _ := descriptor["model"].(string)
		mode, _ := descriptor["mode"].(string)
		profileID := matchProfile(agentName, model, mode, "")
		if profileID == "" {
			return nil, false
		}
		return profileID, true
	})
}

// remapReviewProfile rewrites the profile key on every run_code_review on_enter
// action, leaving other actions and triggers untouched. When the lookup fails,
// the source key is removed rather than preserved, so no instance-specific or
// unmatched reference survives the round trip.
func remapReviewProfile(events StepEvents, fromKey, toKey string, lookup func(any) (any, bool)) StepEvents {
	result := events
	result.OnEnter = make([]OnEnterAction, 0, len(events.OnEnter))
	for _, a := range events.OnEnter {
		if a.Type == OnEnterRunCodeReview && a.Config != nil {
			if cfg, ok := remapConfigKey(a.Config, fromKey, toKey, lookup); ok {
				a = OnEnterAction{Type: a.Type, Config: cfg}
			} else if _, exists := a.Config[fromKey]; exists {
				cfg := make(map[string]any, len(a.Config))
				maps.Copy(cfg, a.Config)
				delete(cfg, fromKey)
				a = OnEnterAction{Type: a.Type, Config: cfg}
			}
		}
		result.OnEnter = append(result.OnEnter, a)
	}
	return result
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

// remapStepEvents rewrites move_to_step config across every trigger —
// OnTurnStart/OnTurnComplete plus the seven ADR-0004 Phase 2 GenericAction
// triggers — replacing fromKey with toKey using the provided lookup function.
// OnEnter and OnExit carry no step_id/step_position references and are
// copied through unchanged.
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
	result.OnComment = remapGenericActionsConfigKey(events.OnComment, fromKey, toKey, lookup)
	result.OnBlockerResolved = remapGenericActionsConfigKey(events.OnBlockerResolved, fromKey, toKey, lookup)
	result.OnChildrenCompleted = remapGenericActionsConfigKey(events.OnChildrenCompleted, fromKey, toKey, lookup)
	result.OnApprovalResolved = remapGenericActionsConfigKey(events.OnApprovalResolved, fromKey, toKey, lookup)
	result.OnHeartbeat = remapGenericActionsConfigKey(events.OnHeartbeat, fromKey, toKey, lookup)
	result.OnBudgetAlert = remapGenericActionsConfigKey(events.OnBudgetAlert, fromKey, toKey, lookup)
	result.OnAgentError = remapGenericActionsConfigKey(events.OnAgentError, fromKey, toKey, lookup)
	result.OnPRMerged = remapGenericActionsConfigKey(events.OnPRMerged, fromKey, toKey, lookup)
	result.OnPRClosed = remapGenericActionsConfigKey(events.OnPRClosed, fromKey, toKey, lookup)
	return result
}

// remapGenericActionsConfigKey applies remapConfigKey to every move_to_step
// GenericAction in actions, leaving other action types untouched.
func remapGenericActionsConfigKey(actions []GenericAction, fromKey, toKey string, lookup func(any) (any, bool)) []GenericAction {
	result := make([]GenericAction, 0, len(actions))
	for _, a := range actions {
		if a.Type == GenericActionMoveToStep {
			if cfg, ok := remapConfigKey(a.Config, fromKey, toKey, lookup); ok {
				a = GenericAction{Type: a.Type, Config: cfg}
			}
		}
		result = append(result, a)
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
		if math.IsNaN(val) || math.IsInf(val, 0) || math.Trunc(val) != val {
			return 0, false
		}
		return int(val), true
	case int:
		return val, true
	default:
		return 0, false
	}
}
