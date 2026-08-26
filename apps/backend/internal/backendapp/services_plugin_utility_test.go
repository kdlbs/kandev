package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/hostutility"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins"
	userModels "github.com/kandev/kandev/internal/user/models"
	userService "github.com/kandev/kandev/internal/user/service"
	userStore "github.com/kandev/kandev/internal/user/store"
	utilitymodels "github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/profilebinding"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
	utilitystore "github.com/kandev/kandev/internal/utility/store"
)

type pluginHostUtilityManagerStub struct {
	err error
}

func (s pluginHostUtilityManagerStub) ExecuteProfilePrompt(context.Context, string, string) (*hostutility.PromptResult, error) {
	return nil, s.err
}

type pluginUtilityAgentRepository struct {
	utilitystore.Repository
	err error
}

func (r *pluginUtilityAgentRepository) GetAgentByID(context.Context, string) (*utilitymodels.UtilityAgent, error) {
	return nil, r.err
}

func TestPluginsUtilityAgentAdapter_MapsTypedNotFound(t *testing.T) {
	adapter := pluginsUtilityAgentAdapter{
		svc: utilityservice.NewService(&pluginUtilityAgentRepository{err: sql.ErrNoRows}),
	}

	_, err := adapter.GetAgentByID(context.Background(), "missing")
	if !errors.Is(err, plugins.ErrUtilityAgentNotFound) {
		t.Fatalf("GetAgentByID() error = %v, want plugin not-found error", err)
	}
}

func TestPluginsUtilityAgentAdapter_PreservesOperationalFailure(t *testing.T) {
	storeErr := errors.New("utility agent database unavailable")
	adapter := pluginsUtilityAgentAdapter{
		svc: utilityservice.NewService(&pluginUtilityAgentRepository{err: storeErr}),
	}

	_, err := adapter.GetAgentByID(context.Background(), "configured")
	if !errors.Is(err, storeErr) {
		t.Fatalf("GetAgentByID() error = %v, want store error", err)
	}
}

func TestPluginsUtilityAgentAdapter_ResolvesEmptyBuiltinThroughDefault(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	userRepo := &pluginUserRepository{
		getSettings: &userModels.UserSettings{DefaultUtilityAgentProfileID: "default-profile"},
	}
	adapter := pluginsUtilityAgentAdapter{
		svc: utilityservice.NewService(&utilityAgentRepositoryStub{agent: &utilitymodels.UtilityAgent{
			ID:                  "builtin",
			Builtin:             true,
			ProfileBindingState: utilitymodels.ProfileBindingInherit,
		}}),
		userSvc: userService.NewService(userRepo, bus.NewMemoryEventBus(log), log),
	}

	got, err := adapter.GetAgentByID(context.Background(), "builtin")
	if err != nil {
		t.Fatalf("GetAgentByID() error = %v", err)
	}
	if got.AgentProfileID != "default-profile" {
		t.Fatalf("AgentProfileID = %q, want %q", got.AgentProfileID, "default-profile")
	}
	if got.ProfileBindingState != utilitymodels.ProfileBindingExplicit {
		t.Fatalf("ProfileBindingState = %q, want %q", got.ProfileBindingState, utilitymodels.ProfileBindingExplicit)
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

type utilityAgentRepositoryStub struct {
	utilitystore.Repository
	agent *utilitymodels.UtilityAgent
}

type pluginAgentProfileResolverStub struct {
	profile *agentsettingsmodels.AgentProfile
	err     error
}

func (s *pluginAgentProfileResolverStub) Resolve(context.Context, string) (*agentsettingsmodels.AgentProfile, error) {
	return s.profile, s.err
}

func (r *utilityAgentRepositoryStub) GetAgentByID(context.Context, string) (*utilitymodels.UtilityAgent, error) {
	return r.agent, nil
}

type pluginUserRepository struct {
	userStore.Repository
	getSettings *userModels.UserSettings
}

func (r *pluginUserRepository) GetUserSettings(context.Context, string) (*userModels.UserSettings, error) {
	return r.getSettings, nil
}
