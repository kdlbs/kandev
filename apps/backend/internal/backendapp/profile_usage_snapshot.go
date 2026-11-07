package backendapp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

const (
	snapshotCodexACPAgentID       = "codex-acp"
	snapshotCodexAppServerAgentID = "codex-app-server"
	snapshotHostDefault           = "host_default"
	snapshotWindowsOS             = "windows"
	snapshotBindingUnknown        = "binding_unknown"
)

// profileUsageSnapshot is composed only for the enabled native read surface.
// It shares Office's UsageService without changing Office's binding behavior.
type profileUsageSnapshot struct {
	store settingsstore.Repository
	usage *agentusage.UsageService
	proxy usageProxyResolver
	home  string
}

type snapshotBinding struct {
	agentType string
	profile   *settingsmodels.AgentProfile
	entry     dto.AgentProfileUsageDTO
	client    agentusage.ProviderUsageClient
	key       string
}

func newProfileUsageSnapshot(adapter *usageProviderAdapter) *profileUsageSnapshot {
	home, _ := os.UserHomeDir()
	return &profileUsageSnapshot{store: adapter.settingsStore, usage: adapter.svc, proxy: adapter.proxyResolver, home: home}
}

func (s *profileUsageSnapshot) ListProfileUsage(ctx context.Context) ([]dto.AgentProfileUsageDTO, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	bindings, err := s.bindings(ctx)
	if err != nil {
		return nil, err
	}
	// Group only by the credential cache key already used by provider reads.
	// Public IDs use a representative profile ID, never a path or fingerprint.
	accounts := make(map[string]string)
	for _, binding := range bindings {
		id, found := accounts[binding.key]
		if binding.key != "" && (!found || binding.profile.ID < id) {
			accounts[binding.key] = binding.profile.ID
		}
	}
	result := make([]dto.AgentProfileUsageDTO, 0, len(bindings))
	for _, binding := range bindings {
		entry := binding.entry
		if binding.key != "" {
			entry.AccountID = entry.Provider + ":" + accounts[binding.key]
		}
		if binding.client != nil {
			entry = s.read(ctx, binding, entry)
		}
		result = append(result, entry)
	}
	return result, nil
}

func (s *profileUsageSnapshot) bindings(ctx context.Context) ([]snapshotBinding, error) {
	agents, err := s.store.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	var bindings []snapshotBinding
	for _, agent := range agents {
		if agent == nil || agent.Name == "dynamic" {
			continue
		}
		profiles, err := s.store.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return nil, err
		}
		for _, profile := range profiles {
			if profile == nil || profile.DeletedAt != nil || profile.WorkspaceID != "" {
				continue
			}
			bindings = append(bindings, s.bind(profile, agent.Name))
		}
	}
	return bindings, nil
}

func (s *profileUsageSnapshot) bind(profile *settingsmodels.AgentProfile, agentName string) snapshotBinding {
	return s.bindWithProviderSelection(profile, agentName, snapshotClaudeProviderUncertain(profile))
}

func (s *profileUsageSnapshot) bindWithProviderSelection(profile *settingsmodels.AgentProfile, agentName string, providerUnknown bool) snapshotBinding {
	b := snapshotBinding{profile: profile, agentType: agentName, entry: dto.AgentProfileUsageDTO{
		ProfileID: profile.ID, ModelID: profile.Model, State: "unsupported", Source: "none",
		Windows: []dto.AgentProfileUsageWindowDTO{},
	}}
	if s.hostBindingUncertain(profile, agentName, providerUnknown) {
		b.entry.Reason = snapshotBindingUnknown
		return b
	}
	// The existing proxy resolver consumes an agent type, whereas the stored
	// profile references the agent row. Use a copy; never rewrite the profile.
	proxyProfile := *profile
	proxyProfile.AgentID = agentName
	if s.proxy != nil {
		if client, key, ok := s.proxy.Resolve(&proxyProfile); ok {
			b.client, b.key = client, key
			b.entry.Provider, b.entry.Source = "teamclaude", "proxy"
			b.entry.BindingScope = "local_proxy"
			return b
		}
	}
	if snapshotCredentialOverride(profile) || snapshotProcessOverride(agentName) {
		b.entry.Reason = snapshotBindingUnknown
		return b
	}
	switch agentName {
	case claudeACPAgentID:
		path := filepath.Join(s.home, ".claude", ".credentials.json")
		client := agentusage.NewClaudeUsageClientWithPath(path)
		b.entry.Provider = "anthropic"
		if client.HasSubscriptionCredentials() {
			b.client, b.key = client, agentusage.CacheKey("anthropic", path)
			b.entry.ModelClass = "subscription"
		}
	case snapshotCodexACPAgentID, snapshotCodexAppServerAgentID:
		path := filepath.Join(s.home, ".codex", "auth.json")
		client := agentusage.NewCodexUsageClientWithPath(path)
		b.entry.Provider = "openai"
		if client.HasSubscriptionCredentials() {
			b.client, b.key = client, agentusage.CacheKey("openai", path)
			b.entry.ModelClass = "subscription"
		}
	case "opencode-acp":
		return s.openCodeBinding(b)
	default:
		return b
	}
	b.entry.BindingScope = snapshotHostDefault
	if b.client == nil {
		b.entry.State, b.entry.Reason = "unavailable", "credential_missing"
	} else {
		b.entry.Source = "provider_api"
	}
	return b
}

