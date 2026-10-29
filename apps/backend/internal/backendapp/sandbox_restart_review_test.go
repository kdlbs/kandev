package backendapp

import (
	"encoding/json"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/stretchr/testify/require"
)

func TestSandboxRestartReviewSurvivesLifecycleAdapter(t *testing.T) {
	var request executor.LaunchAgentRequest
	require.NoError(t, json.Unmarshal([]byte(`{"SandboxRestartReview":{"attempt_id":"attempt","selection_digest":"digest"}}`), &request))
	launch := buildLifecycleLaunchRequest(&request, "/workspace", "profile")
	review := reflect.ValueOf(launch).Elem().FieldByName("SandboxRestartReview")
	require.True(t, review.IsValid(), "the lifecycle adapter must preserve the reviewed selection")
	require.False(t, review.IsNil())
	raw, err := json.Marshal(review.Interface())
	require.NoError(t, err)
	require.JSONEq(t, `{"attempt_id":"attempt","selection_digest":"digest"}`, string(raw))
}

func TestSandboxRestartReviewCannotBeDroppedByOrdinaryAdmission(t *testing.T) {
	h := newBootStateTestHarness(t)
	policies, err := policy.NewService(h.db, true, nil)
	require.NoError(t, err)
	admission := sandboxLaunchAdmission{policies: policies, tasks: h.taskRepo}
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	ordinary := &lifecycle.LaunchRequest{Metadata: map[string]any{"executor_id": "ordinary"}}
	require.NoError(t, admission.admit(ctx, ordinary))
	ordinary.SandboxRestartReview = &contract.RestartPolicyReview{AttemptID: "missing", SelectionDigest: strings.Repeat("a", 64)}
	require.Error(t, admission.admit(ctx, ordinary), "explicit review must not disappear on the no-policy path")
	require.Nil(t, ordinary.SandboxSelection)
}
