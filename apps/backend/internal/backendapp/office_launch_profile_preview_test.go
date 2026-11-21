package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

type fakePreviewAgents struct {
	agent *officemodels.AgentInstance
	err   error
}

func (f fakePreviewAgents) GetAgentInstance(context.Context, string) (*officemodels.AgentInstance, error) {
	return f.agent, f.err
}

type fakePreviewResolver struct {
	res      *routing.Resolution
	err      error
	gotAgent string
	gotOpts  routing.ResolveOptions
}

func (f *fakePreviewResolver) Resolve(
	_ context.Context, _ string, agent officemodels.AgentInstance, opts routing.ResolveOptions,
) (*routing.Resolution, error) {
	f.gotAgent, f.gotOpts = agent.ID, opts
	return f.res, f.err
}

func TestOfficeLaunchProfilePreview_UsesFirstRoutedCandidate(t *testing.T) {
	resolver := &fakePreviewResolver{res: &routing.Resolution{Candidates: []routing.Candidate{
		{ExecutionProfileID: "profile-primary"}, {ExecutionProfileID: "profile-fallback"},
	}}}
	preview := &officeLaunchProfilePreview{
		agents:   fakePreviewAgents{agent: &officemodels.AgentInstance{ID: "agent-ceo"}},
		resolver: resolver,
	}

	got, err := preview.Preview(context.Background(), "ws-1", "agent-ceo")

	require.NoError(t, err)
	assert.Equal(t, "profile-primary", got)
	assert.Equal(t, "agent-ceo", resolver.gotAgent)
	assert.Equal(t, routing.ResolveOptions{}, resolver.gotOpts, "preparation has no run reason or exclusions")
}

func TestOfficeLaunchProfilePreview_NoCandidateKeepsAgentProfile(t *testing.T) {
	preview := &officeLaunchProfilePreview{
		agents:   fakePreviewAgents{agent: &officemodels.AgentInstance{ID: "agent-ceo"}},
		resolver: &fakePreviewResolver{res: &routing.Resolution{Enabled: false}},
	}

	got, err := preview.Preview(context.Background(), "ws-1", "agent-ceo")

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestOfficeLaunchProfilePreview_PropagatesLookupErrors(t *testing.T) {
	agentErr := errors.New("agent lookup failed")
	resolveErr := errors.New("routing config unavailable")
	for name, preview := range map[string]*officeLaunchProfilePreview{
		"agent": {agents: fakePreviewAgents{err: agentErr}, resolver: &fakePreviewResolver{}},
		"resolve": {
			agents:   fakePreviewAgents{agent: &officemodels.AgentInstance{ID: "agent-ceo"}},
			resolver: &fakePreviewResolver{err: resolveErr},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := preview.Preview(context.Background(), "ws-1", "agent-ceo")
			assert.Error(t, err)
		})
	}
}
