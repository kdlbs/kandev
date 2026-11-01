package runtime

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// SetKeywordCredentialBindingResolver supplies binding evidence from the actual
// launch. A sessionless or unproven binding cannot establish account scope.
// Configure it during startup before enabling keyword context.
func (r *ProfileExecutionResolver) SetKeywordCredentialBindingResolver(resolve func(context.Context, string, *agentsettingsmodels.AgentProfile, string) dynamic.CredentialBindingDescriptor) {
	r.keywordBindingResolver = resolve
}

func (r *ProfileExecutionResolver) keywordCredentialBinding(ctx context.Context, sessionID string, profile *agentsettingsmodels.AgentProfile) dynamic.CredentialBindingDescriptor {
	if profile == nil || sessionID == "" || !r.keywordContextEnabled.Load() || r.keywordBindingResolver == nil {
		return dynamic.CredentialBindingDescriptor{}
	}
	name, err := r.agentNameForProfile(ctx, profile)
	if err != nil {
		return dynamic.CredentialBindingDescriptor{}
	}
	binding := r.keywordBindingResolver(ctx, sessionID, profile, name)
	if binding.AgentFamilyID != profile.AgentID || strings.TrimSpace(binding.CredentialLocator) == "" ||
		strings.TrimSpace(binding.ExecutorNamespace) == "" ||
		(binding.AuthenticationMethod != "subscription" && binding.AuthenticationMethod != "api_key") {
		return dynamic.CredentialBindingDescriptor{}
	}
	return binding
}

func (r *ProfileExecutionResolver) populateKeywordCandidate(ctx context.Context, sessionID string, profile *agentsettingsmodels.AgentProfile, candidate *dynamic.Candidate) {
	if profile == nil || !candidate.KeywordContextEnabled {
		return
	}
	candidate.ModelID = profile.Model
	if r.bindingResolver != nil {
		candidate.KeywordAccountID = r.bindingResolver.Fingerprint(r.keywordCredentialBinding(ctx, sessionID, profile))
	}
}

// ProviderKeywordContext reuses the existing credential-binding identity.
// Missing billing or credential evidence cannot establish account scope.
func (r *ProfileExecutionResolver) ProviderKeywordContext(ctx context.Context, profileID string, sessionIDs ...string) routingerr.KeywordContext {
	var result routingerr.KeywordContext
	if r == nil || r.profiles == nil || !r.keywordContextEnabled.Load() {
		return result
	}
	profile, err := r.profiles.GetAgentProfile(ctx, profileID)
	if err != nil || profile == nil {
		return result
	}
	result.ModelID = profile.Model
	result.PathID = profile.AgentID
	if r.bindingResolver != nil && len(sessionIDs) != 0 {
		binding := r.keywordCredentialBinding(ctx, sessionIDs[0], profile)
		result.AccountID = r.bindingResolver.Fingerprint(binding)
		if result.AccountID != "" {
			result.Tier = binding.AuthenticationMethod
		}
	}
	return result
}
