package scriptengine

import "fmt"

// RepositoryProvider returns git-related placeholders from metadata and environment.
// Parameters:
//   - metadata: executor create request metadata (contains "repository_path", "base_branch", etc.)
//   - env: environment variables (contains "GITHUB_TOKEN", etc.)
//   - repoURLResolver: resolves a local repo path to its remote URL (e.g., `git remote get-url origin`)
//   - tokenInjector: injects auth token into a clone URL
func RepositoryProvider(
	metadata map[string]any,
	env map[string]string,
	repoURLResolver func(string) (string, error),
	tokenInjector func(string, map[string]string) string,
) PlaceholderProvider {
	return func() map[string]string {
		vars := make(map[string]string)

		repoPath := getMetaString(metadata, "repository_path")
		if repoPath != "" {
			vars["repository.path"] = repoPath
			vars["repository.name"] = repoNameFromPath(repoPath)
		}

		branch := getMetaString(metadata, "base_branch")
		if branch == "" {
			branch = getMetaString(metadata, "repository_branch")
		}
		vars["repository.branch"] = branch

		setupScript := getMetaString(metadata, "setup_script")
		vars["repository.setup_script"] = setupScript

		// Clone URL: prefer explicit metadata, fall back to resolving from local repo
		cloneURL := getMetaString(metadata, "repository_clone_url")
		if cloneURL == "" && repoPath != "" && repoURLResolver != nil {
			if remoteURL, err := repoURLResolver(repoPath); err == nil && remoteURL != "" {
				vars["repository.ssh_url"] = remoteURL
				cloneURL = remoteURL
			}
		}
		if cloneURL != "" {
			vars["repository.clone_url"] = injectToken(cloneURL, env, tokenInjector)
		}

		return vars
	}
}

// AgentctlProvider returns kandev agentctl-related placeholders.
func AgentctlProvider(agentctlPort int, workspacePath string) PlaceholderProvider {
	return func() map[string]string {
		portStr := fmt.Sprintf("%d", agentctlPort)
		return map[string]string{
			"kandev.agentctl.port":    portStr,
			"kandev.agentctl.install": "chmod +x /usr/local/bin/agentctl",
			"kandev.agentctl.start": fmt.Sprintf(
				"nohup agentctl --port %s --workdir %s > /tmp/agentctl.log 2>&1 &\nsleep 1",
				portStr, workspacePath,
			),
		}
	}
}

// WorkspaceProvider returns workspace path placeholder.
func WorkspaceProvider(workspacePath string) PlaceholderProvider {
	return func() map[string]string {
		return map[string]string{
			"workspace.path": workspacePath,
		}
	}
}

// injectToken applies token injection to a URL if an injector is provided.
func injectToken(url string, env map[string]string, injector func(string, map[string]string) string) string {
	if injector != nil {
		return injector(url, env)
	}
	return url
}

// getMetaString extracts a string value from a metadata map.
func getMetaString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata[key].(string); ok {
		return v
	}
	return ""
}

// repoNameFromPath extracts the repository name from a file path.
func repoNameFromPath(repoPath string) string {
	if repoPath == "" {
		return ""
	}
	// Find last path component
	for i := len(repoPath) - 1; i >= 0; i-- {
		if repoPath[i] == '/' {
			return repoPath[i+1:]
		}
	}
	return repoPath
}
