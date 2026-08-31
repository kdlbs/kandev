package models

import (
	"fmt"
	"maps"
	"math"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	ExportVersion = 1
	ExportType    = "kandev_workflow"
)

// WorkflowExport is the portable format for sharing workflows.
type WorkflowExport struct {
	Version   int                `json:"version" yaml:"version"`
	Type      string             `json:"type" yaml:"type"`
	Workflows []WorkflowPortable `json:"workflows" yaml:"workflows"`
}

// AgentProfilePortable stores enough agent profile info for cross-workspace matching.
type AgentProfilePortable struct {
	AgentName string `json:"agent_name" yaml:"agent_name"`
	Model     string `json:"model,omitempty" yaml:"model,omitempty"`
	Mode      string `json:"mode,omitempty" yaml:"mode,omitempty"`
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
	Name                       string                `json:"name" yaml:"name"`
	Position                   int                   `json:"position" yaml:"position"`
	Color                      string                `json:"color" yaml:"color"`
	Prompt                     string                `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Events                     StepEvents            `json:"events" yaml:"events"`
	IsStartStep                bool                  `json:"is_start_step" yaml:"is_start_step"`
	ShowInCommandPanel         bool                  `json:"show_in_command_panel" yaml:"show_in_command_panel"`
	AllowManualMove            bool                  `json:"allow_manual_move" yaml:"allow_manual_move"`
	AutoArchiveAfterHours      int                   `json:"auto_archive_after_hours,omitempty" yaml:"auto_archive_after_hours,omitempty"`
	AgentProfile               *AgentProfilePortable `json:"agent_profile,omitempty" yaml:"agent_profile,omitempty"`
	AutoAdvanceRequiresSignal  bool                  `json:"auto_advance_requires_signal" yaml:"auto_advance_requires_signal"`
	CancelTriggersTurnComplete bool                  `json:"cancel_triggers_turn_complete" yaml:"cancel_triggers_turn_complete"`
	WIPLimit                   int                   `json:"wip_limit,omitempty" yaml:"wip_limit,omitempty"`
	PullFromStepPosition       *int                  `json:"pull_from_step_position,omitempty" yaml:"pull_from_step_position,omitempty"`
}

// BuildWorkflowExport builds a portable WorkflowExport from domain models.
// stepsByWorkflow maps workflow ID → its steps (ordered by position).
// resolveProfile converts agent profile IDs to portable form (may be nil).
func BuildWorkflowExport(workflows []*taskmodels.Workflow, stepsByWorkflow map[string][]*WorkflowStep, resolveProfile AgentProfileResolver) *WorkflowExport {
	portable := make([]WorkflowPortable, 0, len(workflows))
	for _, wf := range workflows {
		steps := stepsByWorkflow[wf.ID]
		portable = append(portable, buildWorkflowPortable(wf, steps, resolveProfile))
	}
	return &WorkflowExport{
		Version:   ExportVersion,
		Type:      ExportType,
		Workflows: portable,
	}
}

func buildWorkflowPortable(wf *taskmodels.Workflow, steps []*WorkflowStep, resolveProfile AgentProfileResolver) WorkflowPortable {
	portableSteps := make([]StepPortable, 0, len(steps))
	// Build step ID → position map for converting move_to_step references.
	idToPos := make(map[string]int, len(steps))
	for _, s := range steps {
		idToPos[s.ID] = s.Position
	}
	for _, s := range steps {
		sp := StepPortable{
			Name:                       s.Name,
			Position:                   s.Position,
			Color:                      s.Color,
			Prompt:                     s.Prompt,
			Events:                     ConvertReviewProfileToPortable(convertStepIDToPosition(s.Events, idToPos), resolveProfile),
			IsStartStep:                s.IsStartStep,
			ShowInCommandPanel:         s.ShowInCommandPanel,
			AllowManualMove:            s.AllowManualMove,
			AutoArchiveAfterHours:      s.AutoArchiveAfterHours,
			AutoAdvanceRequiresSignal:  s.AutoAdvanceRequiresSignal,
			CancelTriggersTurnComplete: s.CancelTriggersTurnComplete,
			WIPLimit:                   s.WIPLimit,
		}
		if pos, ok := idToPos[s.PullFromStepID]; ok {
			sp.PullFromStepPosition = &pos
		}
		if resolveProfile != nil && s.AgentProfileID != "" {
			sp.AgentProfile = resolveProfile(s.AgentProfileID)
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
		wp.AgentProfile = resolveProfile(wf.AgentProfileID)
	}
	return wp
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
			if err := validateOnEnterActions(step); err != nil {
				return fmt.Errorf("workflow %d step %d: %w", i, j, err)
			}
			if step.WIPLimit < 0 {
				return fmt.Errorf("workflow %d step %d: wip_limit must be non-negative", i, j)
			}
		}
		// Validate that move_to_step references point to valid positions.
		if err := validateStepPositionRefs(wf.Steps, positions); err != nil {
			return fmt.Errorf("workflow %d: %w", i, err)
		}
		if err := validatePullSourceRefs(wf.Steps, positions); err != nil {
			return fmt.Errorf("workflow %d: %w", i, err)
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
	if err := ValidateStepEvents(step.Events, step.AgentProfile != nil); err != nil {
		return fmt.Errorf("step %q on_enter: %w", step.Name, err)
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
		if err := validateGenericStepPositionRefs(step, validPositions); err != nil {
			return err
		}
	}
	return nil
}

// validateGenericStepPositionRefs checks move_to_step position refs across
// the seven ADR-0004 Phase 2 GenericAction triggers.
func validateGenericStepPositionRefs(step StepPortable, validPositions map[int]bool) error {
	triggers := map[string][]GenericAction{
		"on_comment":            step.Events.OnComment,
		"on_blocker_resolved":   step.Events.OnBlockerResolved,
		"on_children_completed": step.Events.OnChildrenCompleted,
		"on_approval_resolved":  step.Events.OnApprovalResolved,
		"on_heartbeat":          step.Events.OnHeartbeat,
		"on_budget_alert":       step.Events.OnBudgetAlert,
		"on_agent_error":        step.Events.OnAgentError,
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
		portable := resolveProfile(profileID)
		if portable == nil {
			return nil, false
		}
		return map[string]any{
			"agent_name": portable.AgentName,
			"model":      portable.Model,
			"mode":       portable.Mode,
		}, true
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
