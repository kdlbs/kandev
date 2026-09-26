package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/hostutility"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/utility/profilebinding"
)

type pluginHostUtilityManagerStub struct {
	err error
}

func (s pluginHostUtilityManagerStub) ExecuteProfilePrompt(context.Context, string, string) (*hostutility.PromptResult, error) {
	return nil, s.err
}

type pluginDefaultUtilityProfileSourceStub struct {
	profileID string
	err       error
}

func (s pluginDefaultUtilityProfileSourceStub) GetDefaultUtilityAgentProfileID(context.Context) (string, error) {
	return s.profileID, s.err
}

func TestPluginsDefaultUtilityProfileAdapterDelegates(t *testing.T) {
	adapter := pluginsDefaultUtilityProfileAdapter{source: pluginDefaultUtilityProfileSourceStub{profileID: "profile-1"}}

	got, err := adapter.GetDefaultUtilityAgentProfileID(context.Background())
	if err != nil {
		t.Fatalf("GetDefaultUtilityAgentProfileID() error = %v", err)
	}
	if got != "profile-1" {
		t.Fatalf("GetDefaultUtilityAgentProfileID() = %q, want profile-1", got)
	}
}

func TestPluginsDefaultUtilityProfileAdapterPreservesOperationalFailure(t *testing.T) {
	storeErr := errors.New("user settings unavailable")
	adapter := pluginsDefaultUtilityProfileAdapter{source: pluginDefaultUtilityProfileSourceStub{err: storeErr}}

	_, err := adapter.GetDefaultUtilityAgentProfileID(context.Background())
	if !errors.Is(err, storeErr) {
		t.Fatalf("GetDefaultUtilityAgentProfileID() error = %v, want %v", err, storeErr)
	}
}

func TestPluginsAgentProfileAdapter_MapsTypedNotFound(t *testing.T) {
	adapter := pluginsAgentProfileAdapter{resolver: &pluginAgentProfileResolverStub{err: sql.ErrNoRows}}

	_, err := adapter.GetProfileByID(context.Background(), "missing")
	if !errors.Is(err, plugins.ErrAgentProfileNotFound) {
		t.Fatalf("GetProfileByID() error = %v, want plugin not-found error", err)
	}
}

func TestPluginsAgentProfileAdapter_PreservesOperationalFailure(t *testing.T) {
	storeErr := errors.New("agent profile database unavailable")
	adapter := pluginsAgentProfileAdapter{resolver: &pluginAgentProfileResolverStub{err: storeErr}}

	_, err := adapter.GetProfileByID(context.Background(), "configured")
	if !errors.Is(err, storeErr) {
		t.Fatalf("GetProfileByID() error = %v, want store error", err)
	}
}

func TestPluginsAgentProfileAdapter_PreservesEligibilityFields(t *testing.T) {
	adapter := pluginsAgentProfileAdapter{resolver: &pluginAgentProfileResolverStub{profile: &agentsettingsmodels.AgentProfile{
		Enabled:        true,
		CLIPassthrough: false,
		WorkspaceID:    "",
	}}}

	got, err := adapter.GetProfileByID(context.Background(), "profile-1")
	if err != nil {
		t.Fatalf("GetProfileByID() error = %v", err)
	}
	if !got.Enabled || got.CLIPassthrough || got.WorkspaceID != "" || !got.InferenceCapable {
		t.Fatalf("GetProfileByID() = %+v, want an eligible profile", got)
	}
}

func TestPluginsAgentProfileAdapter_MapsIneligibleProfile(t *testing.T) {
	adapter := pluginsAgentProfileAdapter{resolver: &pluginAgentProfileResolverStub{err: profilebinding.ErrProfileIneligible}}

	_, err := adapter.GetProfileByID(context.Background(), "not-inference-capable")
	if !errors.Is(err, plugins.ErrAgentProfileIneligible) {
		t.Fatalf("GetProfileByID() error = %v, want plugin ineligible error", err)
	}
}

func TestPluginsHostUtilityAdapter_MapsRevalidationIneligibleProfile(t *testing.T) {
	adapter := pluginsHostUtilityAdapter{mgr: pluginHostUtilityManagerStub{err: profilebinding.ErrProfileIneligible}}

	_, err := adapter.ExecuteProfilePrompt(context.Background(), "profile-1", "prompt")
	if !errors.Is(err, plugins.ErrAgentProfileIneligible) {
		t.Fatalf("ExecuteProfilePrompt() error = %v, want plugin ineligible error", err)
	}
}

type pluginAgentProfileResolverStub struct {
	profile *agentsettingsmodels.AgentProfile
	err     error
}

func (s *pluginAgentProfileResolverStub) Resolve(context.Context, string) (*agentsettingsmodels.AgentProfile, error) {
	return s.profile, s.err
}
