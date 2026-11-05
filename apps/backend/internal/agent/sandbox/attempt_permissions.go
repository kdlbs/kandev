package sandbox

import contract "github.com/kandev/kandev/internal/common/sandbox"

type attemptPermissions struct {
	static   contract.Grants
	ceiling  contract.Grants
	policyV2 *contract.PolicyV2
}

func (s *Service) resolveCapturedPermissions(resolved *Resolved, request CaptureRequest, roots []string) (attemptPermissions, error) {
	result := attemptPermissions{}
	limit := resolved.Ceiling.Policy.Grants
	if typed := resolved.Ceiling.Policy.PolicyV2; typed != nil {
		var err error
		limit, err = typed.StaticAccess()
		if err != nil {
			return result, err
		}
	}
	var err error
	result.ceiling, err = resolveAttemptCeiling(limit, request, s.protectedRoots)
	if err != nil {
		return result, err
	}
	if typed := resolved.Revision.Policy.PolicyV2; typed != nil {
		if request.Provider.ProtocolVersion() != 2 {
			return result, ErrIneligible
		}
		value, resolveErr := contract.ResolveAttemptPolicyV2(*typed, request.WorkDir, request.AgentState, roots, request.SessionID)
		if resolveErr != nil {
			return result, resolveErr
		}
		result.policyV2 = &value
		result.static, err = value.StaticAccess()
		if err == nil {
			err = contract.WithinLegacyCeilingV2(value, result.ceiling)
		}
		return result, err
	}
	result.static, err = contract.ResolveAttemptGrants(resolved.Revision.Policy.Grants, request.WorkDir, request.AgentState, roots, request.SessionID)
	if err == nil {
		err = contract.WithinCeiling(result.static, result.ceiling)
	}
	return result, err
}
