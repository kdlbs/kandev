package scriptengine

// PlaceholderInfo describes an available placeholder for documentation/autocomplete.
type PlaceholderInfo struct {
	Key           string   `json:"key"`
	Description   string   `json:"description"`
	Example       string   `json:"example"`
	ExecutorTypes []string `json:"executor_types"`
}

const (
	executorTypeLocal        = "local"
	executorTypeWorktree     = "worktree"
	executorTypeLocalDocker  = "local_docker"
	executorTypeRemoteDocker = "remote_docker"
	executorTypeSprites      = "sprites"
	executorTypeKubernetes   = "k8s"
)

// DefaultPlaceholders is the registry of all available script template placeholders.
var DefaultPlaceholders = []PlaceholderInfo{
	{
		Key:           "repository.path",
		Description:   "Local repository path on the host machine",
		Example:       "/Users/dev/myapp",
		ExecutorTypes: []string{executorTypeLocal, executorTypeWorktree},
	},
	{
		Key:           "repository.name",
		Description:   "Repository name",
		Example:       "myapp",
		ExecutorTypes: []string{executorTypeLocal, executorTypeWorktree, executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "repository.clone_url",
		Description:   "HTTPS clone URL (with auth token injected if available)",
		Example:       "https://token@github.com/org/myapp.git",
		ExecutorTypes: []string{executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "repository.ssh_url",
		Description:   "SSH clone URL",
		Example:       "git@github.com:org/myapp.git",
		ExecutorTypes: []string{executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "repository.branch",
		Description:   "Target branch name",
		Example:       "main",
		ExecutorTypes: []string{executorTypeLocal, executorTypeWorktree, executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "repository.setup_script",
		Description:   "Repository-level setup script (if configured in repo settings)",
		Example:       "npm install",
		ExecutorTypes: []string{executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "git.user_name",
		Description:   "Git author name configured for remote executor",
		Example:       "Jane Developer",
		ExecutorTypes: []string{executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "git.user_email",
		Description:   "Git author email configured for remote executor",
		Example:       "jane@example.com",
		ExecutorTypes: []string{executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "git.identity_setup",
		Description:   "Expands to git config commands when name/email are provided",
		Example:       "git config --global user.name 'Jane Developer'",
		ExecutorTypes: []string{executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "workspace.path",
		Description:   "Working directory for the current executor",
		Example:       "/workspace",
		ExecutorTypes: []string{executorTypeLocal, executorTypeWorktree, executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "worktree.base_path",
		Description:   "Base directory where worktrees are stored",
		Example:       "/Users/dev/.kandev/worktrees",
		ExecutorTypes: []string{executorTypeWorktree},
	},
	{
		Key:           "worktree.path",
		Description:   "Resolved worktree directory path for this session",
		Example:       "/Users/dev/.kandev/worktrees/fix-bug_ab12cd34",
		ExecutorTypes: []string{executorTypeWorktree},
	},
	{
		Key:           "worktree.id",
		Description:   "Worktree ID for this session",
		Example:       "f4db4fa6-82f4-4d8d-b29c-6ffbd44f57de",
		ExecutorTypes: []string{executorTypeWorktree},
	},
	{
		Key:           "worktree.branch",
		Description:   "Created/reused worktree branch name",
		Example:       "feature/fix-login-abc",
		ExecutorTypes: []string{executorTypeWorktree},
	},
	{
		Key:           "worktree.base_branch",
		Description:   "Base branch used for worktree creation",
		Example:       "main",
		ExecutorTypes: []string{executorTypeWorktree},
	},
	{
		Key:           "kandev.agents.install",
		Description:   "Pre-installs agent CLIs globally for all agents configured on the profile",
		Example:       "npm install -g @anthropic-ai/claude-code@2.1.50",
		ExecutorTypes: []string{executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "kandev.agentctl.install",
		Description:   "Expands to full agentctl binary upload and install commands",
		Example:       "# (multi-line: upload binary, chmod, verify)",
		ExecutorTypes: []string{executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "kandev.agentctl.start",
		Description:   "Expands to agentctl start command with configured flags",
		Example:       "agentctl --port 8765 --workspace /workspace &",
		ExecutorTypes: []string{executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "kandev.agentctl.port",
		Description:   "Agentctl port number",
		Example:       "8765",
		ExecutorTypes: []string{executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
	{
		Key:           "github.auth_setup",
		Description:   "Expands to commands that configure gh CLI and git credential helper for GitHub token auth",
		Example:       "git config --global credential.https://github.com.helper '...'",
		ExecutorTypes: []string{executorTypeLocalDocker, executorTypeRemoteDocker, executorTypeSprites, executorTypeKubernetes},
	},
}