func (s *profileUsageSnapshot) hostBindingUncertain(profile *settingsmodels.AgentProfile, agentName string, providerUnknown bool) bool {
	return (agentName == claudeACPAgentID && providerUnknown) || s.home == "" || !snapshotHostBinding(profile)
}

func (s *profileUsageSnapshot) openCodeBinding(b snapshotBinding) snapshotBinding {
	path := agentusage.OpenCodeAuthPath(s.home)
	model := strings.TrimSpace(b.profile.Model)
	switch {
	case strings.HasPrefix(model, "opencode-go/"):
		b.entry.Provider, b.entry.Plan = "opencode-go", "go"
		b.entry.BindingScope = snapshotHostDefault
		b.key = agentusage.CacheKey("opencode-go", path)
		if strings.HasSuffix(model, "-free") {
			b.entry.State, b.entry.ModelClass = "no_usage_api", "free"
		} else {
			b.entry.Source, b.entry.ModelClass = "provider_api", "paid"
			b.client = agentusage.NewOpenCodeGoUsageClient(path)
		}
	case strings.HasPrefix(model, "opencode/"):
		b.entry.Provider, b.entry.State = "opencode-zen", "no_usage_api"
		b.entry.BindingScope = snapshotHostDefault
		b.key = agentusage.CacheKey("opencode-zen", path)
		if strings.HasSuffix(model, "-free") {
			b.entry.ModelClass = "free"
		}
	}
	return b
}

func (s *profileUsageSnapshot) read(ctx context.Context, b snapshotBinding, entry dto.AgentProfileUsageDTO) dto.AgentProfileUsageDTO {
	s.usage.Register(b.profile.ID, b.client, b.key)
	reading, err := s.usage.GetUsage(ctx, b.profile.ID)
	if err != nil {
		entry.State = "unavailable"
		entry.Reason, entry.Status = agentusage.UsageReadFailure(err)
		return entry
	}
	if reading == nil || len(reading.Windows) == 0 {
		entry.State, entry.Reason = "unavailable", "no_reading"
		return entry
	}
	entry.Stale = reading.Stale
	entry.Plan = reading.Plan
	if !reading.FetchedAt.IsZero() {
		fetchedAt := reading.FetchedAt
		entry.FetchedAt = &fetchedAt
	}
	for _, window := range reading.Windows {
		if window.Observation == nil || !window.Observation.UtilizationObserved {
			continue
		}
		converted := dto.AgentProfileUsageWindowDTO{Label: window.Label, UtilizationPct: window.UtilizationPct,
			Scope: window.Scope, LimitReached: window.LimitReached}
		if window.Observation.ResetAtObserved && !window.ResetAt.IsZero() {
			resetAt := window.ResetAt
			converted.ResetAt = &resetAt
		}
		entry.Windows = append(entry.Windows, converted)
	}
	if len(entry.Windows) == 0 {
		entry.State, entry.Reason = "unavailable", "no_reading"
		return entry
	}
	entry.State = "ok"
	return entry
}

func snapshotHostBinding(profile *settingsmodels.AgentProfile) bool {
	if profile.CommandPrefix != "" || profile.ProviderKind != "" {
		return false
	}
	switch strings.TrimSpace(profile.ExecutorPreference) {
	case "", hostSourceLocal, "worktree", "standalone":
	default:
		return false
	}
	for _, env := range profile.EnvVars {
		switch snapshotEnvKey(env.Key) {
		case "HOME", "USERPROFILE", "XDG_DATA_HOME", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "OPENCODE_CONFIG", "OPENCODE_CONFIG_DIR", "OPENCODE_CONFIG_CONTENT":
			return false
		}
	}
	return true
}

func snapshotCredentialOverride(profile *settingsmodels.AgentProfile) bool {
	for _, env := range profile.EnvVars {
		switch snapshotEnvKey(env.Key) {
		case "ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_BASE_URL", "OPENAI_API_KEY", "OPENAI_BASE_URL", "USAGE_PROXY_VENDOR":
			return true
		}
	}
	return false
}

func snapshotEnvKey(key string) string {
	if runtime.GOOS == snapshotWindowsOS {
		return strings.ToUpper(key)
	}
	return key
}

func snapshotProcessOverride(agentName string) bool {
	var keys []string
	switch agentName {
	case claudeACPAgentID:
		keys = []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "CLAUDE_CONFIG_DIR"}
	case snapshotCodexACPAgentID, snapshotCodexAppServerAgentID:
		keys = []string{"CODEX_HOME", "OPENAI_API_KEY", "OPENAI_BASE_URL"}
	case "opencode-acp":
		keys = []string{"OPENCODE_CONFIG", "OPENCODE_CONFIG_DIR", "OPENCODE_CONFIG_CONTENT"}
	}
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}
