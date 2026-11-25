package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestExcludesAuthorProvider(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]interface{}
		want   bool
	}{
		{"nil config", nil, false},
		{"missing key", map[string]interface{}{"role": "reviewer"}, false},
		{"true", map[string]interface{}{"exclude_author_provider": true}, true},
		{"false", map[string]interface{}{"exclude_author_provider": false}, false},
		{"non-bool string", map[string]interface{}{"exclude_author_provider": "true"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExcludesAuthorProvider(tc.config); got != tc.want {
				t.Fatalf("ExcludesAuthorProvider() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAuthorProviderExclusionRoles(t *testing.T) {
	events := StepEvents{OnEnter: []OnEnterAction{
		{Type: OnEnterClearDecisions},
		{Type: OnEnterEnsureParticipantSeat, Config: map[string]interface{}{
			"role": "reviewer", "exclude_author_provider": true,
		}},
		{Type: OnEnterEnsureParticipantSeat, Config: map[string]interface{}{
			"role": "approver",
		}},
		{Type: OnEnterEnsureParticipantSeat, Config: map[string]interface{}{
			"exclude_author_provider": true,
		}},
		{Type: OnEnterQueueRunForEachParticipant, Config: map[string]interface{}{
			"role": "watcher", "exclude_author_provider": true,
		}},
	}}
	got := AuthorProviderExclusionRoles(events)
	if len(got) != 1 || !got["reviewer"] {
		t.Fatalf("AuthorProviderExclusionRoles() = %v, want only reviewer", got)
	}
	if roles := AuthorProviderExclusionRoles(StepEvents{}); len(roles) != 0 {
		t.Fatalf("empty events roles = %v, want none", roles)
	}
}

func TestExcludeAuthorProviderSurvivesExportImportYAML(t *testing.T) {
	steps := []*WorkflowStep{
		{ID: "work", Name: "Work", Position: 0, StageType: StageTypeWork},
		{
			ID: "review", Name: "Review", Position: 1, StageType: StageTypeReview,
			Events: StepEvents{OnEnter: []OnEnterAction{
				{Type: OnEnterEnsureParticipantSeat, Config: map[string]interface{}{
					"role": "reviewer", ExcludeAuthorProviderConfigKey: true,
				}},
			}},
		},
	}
	wf := &taskmodels.Workflow{ID: "wf-1", Name: "Office Review"}
	export := BuildWorkflowExport([]*taskmodels.Workflow{wf}, map[string][]*WorkflowStep{"wf-1": steps}, nil)

	raw, err := yaml.Marshal(export)
	require.NoError(t, err)
	var imported WorkflowExport
	require.NoError(t, yaml.Unmarshal(raw, &imported))
	require.NoError(t, imported.Validate())

	var review *StepPortable
	for i := range imported.Workflows[0].Steps {
		if imported.Workflows[0].Steps[i].Name == "Review" {
			review = &imported.Workflows[0].Steps[i]
		}
	}
	require.NotNil(t, review)
	events := ConvertPositionToStepID(review.Events, map[int]string{0: "new-work", 1: "new-review"})
	assert.Equal(t, map[string]bool{"reviewer": true}, AuthorProviderExclusionRoles(events))
}
