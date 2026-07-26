package github

// GitHubPushEventPayload is published on events.GitHubPushReceived after a
// push webhook is verified and its installation resolved to workspaces.
type GitHubPushEventPayload struct {
	WorkspaceIDs  []string
	Owner         string
	Name          string
	Branch        string // ref with refs/heads/ stripped
	SHA           string // head commit (payload "after")
	PusherLogin   string
	HeadCommitMsg string // head_commit.message (may be empty for some pushes)
}

// GitHubCheckRunEventPayload is published on events.GitHubCheckRunCompleted
// for a completed check_run.
type GitHubCheckRunEventPayload struct {
	WorkspaceIDs []string
	Owner        string
	Name         string // repository name
	Branch       string // check_run.check_suite.head_branch
	SHA          string // check_run.head_sha
	CheckName    string // check_run.name
	Conclusion   string // check_run.conclusion (success, failure, timed_out, ...)
	CheckRunID   int64  // check_run.id — for dedup uniqueness
	HTMLURL      string // check_run.html_url — the check's page on GitHub
}
