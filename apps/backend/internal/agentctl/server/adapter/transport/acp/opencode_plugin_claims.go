package acp

import (
	"sort"

	"github.com/kandev/kandev/internal/agentctl/server/opencodeplugin"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// OpenCodePluginClaims returns the immutable launch bindings confirmed for the
// current native session. The process manager enriches submission attribution
// and publishes them to the launch host.
func (a *Adapter) OpenCodePluginClaims() []opencodeplugin.Claim {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.agentID != openCodeAgentID || len(a.openCodePluginClaims) == 0 {
		return nil
	}
	claims := make([]opencodeplugin.Claim, 0, len(a.openCodePluginClaims))
	for _, claim := range a.openCodePluginClaims {
		claims = append(claims, claim)
	}
	sort.Slice(claims, func(i, j int) bool { return claims[i].ShellID < claims[j].ShellID })
	return claims
}

// recordOpenCodePluginClaim retains one confirmed launch outside active tool
// calls. Only OpenCode sessions with shared background work enabled produce a
// claim; every other provider and configuration stays unchanged.
func (a *Adapter) recordOpenCodePluginClaimLocked(toolCallID string, promptGeneration uint64, payload *streams.NormalizedPayload) {
	if a == nil || a.agentID != openCodeAgentID || !a.backgroundWorkEnabled || toolCallID == "" || payload == nil {
		return
	}
	shell := payload.ShellExec()
	work := payload.BackgroundWork()
	if shell == nil || work == nil || work.WorkID == "" || !work.Unverified || !work.Detached {
		return
	}
	title := shell.Description
	if title == "" {
		title = shell.Command
	}
	if a.sessionID == "" || a.nativeGeneration == 0 {
		return
	}
	if len(a.openCodePluginClaims) >= opencodeplugin.MaxClaims {
		return
	}
	a.openCodePluginClaims[work.WorkID] = opencodeplugin.Claim{
		ShellID:          work.WorkID,
		NativeSessionID:  a.sessionID,
		NativeGeneration: a.nativeGeneration,
		WorkID:           work.WorkID,
		RunID:            work.RunID,
		SourceCallID:     toolCallID,
		Title:            title,
		PromptGeneration: promptGeneration,
	}
}

// ValidateOpenCodePluginAdmission fences a plugin observation against the
// current native session generation. Shell claims add their own exact claim
// check; other approved behaviors inherit the shared session fence.
func (a *Adapter) ValidateOpenCodePluginAdmission(admission opencodeplugin.Admission) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.agentID != openCodeAgentID || a.sessionID == "" || a.nativeGeneration == 0 {
		return false
	}
	if admission.NativeSessionID != a.sessionID || admission.NativeGeneration != a.nativeGeneration {
		return false
	}
	if admission.BehaviorID != opencodeplugin.BehaviorShellOutcomes {
		return true
	}
	claim, ok := a.openCodePluginClaims[admission.ClaimKey]
	return ok &&
		claim.NativeSessionID == admission.NativeSessionID &&
		claim.NativeGeneration == admission.NativeGeneration &&
		claim.WorkID == admission.Observation.WorkID
}

// BeginOpenCodePluginAdmission holds the native-session read fence until the
// returned release function runs. A new session, load, resume, or reset needs
// the write side, so it cannot revoke ownership while the plugin observation is
// being committed.
func (a *Adapter) BeginOpenCodePluginAdmission(admission opencodeplugin.Admission) (func(), bool) {
	if a == nil {
		return nil, false
	}
	a.mu.RLock()
	if a.agentID != openCodeAgentID || a.sessionID == "" || a.nativeGeneration == 0 ||
		admission.NativeSessionID != a.sessionID || admission.NativeGeneration != a.nativeGeneration {
		a.mu.RUnlock()
		return nil, false
	}
	if admission.BehaviorID == opencodeplugin.BehaviorShellOutcomes {
		claim, ok := a.openCodePluginClaims[admission.ClaimKey]
		if !ok || claim.NativeSessionID != admission.NativeSessionID ||
			claim.NativeGeneration != admission.NativeGeneration || claim.WorkID != admission.Observation.WorkID {
			a.mu.RUnlock()
			return nil, false
		}
	}
	return a.mu.RUnlock, true
}

// RetireOpenCodePluginClaim removes an exactly matching accepted claim. A
// stale acknowledgement cannot remove a replacement claim.
func (a *Adapter) RetireOpenCodePluginClaim(admission opencodeplugin.Admission) {
	if a == nil || admission.BehaviorID != opencodeplugin.BehaviorShellOutcomes {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	claim, ok := a.openCodePluginClaims[admission.ClaimKey]
	if !ok || claim.NativeSessionID != admission.NativeSessionID ||
		claim.NativeGeneration != admission.NativeGeneration || claim.WorkID != admission.Observation.WorkID {
		return
	}
	delete(a.openCodePluginClaims, admission.ClaimKey)
}

// advanceNativeGenerationLocked starts a new native conversation identity and
// removes every claim from the superseded one. Callers hold a.mu.
func (a *Adapter) advanceNativeGenerationLocked() {
	a.nativeGeneration++
	clear(a.openCodePluginClaims)
}
