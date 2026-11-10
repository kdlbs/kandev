package github

// WorkspacePATSecretKeyForHost binds stored PAT material to the provider authority.
func WorkspacePATSecretKeyForHost(workspaceID, hostname string) string {
	if hostname == "" || hostname == defaultGitHubHost {
		return WorkspacePATSecretKey(workspaceID)
	}
	return "github:workspace:" + workspaceID + ":host:" + hostname + ":pat"
}

// UserAccessTokenSecretKeyForHost binds OAuth access material to its registration host.
func UserAccessTokenSecretKeyForHost(workspaceID, userID, hostname string) string {
	if hostname == "" || hostname == defaultGitHubHost {
		return UserAccessTokenSecretKey(workspaceID, userID)
	}
	return "github:user:" + workspaceID + ":" + userID + ":host:" + hostname + ":access"
}

// UserRefreshTokenSecretKeyForHost binds OAuth refresh material to its registration host.
func UserRefreshTokenSecretKeyForHost(workspaceID, userID, hostname string) string {
	if hostname == "" || hostname == defaultGitHubHost {
		return UserRefreshTokenSecretKey(workspaceID, userID)
	}
	return "github:user:" + workspaceID + ":" + userID + ":host:" + hostname + ":refresh"
}
