package models

// TaskGitHubIssueLink is the complete metadata-backed GitHub issue identity.
type TaskGitHubIssueLink struct {
	URL    string
	Number int
	Owner  string
	Repo   string
}
