package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateWorkflowStepConfigureSession(t *testing.T) {
	tests := []struct {
		name    string
		step    *WorkflowStep
		wantErr string
	}{
		{
			name: "accepts multiple families and a set rule",
			step: &WorkflowStep{Events: StepEvents{OnEnter: []OnEnterAction{{
				Type: OnEnterConfigureSession,
				Config: map[string]interface{}{"rules": []interface{}{
					map[string]interface{}{
						"agent_name": "codex",
						"operation":  "set",
						"model":      "gpt-5.6-luna",
						"config_options": map[string]interface{}{
							"reasoning_effort": "max",
						},
					},
					map[string]interface{}{
						"agent_name": "claude",
						"operation":  "restore_original",
					},
				}}},
			}}},
		},
		{
			name: "rejects duplicate family",
			step: &WorkflowStep{Events: StepEvents{OnEnter: []OnEnterAction{{
				Type: OnEnterConfigureSession,
				Config: map[string]interface{}{"rules": []interface{}{
					map[string]interface{}{"agent_name": "codex", "operation": "keep"},
					map[string]interface{}{"agent_name": "codex", "operation": "restore_original"},
				}}},
			}}},
			wantErr: "duplicate agent_name",
		},
		{
			name: "rejects empty set",
			step: &WorkflowStep{Events: StepEvents{OnEnter: []OnEnterAction{{
				Type: OnEnterConfigureSession,
				Config: map[string]interface{}{"rules": []interface{}{
					map[string]interface{}{"agent_name": "codex", "operation": "set"},
				}}},
			}}},
			wantErr: "set requires a non-empty model or config_options",
		},
		{
			name: "rejects values on restore",
			step: &WorkflowStep{Events: StepEvents{OnEnter: []OnEnterAction{{
				Type: OnEnterConfigureSession,
				Config: map[string]interface{}{"rules": []interface{}{
					map[string]interface{}{
						"agent_name": "codex",
						"operation":  "restore_original",
						"model":      "gpt-5.6-sol",
					},
				}}},
			}}},
			wantErr: "does not accept model or config_options",
		},
		{
			name: "rejects fixed profile combination",
			step: &WorkflowStep{
				AgentProfileID: "profile-1",
				Events: StepEvents{OnEnter: []OnEnterAction{{
					Type: OnEnterConfigureSession,
					Config: map[string]interface{}{"rules": []interface{}{
						map[string]interface{}{"agent_name": "codex", "operation": "keep"},
					}}},
				}},
			},
			wantErr: "cannot be combined with agent_profile_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkflowStep(tt.step)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestConfigureSessionActionConfig(t *testing.T) {
	action := OnEnterAction{
		Type: OnEnterConfigureSession,
		Config: map[string]interface{}{"rules": []interface{}{
			map[string]interface{}{
				"agent_name": "codex",
				"operation":  "set",
				"model":      "gpt-5.6-luna",
				"config_options": map[string]interface{}{
					"reasoning_effort": "max",
				},
			},
		}},
	}

	rules, err := ParseConfigureSessionRules(action)
	require.NoError(t, err)
	require.Equal(t, []ConfigureSessionRule{
		{
			AgentName: "codex",
			Operation: ConfigureSessionSet,
			Model:     "gpt-5.6-luna",
			ConfigOptions: map[string]string{
				"reasoning_effort": "max",
			},
		},
	}, rules)
}

func TestWorkflowExportRejectsConfigureSessionWithPortableProfile(t *testing.T) {
	profile := &AgentProfilePortable{AgentName: "codex", Model: "gpt-5.6-sol"}
	export := &WorkflowExport{
		Version: ExportVersion,
		Type:    ExportType,
		Workflows: []WorkflowPortable{{
			Name: "workflow",
			Steps: []StepPortable{{
				Name:         "work",
				Position:     0,
				Color:        "blue",
				AgentProfile: profile,
				Events: StepEvents{OnEnter: []OnEnterAction{{
					Type: OnEnterConfigureSession,
					Config: map[string]interface{}{"rules": []interface{}{
						map[string]interface{}{"agent_name": "codex", "operation": "keep"},
					}}},
				}},
			}},
		}},
	}

	require.ErrorContains(t, export.Validate(), "cannot be combined with agent_profile")
}
