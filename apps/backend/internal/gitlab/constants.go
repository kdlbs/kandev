package gitlab

// Auth method constants.
const (
	AuthMethodNone = "none"
	AuthMethodPAT  = "pat"
	AuthMethodGLab = "glab_cli"
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
)

// DefaultHost is the public GitLab.com host. Self-managed instances override
// this via the per-workspace gitlab_host setting.
const DefaultHost = "https://gitlab.com"
