package controller

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestSessionTargetPatchPreservesOmissionAndExplicitNull(t *testing.T) {
	var omitted struct {
		Target SessionTargetPatch `json:"session_target,omitempty"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{}`), &omitted))
	require.False(t, omitted.Target.Set)

	var cleared struct {
		Target SessionTargetPatch `json:"session_target,omitempty"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"session_target":null}`), &cleared))
	require.True(t, cleared.Target.Set)
	require.Nil(t, cleared.Target.Target)
}

func TestSessionTargetPatchValidatesClosedUnion(t *testing.T) {
	var request struct {
		Target SessionTargetPatch `json:"session_target"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"session_target":{"kind":"initial"}}`), &request))
	require.True(t, request.Target.Set)
	require.Equal(t, &models.WorkflowSessionTarget{Kind: models.WorkflowSessionTargetInitial}, request.Target.Target)

	err := json.Unmarshal([]byte(`{"session_target":{"kind":"unknown"}}`), &request)
	require.ErrorContains(t, err, "unknown session target kind")
}
