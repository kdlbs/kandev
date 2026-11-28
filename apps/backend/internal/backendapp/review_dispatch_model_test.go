package backendapp

import (
	"context"
	"errors"
	"testing"

	utility "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/review"
	"github.com/stretchr/testify/require"
)

type dispatchOriginProfiles map[string]review.ReviewerProfile

func (p dispatchOriginProfiles) ReviewerOriginProfile(_ context.Context, id string) (review.ReviewerProfile, bool, error) {
	profile, ok := p[id]
	return profile, ok, nil
}

type dispatchDefaults struct{ agent, model string }

func (d dispatchDefaults) DefaultUtilitySettings(context.Context) (string, string, error) {
	return d.agent, d.model, nil
}

// profileOnlySession can dispatch a profile but cannot carry a model with it.
type profileOnlySession struct{ profileCalls []string }

func (s *profileOnlySession) ExecuteInferencePrompt(context.Context, string, string, string, string) (*utility.PromptResponse, error) {
	return nil, errors.New("unexpected agent/model dispatch")
}

func (s *profileOnlySession) ExecuteInferenceProfilePrompt(_ context.Context, _, profileID, _ string) (*utility.PromptResponse, error) {
	s.profileCalls = append(s.profileCalls, profileID)
	return &utility.PromptResponse{Success: true, Response: `{"findings":[]}`}, nil
}

// modelSession records the model each profile dispatch carried; "" means the
// profile's own model.
type modelSession struct {
	profileOnlySession
	models []string
}

func (s *modelSession) ExecuteInferenceProfilePrompt(ctx context.Context, sessionID, profileID, prompt string) (*utility.PromptResponse, error) {
	s.models = append(s.models, "")
	return s.profileOnlySession.ExecuteInferenceProfilePrompt(ctx, sessionID, profileID, prompt)
}

func (s *modelSession) ExecuteInferenceProfileModelPrompt(ctx context.Context, sessionID, profileID, model, prompt string) (*utility.PromptResponse, error) {
	s.models = append(s.models, model)
	return s.profileOnlySession.ExecuteInferenceProfilePrompt(ctx, sessionID, profileID, prompt)
}

// A reviewer profile without a model inherits the user's utility default for
// the writer-exclusion check; the dispatch must then run that same model.
func excludingWriterContext(t *testing.T) context.Context {
	t.Helper()
	resolver := review.NewResolver(nil, nil, dispatchDefaults{agent: "claude-acp", model: "default-model"})
	ctx, err := resolver.WithOriginExclusion(context.Background(), &review.OriginIdentity{
		ProfileID: "writer-profile", AgentID: "claude-acp", Model: "writer-model",
	}, true)
	require.NoError(t, err)
	require.True(t, review.OriginExclusionActive(ctx))
	return ctx
}

func modellessReviewer() (review.ReviewerIdentity, dispatchOriginProfiles) {
	return review.ReviewerIdentity{ProfileID: "reviewer-profile", AgentID: "claude-acp"},
		dispatchOriginProfiles{"reviewer-profile": {AgentID: "claude-acp"}}
}

func TestReviewDispatchCarriesTheExclusionCheckedModel(t *testing.T) {
	identity, profiles := modellessReviewer()
	session := &modelSession{}
	inference := reviewInference{session: session, originProfiles: profiles}

	_, err := inference.Run(excludingWriterContext(t), identity, "session-1", "review")
	require.NoError(t, err)
	require.Equal(t, []string{"default-model"}, session.models)
	require.Equal(t, []string{"reviewer-profile"}, session.profileCalls)
}

func TestReviewDispatchRejectsAnExecutorThatCannotCarryTheCheckedModel(t *testing.T) {
	identity, profiles := modellessReviewer()
	session := &profileOnlySession{}
	inference := reviewInference{session: session, originProfiles: profiles}

	_, err := inference.Run(excludingWriterContext(t), identity, "session-1", "review")
	require.ErrorIs(t, err, review.ErrAgentUnavailable)
	require.Empty(t, session.profileCalls, "an unchecked model must never run")
}

func TestReviewDispatchWithoutExclusionKeepsTheProfileModel(t *testing.T) {
	identity, _ := modellessReviewer()
	session := &modelSession{}
	inference := reviewInference{session: session}

	_, err := inference.Run(context.Background(), identity, "session-1", "review")
	require.NoError(t, err)
	require.Equal(t, []string{""}, session.models)
}
