package models

import (
	"fmt"
	"maps"
	"strings"
)

const (
	WorkflowScriptDefaultTimeoutSeconds = 600
	WorkflowScriptMinTimeoutSeconds     = 1
	WorkflowScriptMaxTimeoutSeconds     = 86400
)

// WorkflowScriptFailurePolicy controls whether a failed lifecycle command
// prevents the workflow operation from continuing.
type WorkflowScriptFailurePolicy string

const (
	WorkflowScriptFailurePolicyBlock    WorkflowScriptFailurePolicy = "block"
	WorkflowScriptFailurePolicyContinue WorkflowScriptFailurePolicy = "continue"
)

// WorkflowScriptAction is the validated, typed form of a run_script config.
// The persisted workflow wire format remains the action-specific config map.
type WorkflowScriptAction struct {
	Command        string
	TimeoutSeconds int
	FailurePolicy  WorkflowScriptFailurePolicy
}

// ParseWorkflowScriptAction validates and normalizes a persisted run_script
// config. The command is preserved exactly after checking that it is not only
// whitespace; omitted optional values receive their documented defaults.
func ParseWorkflowScriptAction(config map[string]interface{}) (WorkflowScriptAction, error) {
	if config == nil {
		return WorkflowScriptAction{}, fmt.Errorf("run_script requires a config")
	}

	command, ok := config["command"].(string)
	if !ok || strings.TrimSpace(command) == "" {
		return WorkflowScriptAction{}, fmt.Errorf("run_script command must be a non-empty string")
	}

	action := WorkflowScriptAction{
		Command:        command,
		TimeoutSeconds: WorkflowScriptDefaultTimeoutSeconds,
		FailurePolicy:  WorkflowScriptFailurePolicyBlock,
	}
	if rawTimeout, exists := config["timeout_seconds"]; exists {
		timeout, ok := toInt(rawTimeout)
		if !ok {
			return WorkflowScriptAction{}, fmt.Errorf("run_script timeout_seconds must be an integer")
		}
		if timeout < WorkflowScriptMinTimeoutSeconds || timeout > WorkflowScriptMaxTimeoutSeconds {
			return WorkflowScriptAction{}, fmt.Errorf(
				"run_script timeout_seconds must be between %d and %d",
				WorkflowScriptMinTimeoutSeconds,
				WorkflowScriptMaxTimeoutSeconds,
			)
		}
		action.TimeoutSeconds = timeout
	}
	if rawPolicy, exists := config["failure_policy"]; exists {
		policy, ok := rawPolicy.(string)
		if !ok || !isWorkflowScriptFailurePolicy(policy) {
			return WorkflowScriptAction{}, fmt.Errorf("run_script failure_policy must be block or continue")
		}
		action.FailurePolicy = WorkflowScriptFailurePolicy(policy)
	}
	return action, nil
}

// NormalizeWorkflowScriptConfig returns an independent config map with
// defaulted script fields. Callers use it when serializing portable workflows.
func NormalizeWorkflowScriptConfig(config map[string]interface{}) (map[string]interface{}, error) {
	action, err := ParseWorkflowScriptAction(config)
	if err != nil {
		return nil, err
	}
	normalized := make(map[string]interface{}, len(config)+2)
	maps.Copy(normalized, config)
	normalized["command"] = action.Command
	normalized["timeout_seconds"] = action.TimeoutSeconds
	normalized["failure_policy"] = string(action.FailurePolicy)
	return normalized, nil
}

func normalizeWorkflowScriptEvents(events StepEvents) StepEvents {
	result := events
	onEnterCloned := false
	for index, action := range events.OnEnter {
		if action.Type != OnEnterRunScript {
			continue
		}
		if !onEnterCloned {
			result.OnEnter = append([]OnEnterAction(nil), events.OnEnter...)
			onEnterCloned = true
		}
		config, err := NormalizeWorkflowScriptConfig(action.Config)
		if err == nil {
			result.OnEnter[index].Config = config
		}
	}
	onTurnCompleteCloned := false
	for index, action := range events.OnTurnComplete {
		if action.Type != OnTurnCompleteRunScript {
			continue
		}
		if !onTurnCompleteCloned {
			result.OnTurnComplete = append([]OnTurnCompleteAction(nil), events.OnTurnComplete...)
			onTurnCompleteCloned = true
		}
		config, err := NormalizeWorkflowScriptConfig(action.Config)
		if err == nil {
			result.OnTurnComplete[index].Config = config
		}
	}
	onExitCloned := false
	for index, action := range events.OnExit {
		if action.Type != OnExitRunScript {
			continue
		}
		if !onExitCloned {
			result.OnExit = append([]OnExitAction(nil), events.OnExit...)
			onExitCloned = true
		}
		config, err := NormalizeWorkflowScriptConfig(action.Config)
		if err == nil {
			result.OnExit[index].Config = config
		}
	}
	return result
}

func isWorkflowScriptFailurePolicy(policy string) bool {
	return WorkflowScriptFailurePolicy(policy) == WorkflowScriptFailurePolicyBlock ||
		WorkflowScriptFailurePolicy(policy) == WorkflowScriptFailurePolicyContinue
}
