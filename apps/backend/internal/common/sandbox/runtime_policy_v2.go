package sandbox

// StaticAccess returns the initial permissions without conditional expansion.
func (policy PolicyV2) StaticAccess() (Grants, error) {
	if policy.SchemaVersion != PolicySchemaV2 {
		return Grants{}, ErrInvalidPolicy
	}
	return permissionGrantsV2(policy.StaticGrants)
}

// ResolveAttemptPolicyV2 binds both permission groups to the same owned paths.
func ResolveAttemptPolicyV2(policy PolicyV2, workDir string, state *AgentState, protectedRoots []string, sessionID string) (PolicyV2, error) {
	canonical, err := CanonicalizePolicyV2(policy, protectedRoots)
	if err != nil {
		return PolicyV2{}, err
	}
	static, err := resolvePermissionsV2(canonical.StaticGrants, workDir, state, protectedRoots, sessionID)
	if err != nil {
		return PolicyV2{}, err
	}
	envelope, err := resolvePermissionsV2(canonical.ApprovalEnvelope, workDir, state, protectedRoots, sessionID)
	if err != nil {
		return PolicyV2{}, err
	}
	return PolicyV2{SchemaVersion: PolicySchemaV2, StaticGrants: static, ApprovalEnvelope: envelope}, nil
}

func resolvePermissionsV2(permissions PermissionsV2, workDir string, state *AgentState, protectedRoots []string, sessionID string) (PermissionsV2, error) {
	grants, err := permissionGrantsV2(permissions)
	if err != nil {
		return PermissionsV2{}, err
	}
	resolved, err := ResolveAttemptGrants(grants, workDir, state, protectedRoots, sessionID)
	if err != nil {
		return PermissionsV2{}, err
	}
	tcp := make([]TCPDestination, 0, len(resolved.Network))
	for _, grant := range resolved.Network {
		tcp = append(tcp, TCPDestination{Host: grant.Host, Port: grant.Port, Transport: grant.Protocol})
	}
	return PermissionsV2{Filesystem: resolved.Filesystem, TCP: tcp, EnvironmentKeys: resolved.EnvironmentKeys, KandevMCP: resolved.KandevMCP}, nil
}
