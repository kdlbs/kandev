package acp

import "github.com/kandev/kandev/internal/agentctl/types/streams"

func (a *Adapter) beginContinuationPermission(sessionID, toolCallID string, options []PermissionOption) func(*PermissionResponse, error) {
	a.mu.RLock()
	enabled := a.cfg.ProviderInterruptionContinuation && sessionID == a.sessionID && !a.isLoadingSession
	support := a.dialect.continuationSupport
	a.mu.RUnlock()
	turn := a.currentPromptTurn()
	if !enabled || turn == nil || turn.promptGeneration == 0 {
		return nil
	}
	turn.evidenceMu.Lock()
	if support != streams.ContinuationNativeSavedHistoryV2 || toolCallID == "" ||
		turn.continuationPermissions >= continuationToolLimit || len(turn.continuationPermissionTools) >= continuationToolLimit {
		turn.continuationUnsafe = true
		turn.evidenceMu.Unlock()
		return nil
	}
	turn.continuationPermissions++
	if turn.continuationPermissionTools == nil {
		turn.continuationPermissionTools = make(map[string]struct{})
	}
	turn.continuationPermissionTools[toolCallID] = struct{}{}
	turn.evidenceMu.Unlock()
	offered := append([]PermissionOption(nil), options...)
	// The captured turn owns resolution even after a successor starts.
	return func(response *PermissionResponse, err error) {
		turn.evidenceMu.Lock()
		defer turn.evidenceMu.Unlock()
		turn.continuationPermissions--
		if err != nil || !continuationPermissionAllowed(offered, response) {
			turn.continuationUnsafe = true
		}
	}
}

func continuationPermissionAllowed(options []PermissionOption, response *PermissionResponse) bool {
	if response == nil || response.Cancelled || response.OptionID == "" {
		return false
	}
	matches, allowed := 0, false
	for _, option := range options {
		if option.OptionID == response.OptionID {
			matches++
			allowed = option.Kind == streams.PermissionOptionKindAllowOnce || option.Kind == streams.PermissionOptionKindAllowAlways
		}
	}
	return matches == 1 && allowed
}
