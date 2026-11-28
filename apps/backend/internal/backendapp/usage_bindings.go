package backendapp

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

const (
	codexACPAgentID       = "codex-acp"
	codexAppServerAgentID = "codex-app-server"
	openCodeACPAgentID    = "opencode-acp"
	antigravityAgentID    = "antigravity-acp"

	accountPrefixAnthropic   = "anthropic"
	accountPrefixOpenAI      = "openai"
	accountPrefixOpenCodeZen = "opencode-zen"
	accountPrefixOpenCodeGo  = "opencode-go"
	accountPrefixOpenRouter  = "openrouter"
	accountPrefixLLMGateway  = "llmgateway"
	accountPrefixProfile     = "profile"

	accountKindUnknown    = "unknown"
	accountKindAnthropic  = "anthropic"
	accountKindOpenAI     = "openai"
	accountKindOpenCode   = "opencode_zen"
	accountKindOpenCodeGo = "opencode_go"
	accountKindOpenRouter = "openrouter"
	accountKindLLMGateway = "llmgateway"
	accountKindProfile    = "profile"

	// subcentsPerDollar converts recorded cost subcents into dollars.
	subcentsPerDollar = 10000.0

	usageSourceProviderAPI = "provider_api"
)

// modelClassResolver reports the provider's class for the profile's model.
type modelClassResolver func(ctx context.Context) agentusage.ModelClass

func fixedModelClass(class agentusage.ModelClass) modelClassResolver {
	return func(context.Context) agentusage.ModelClass { return class }
}

// usageBinding is the account reader for one concrete profile: which client
// reads the account, the cache key that lets profiles on the same account share
// one read, and how the profile's model is classified.
type usageBinding struct {
	client     agentusage.ProviderUsageClient
	cacheKey   string
	source     string
	accountKey string
	modelID    string
	class      modelClassResolver
}

// profileUsage is one profile's observed utilization and model class.
type profileUsage struct {
	Usage      *agentusage.ProviderUsage
	ModelClass agentusage.ModelClass
	Source     string
}

// accountIdentity is the client-safe name of one provider account. The account
// key names a credential file, so only this derived identity leaves the backend.
type accountIdentity struct {
	ID   string
	Kind string
}

// accountIdentityFor names an account key without exposing the credential it
// points at.
func accountIdentityFor(accountKey string) accountIdentity {
	prefix, _, _ := strings.Cut(accountKey, ":")
	switch prefix {
	case accountPrefixAnthropic:
		return accountIdentity{ID: accountPrefixAnthropic, Kind: accountKindAnthropic}
	case accountPrefixOpenAI:
		return accountIdentity{ID: accountPrefixOpenAI, Kind: accountKindOpenAI}
	case accountPrefixOpenCodeZen:
		return accountIdentity{ID: accountPrefixOpenCodeZen, Kind: accountKindOpenCode}
	case accountPrefixOpenCodeGo:
		return accountIdentity{ID: accountPrefixOpenCodeGo, Kind: accountKindOpenCodeGo}
	case accountPrefixOpenRouter:
		return accountIdentity{ID: accountPrefixOpenRouter, Kind: accountKindOpenRouter}
	case accountPrefixLLMGateway:
		return accountIdentity{ID: accountPrefixLLMGateway, Kind: accountKindLLMGateway}
	case accountPrefixProfile:
		return accountIdentity{ID: accountKey, Kind: accountKindProfile}
	}
	if prefix != "" {
		return accountIdentity{ID: prefix, Kind: accountKindUnknown}
	}
	return accountIdentity{Kind: accountKindUnknown}
}

// loadProfile resolves one concrete profile and its agent type.
func (a *usageProviderAdapter) loadProfile(ctx context.Context, profileID string) (*settingsmodels.AgentProfile, string, bool) {
	profile, err := a.settingsStore.GetAgentProfile(ctx, profileID)
	if err != nil || profile == nil || profile.DeletedAt != nil {
		return nil, "", false
	}
	agentType := ""
	if agent, err := a.settingsStore.GetAgent(ctx, profile.AgentID); err == nil && agent != nil {
		agentType = agent.Name
	}
	return profile, agentType, true
}

// bindingFor maps a profile to its own account. It only names credentials a
// host-run agent of that type actually uses. A provider without a readable
// counter still gets a stable account identity so its owner-entered and learned
// allowances can be stored and shown.
func (a *usageProviderAdapter) bindingFor(profile *settingsmodels.AgentProfile, agentType string) (usageBinding, bool) {
	if profile == nil {
		return usageBinding{}, false
	}
	if !hostUsageBindingProved(profile, agentType) {
		return usageBinding{}, false
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		return usageBinding{accountKey: accountPrefixProfile + ":" + profile.ID, source: "none", modelID: profile.Model, class: fixedModelClass(agentusage.ModelClassUnknown)}, true
	}
	switch agentType {
	case claudeACPAgentID:
		path := filepath.Join(home, ".claude", ".credentials.json")
		return usageBinding{
			client: agentusage.NewClaudeUsageClientWithPath(path), cacheKey: agentusage.CacheKey(accountPrefixAnthropic, path),
			source: usageSourceProviderAPI, accountKey: accountPrefixAnthropic + ":" + path,
			modelID: profile.Model, class: fixedModelClass(agentusage.ModelClassUnknown),
		}, true
	case codexACPAgentID, codexAppServerAgentID:
		path := filepath.Join(home, ".codex", "auth.json")
		return usageBinding{
			client: agentusage.NewCodexUsageClientWithPath(path), cacheKey: agentusage.CacheKey(accountPrefixOpenAI, path),
			source: usageSourceProviderAPI, accountKey: accountPrefixOpenAI + ":" + path,
			modelID: profile.Model, class: fixedModelClass(agentusage.ModelClassUnknown),
		}, true
	case openCodeACPAgentID:
		path := agentusage.OpenCodeAuthPath(home)
		provider := dynamicruntime.NormalizeProvider(dynamicruntime.ProviderOf(profile.Model))
		prefix := provider
		switch provider {
		case "", accountPrefixOpenCodeZen, "opencode":
			prefix = accountPrefixOpenCodeZen
		}
		binding := usageBinding{
			source: "none", accountKey: prefix + ":" + path,
			modelID: profile.Model, class: fixedModelClass(agentusage.ModelClassUnknown),
		}
		if prefix == accountPrefixOpenRouter {
			binding.client = agentusage.NewOpenRouterUsageClient(path)
			binding.cacheKey = agentusage.CacheKey(accountPrefixOpenRouter, path)
			binding.source = usageSourceProviderAPI
		}
		return binding, true
	}
	return usageBinding{
		accountKey: accountPrefixProfile + ":" + profile.ID, source: "none",
		modelID: profile.Model, class: fixedModelClass(agentusage.ModelClassUnknown),
	}, true
}

