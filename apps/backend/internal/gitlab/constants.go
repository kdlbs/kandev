package gitlab

// Auth method constants.
const (
	AuthMethodNone        = "none"
	AuthMethodPAT         = "pat"
	AuthMethodGLab        = "glab_cli"
	AuthMethodEnvironment = "environment"
)

const (
	approvalStateApproved = "approved"
	gitlabStateClosed     = "closed"
	gitlabStateLocked     = "locked"
	gitlabStateMerged     = "merged"
	gitlabStateOpened     = "opened"
	pipelineStateFailure  = "failure"
	pipelineStatusFailed  = "failed"
	pipelineStatusSuccess = "success"
	secretNameTokenLower  = "gitlab_token"
	connectionUnavailable = "GitLab connection unavailable"
)

// DefaultHost is the public GitLab.com host. Self-managed instances override
// this via the per-workspace gitlab_host setting.
const DefaultHost = "https://gitlab.com"

// SecretKey is the legacy installation-wide token identifier.
const SecretKey = "GITLAB_TOKEN"

// SecretKeyForWorkspace returns the workspace-owned PAT identifier.
func SecretKeyForWorkspace(workspaceID string) string {
	return "gitlab:" + workspaceID + ":token"
}
