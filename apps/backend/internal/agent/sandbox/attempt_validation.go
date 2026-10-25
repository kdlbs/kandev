package sandbox

import (
	"context"
	"slices"
	"time"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (s *Service) captureSnapshot(ctx context.Context, request CaptureRequest) (AttemptSnapshot, error) {
	if !s.enabled {
		return AttemptSnapshot{}, ErrDisabled
	}
	if !validAttemptCapture(request) {
		return AttemptSnapshot{}, contract.ErrInvalidPolicy
	}
	resolved, previous, err := s.resolveCapture(ctx, request)
	if err != nil {
		return AttemptSnapshot{}, err
	}
	if resolved == nil || resolved.Revision.ID != request.Resolved.Revision.ID || resolved.Revision.Digest != request.Launch.PolicyDigest || resolved.BindingVersion != request.Resolved.BindingVersion || !slices.Equal(resolved.Bindings, request.Resolved.Bindings) {
		return AttemptSnapshot{}, ErrConflict
	}
	if !validAttemptProvider(request.Provider, resolved, request.Launch) {
		return AttemptSnapshot{}, contract.ErrInvalidPolicy
	}
	roots := append(slices.Clone(s.protectedRoots), request.Launch.Root)
	grants, err := contract.ResolveAttemptGrants(resolved.Revision.Policy.Grants, request.WorkDir, request.AgentState, roots, request.SessionID)
	if err != nil {
		return AttemptSnapshot{}, err
	}
	ceiling, err := resolveAttemptCeiling(resolved.Ceiling.Policy.Grants, request, s.protectedRoots)
	if err != nil {
		return AttemptSnapshot{}, err
	}
	if err = contract.WithinCeiling(grants, ceiling); err != nil {
		return AttemptSnapshot{}, err
	}
	if !validCapturedAgentState(request) {
		return AttemptSnapshot{}, contract.ErrInvalidPolicy
	}
	restartVersion := int64(0)
	if previous != nil {
		restartVersion = previous.Version
	}
	return AttemptSnapshot{RestartOf: request.RestartOf, RestartVersion: restartVersion, AgentProfileID: request.Selection.AgentProfileID, AgentState: request.AgentState.Snapshot(), SchemaVersion: 1, OrgID: request.Selection.OrgID, WorkspaceID: request.Selection.WorkspaceID, TaskID: request.Selection.TaskID,
		SessionID: request.SessionID, ExecutionID: request.ExecutionID, WorkDir: request.WorkDir, Resolved: resolved, Grants: grants, Ceiling: ceiling,
		Launch: request.Launch.Clone(), Provider: request.Provider, CreatedAt: time.Now().UTC()}, nil
}

func validAttemptCapture(request CaptureRequest) bool {
	return request.Launch != nil && request.Launch.Validate() == nil && request.Resolved != nil && request.Resolved.Revision != nil && request.ExecutionID != "" && len(request.ExecutionID) <= 128
}

func validAttemptProvider(provider AttemptProvider, resolved *Resolved, launch *contract.Launch) bool {
	policy := resolved.Revision.Policy
	return provider.PluginID != "" && len(provider.PluginID) <= 128 && provider.InstallationID == policy.InstallationID && provider.Key == policy.ProviderKey &&
		provider.Version != "" && len(provider.Version) <= 128 && provider.RuntimeVersion != "" && len(provider.RuntimeVersion) <= 128 &&
		provider.PackageDigest == launch.PackageDigest && provider.RuntimeDigest == launch.Digests["runtime"]
}

func validAttemptTransition(before, after string) bool {
	switch before {
	case AttemptPrepared:
		return slices.Contains([]string{AttemptStarting, AttemptFailed, AttemptStopped, AttemptRecoveryRequired}, after)
	case AttemptStarting:
		return slices.Contains([]string{AttemptStarting, AttemptRunning, AttemptFailed, AttemptStopped, AttemptRecoveryRequired}, after)
	case AttemptRunning:
		return slices.Contains([]string{AttemptRunning, AttemptFailed, AttemptStopped, AttemptRecoveryRequired}, after)
	case AttemptRecoveryRequired:
		return after == AttemptStopped || after == AttemptFailed
	default:
		return false
	}
}

func validAttemptOwner(previous, next *AttemptOwner, state string) bool {
	if next == nil {
		return state != AttemptRunning
	}
	if !completeAttemptOwner(next) {
		return false
	}
	if previous == nil {
		return true
	}
	return next.SupervisorPID == previous.SupervisorPID && next.SupervisorBirth == previous.SupervisorBirth && next.AgentctlIdentity == previous.AgentctlIdentity &&
		(previous.ChildPID == 0 || (next.ChildPID == previous.ChildPID && next.ChildBirth == previous.ChildBirth)) &&
		(previous.ProviderSessionID == "" || next.ProviderSessionID == previous.ProviderSessionID)
}

func completeAttemptOwner(next *AttemptOwner) bool {
	if next.SupervisorPID <= 0 || next.SupervisorBirth == "" || len(next.SupervisorBirth) > 256 || next.AgentctlIdentity == "" || len(next.AgentctlIdentity) > 128 {
		return false
	}
	if next.ChildPID < 0 || len(next.ChildBirth) > 256 || len(next.ProviderSessionID) > 256 {
		return false
	}
	if (next.ChildPID == 0) != (next.ChildBirth == "") {
		return false
	}
	if next.ProviderSessionID != "" && next.ChildPID == 0 {
		return false
	}
	return true
}

func validCapturedAgentState(request CaptureRequest) bool {
	snapshot := request.AgentState.Snapshot()
	if snapshot == nil {
		return true
	}
	expected := contract.AgentStateScope{OrgID: request.Selection.OrgID, WorkspaceID: request.Selection.WorkspaceID, SessionID: request.SessionID, AgentProfileID: request.Selection.AgentProfileID, InstallationID: request.Provider.InstallationID, ProviderKey: request.Provider.Key}
	if expected.SessionID == "" {
		expected.ExecutionID = request.ExecutionID
	}
	return snapshot.Scope == expected
}

func resolveAttemptCeiling(grants contract.Grants, request CaptureRequest, protected []string) (contract.Grants, error) {
	if request.AgentState == nil {
		grants.Filesystem = slices.DeleteFunc(slices.Clone(grants.Filesystem), func(grant contract.FilesystemGrant) bool { return grant.Reference == contract.ReferenceAgentState })
	}
	return contract.ResolveRuntimeDirectories(grants, request.WorkDir, request.AgentState, protected)
}
