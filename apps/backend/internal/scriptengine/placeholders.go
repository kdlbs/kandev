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
		Key:           "git.user_name",
		Description:   "Git author name configured for remote executor",
		Example:       "Jane Developer",
		ExecutorTypes: []string{"local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "git.user_email",
		Description:   "Git author email configured for remote executor",
		Example:       "jane@example.com",
		ExecutorTypes: []string{"local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "git.identity_setup",
		Description:   "Expands to git config commands when name/email are provided",
		Example:       "git config --global user.name 'Jane Developer'",
		ExecutorTypes: []string{"local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "workspace.path",
		Description:   "Working directory for the current executor",
		Example:       "/workspace",
		ExecutorTypes: []string{"local", "worktree", "local_docker", "remote_docker", "sprites"},
	},
	{
		Key:           "worktree.base_path",
		Description:   "Base directory where worktrees are stored",
		Example:       "/Users/dev/.kandev/worktrees",
		ExecutorTypes: []string{"worktree"},
	},
	{
		Key:           "worktree.path",
		Description:   "Resolved worktree directory path for this session",
		Example:       "/Users/dev/.kandev/worktrees/fix-bug_ab12cd34",
		ExecutorTypes: []string{"worktree"},
	},
	{
		Key:           "worktree.id",
		Description:   "Worktree ID for this session",
		Example:       "f4db4fa6-82f4-4d8d-b29c-6ffbd44f57de",
		ExecutorTypes: []string{"worktree"},
	},
	{
		Key:           "worktree.branch",
		Description:   "Created/reused worktree branch name",
		Example:       "feature/fix-login-abc",
		ExecutorTypes: []string{"worktree"},
	},
	{
		Key:           "worktree.base_branch",
		Description:   "Base branch used for worktree creation",
		Example:       "main",
		ExecutorTypes: []string{"worktree"},
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
