package models

import (
	"strings"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// @covers AC-TASKS-PORTABLE-PROFILE-001.1
func TestPortableProfileOptionsVersionThree(t *testing.T) {
	var export WorkflowExport
	err := yaml.Unmarshal([]byte(`version: 3
type: kandev_workflow
workflows:
  - name: Development
    steps:
      - name: Work
        position: 0
        color: blue
        complete_task_on_enter: false
        agent_profile:
          agent_name: Codex
          model: luna
          config_options:
            reasoning_effort: low
`), &export)
	require.NoError(t, err)
	require.NoError(t, export.Validate())
	encoded, err := yaml.Marshal(export)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "reasoning_effort: low")
}

// @covers AC-TASKS-PORTABLE-PROFILE-001.1
func TestExportWithProfileUsesVersionThree(t *testing.T) {
	var profile AgentProfilePortable
	require.NoError(t, yaml.Unmarshal([]byte("agent_name: Codex\nconfig_options: {}\n"), &profile))
	wf := &taskmodels.Workflow{ID: "wf", Name: "Flow", AgentProfileID: "profile"}
	export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, nil, func(string) *AgentProfilePortable { return &profile })
	require.Equal(t, 3, export.Version)
	require.NoError(t, export.Validate())
}

// @covers AC-TASKS-PORTABLE-PROFILE-001.4
func TestPortableProfileOptionsRequireVersionAndMap(t *testing.T) {
	base := `version: VERSION
type: kandev_workflow
workflows:
  - name: Development
    steps:
      - name: Work
        position: 0
        color: blue
        complete_task_on_enter: false
        agent_profile:
          agent_name: Codex
OPTIONS`
	for _, tc := range []struct {
		version, options string
		valid            bool
	}{
		{"3", "          config_options: {}\n", true},
		{"3", "", false},
		{"3", "          config_options: null\n", false},
		{"3", "          config_options: {model: x}\n", false},
		{"2", "          config_options: {effort: low}\n", false},
		{"2", "", true},
	} {
		t.Run(tc.version+tc.options, func(t *testing.T) {
			input := strings.ReplaceAll(strings.ReplaceAll(base, "VERSION", tc.version), "OPTIONS", tc.options)
			var export WorkflowExport
			err := yaml.Unmarshal([]byte(input), &export)
			if err == nil {
				err = export.Validate()
			}
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// @covers AC-TASKS-PORTABLE-PROFILE-001.1
func TestPortableReviewProfileCarriesOptions(t *testing.T) {
	options := map[string]string{"reasoning_effort": "low"}
	workflow := &taskmodels.Workflow{ID: "wf", Name: "Flow"}
	step := &WorkflowStep{ID: "step", Name: "Review", Position: 0, Color: "blue", Events: StepEvents{OnEnter: []OnEnterAction{{Type: OnEnterRunCodeReview, Config: map[string]interface{}{ReviewAgentProfileConfigKey: "profile"}}}}}
	export := BuildWorkflowExport([]*taskmodels.Workflow{workflow}, map[string][]*WorkflowStep{"wf": {step}}, func(string) *AgentProfilePortable {
		return &AgentProfilePortable{AgentName: "Codex", ConfigOptions: &options}
	})
	require.Equal(t, ProfileOptionsExportVersion, export.Version)
	data, err := yaml.Marshal(export)
	require.NoError(t, err)
	require.Contains(t, string(data), "reasoning_effort: low")
	var decoded WorkflowExport
	require.NoError(t, yaml.Unmarshal(data, &decoded))
	require.NoError(t, decoded.Validate())
	decoded.Version = 2
	require.Error(t, decoded.Validate())
}
