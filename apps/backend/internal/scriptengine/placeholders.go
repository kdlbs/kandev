package scriptengine

// PlaceholderInfo describes an available placeholder for documentation/autocomplete.
type PlaceholderInfo struct {
	Key           string   `json:"key"`
	Description   string   `json:"description"`
	Example       string   `json:"example"`
	ExecutorTypes []string `json:"executor_types"`
}

// DefaultPlaceholders is the registry of all available script template placeholders.
var DefaultPlaceholders = []PlaceholderInfo{
	{
		Key:           "repository.path",
		Description:   "Local repository path on the host machine",
		Example:       "/Users/dev/myapp",
		ExecutorTypes: []string{"local", "worktree"},
	},
	{
		Key:           "repository.name",
		Description:   "Repository name",
		Example:       "myapp",
		ExecutorTypes: []string{"local", "worktree", "local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "repository.clone_url",
		Description:   "HTTPS clone URL (with auth token injected if available)",
		Example:       "https://token@github.com/org/myapp.git",
		ExecutorTypes: []string{"local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "repository.ssh_url",
		Description:   "SSH clone URL",
		Example:       "git@github.com:org/myapp.git",
		ExecutorTypes: []string{"local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "repository.branch",
		Description:   "Target branch name",
		Example:       "main",
		ExecutorTypes: []string{"local", "worktree", "local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "repository.setup_script",
		Description:   "Repository-level setup script (if configured in repo settings)",
		Example:       "npm install",
		ExecutorTypes: []string{"local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "workspace.path",
		Description:   "Working directory inside the executor environment",
		Example:       "/workspace",
		ExecutorTypes: []string{"local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "kandev.agentctl.install",
		Description:   "Expands to full agentctl binary upload and install commands",
		Example:       "# (multi-line: upload binary, chmod, verify)",
		ExecutorTypes: []string{"local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "kandev.agentctl.start",
		Description:   "Expands to agentctl start command with configured flags",
		Example:       "agentctl --port 8765 --workspace /workspace &",
		ExecutorTypes: []string{"local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "kandev.agentctl.port",
		Description:   "Agentctl port number",
		Example:       "8765",
		ExecutorTypes: []string{"local_docker", "remote_docker", "sprites"},
	},
}