// hostUsageBindingProved reports whether a host-run profile provably uses the
// host account's own credentials, so the host account's usage is its usage.
func hostUsageBindingProved(profile *settingsmodels.AgentProfile, agentType string) bool {
	if agentType == claudeACPAgentID && snapshotClaudeProviderUncertain(profile) {
		return false
	}
	return snapshotHostBinding(profile) && !snapshotCredentialOverride(profile) && !snapshotProcessOverride(agentType)
}

// ProfileUsage returns the profile's observed utilization when a client is
// registered, and never invents a reading.
func (a *usageProviderAdapter) ProfileUsage(ctx context.Context, profileID string) profileUsage {
	if a == nil {
		return profileUsage{}
	}
	observed, _ := a.GetUsage(ctx, profileID)
	class := agentusage.ModelClassUnknown
	if observed != nil && strings.EqualFold(observed.Plan, "free") {
		class = agentusage.ModelClassFree
	}
	if a.tierLimits != nil {
		class = a.resolvedModelClass(ctx, profileID, class)
	}
	return profileUsage{Usage: observed, ModelClass: class, Source: usageSourceProviderAPI}
}

// resolvedModelClass keeps a known observed class when the binding cannot
// classify the model more precisely.
func (a *usageProviderAdapter) resolvedModelClass(ctx context.Context, profileID string, fallback agentusage.ModelClass) agentusage.ModelClass {
	profile, agentType, ok := a.loadProfile(ctx, profileID)
	if !ok {
		return fallback
	}
	binding, ok := a.bindingFor(profile, agentType)
	if !ok || binding.class == nil {
		return fallback
	}
	if resolved := binding.class(ctx); resolved != agentusage.ModelClassUnknown {
		return resolved
	}
	return fallback
}

// AccountProfileIDs lists every concrete profile bound to the same provider
// account as profileID, including profileID itself.
func (a *usageProviderAdapter) AccountProfileIDs(ctx context.Context, profileID string) ([]string, error) {
	profile, agentType, ok := a.loadProfile(ctx, profileID)
	if !ok {
		return nil, nil
	}
	binding, ok := a.bindingFor(profile, agentType)
	if !ok || binding.accountKey == "" {
		return []string{profileID}, nil
	}
	agents, err := a.settingsStore.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, 4)
	for _, agent := range agents {
		if agent == nil {
			continue
		}
		profiles, err := a.settingsStore.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return nil, err
		}
		for _, candidate := range profiles {
			if candidate == nil || candidate.DeletedAt != nil {
				continue
			}
			bound, ok := a.bindingFor(candidate, agent.Name)
			if ok && bound.accountKey == binding.accountKey {
				ids = append(ids, candidate.ID)
			}
		}
	}
	if len(ids) == 0 {
		return []string{profileID}, nil
	}
	return ids, nil
}

// ListProfileAccountIDs names the provider account of every live concrete
// profile, for callers that group profiles by account.
func (a *usageProviderAdapter) ListProfileAccountIDs(ctx context.Context) (map[string]string, error) {
	agents, err := a.settingsStore.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]string)
	for _, agent := range agents {
		if agent == nil {
			continue
		}
		profiles, err := a.settingsStore.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return nil, err
		}
		for _, profile := range profiles {
			if profile == nil || profile.DeletedAt != nil {
				continue
			}
			binding, ok := a.bindingFor(profile, agent.Name)
			if !ok {
				continue
			}
			if identity := accountIdentityFor(binding.accountKey); identity.ID != "" {
				ids[profile.ID] = identity.ID
			}
		}
	}
	return ids, nil
}

// boundProfile is one concrete profile with its agent type and account binding.
type boundProfile struct {
	profile   *settingsmodels.AgentProfile
	agentType string
	binding   usageBinding
}

// boundProfiles resolves every live concrete profile to its account.
func (a *usageProviderAdapter) boundProfiles(ctx context.Context) ([]boundProfile, error) {
	agents, err := a.settingsStore.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	bound := make([]boundProfile, 0, 16)
	for _, agent := range agents {
		if agent == nil {
			continue
		}
		profiles, err := a.settingsStore.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return nil, err
		}
		for _, profile := range profiles {
			if profile == nil || profile.DeletedAt != nil {
				continue
			}
			binding, ok := a.bindingFor(profile, agent.Name)
			if !ok {
				continue
			}
			bound = append(bound, boundProfile{profile: profile, agentType: agent.Name, binding: binding})
		}
	}
	return bound, nil
}
