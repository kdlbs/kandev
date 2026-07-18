// Package azuredevops implements the Azure DevOps Services integration.
package azuredevops

import "time"

const AuthMethodPAT = "pat"

// Config is the workspace-scoped Azure DevOps connection configuration. The
// PAT is stored separately in the encrypted secret store.
type Config struct {
	WorkspaceID        string     `json:"workspaceId" db:"workspace_id"`
	OrganizationURL    string     `json:"organizationUrl" db:"organization_url"`
	DefaultProjectID   string     `json:"defaultProjectId,omitempty" db:"default_project_id"`
	DefaultProjectName string     `json:"defaultProjectName,omitempty" db:"default_project_name"`
	AuthMethod         string     `json:"authMethod" db:"auth_method"`
	HasSecret          bool       `json:"hasSecret" db:"-"`
	LastCheckedAt      *time.Time `json:"lastCheckedAt,omitempty" db:"last_checked_at"`
	LastOK             bool       `json:"lastOk" db:"last_ok"`
	LastError          string     `json:"lastError,omitempty" db:"last_error"`
	CreatedAt          time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time  `json:"updatedAt" db:"updated_at"`
}

// SetConfigRequest creates or updates a workspace connection. An empty PAT on
// update retains the currently stored credential.
type SetConfigRequest struct {
	OrganizationURL    string `json:"organizationUrl"`
	DefaultProjectID   string `json:"defaultProjectId"`
	DefaultProjectName string `json:"defaultProjectName"`
	AuthMethod         string `json:"authMethod"`
	PAT                string `json:"pat"`
}

// TestConnectionResult reports the result of an authenticated identity probe.
type TestConnectionResult struct {
	OK          bool   `json:"ok"`
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
	Error       string `json:"error,omitempty"`
}

// TaskPR is the persisted summary of an Azure pull request associated with a
// Kandev task repository. Azure-native feedback remains transient.
type TaskPR struct {
	ID                string     `json:"id" db:"id"`
	TaskID            string     `json:"taskId" db:"task_id"`
	RepositoryID      string     `json:"repositoryId" db:"repository_id"`
	OrganizationURL   string     `json:"organizationUrl" db:"organization_url"`
	ProjectID         string     `json:"projectId" db:"project_id"`
	AzureRepositoryID string     `json:"azureRepositoryId" db:"azure_repository_id"`
	PullRequestID     int        `json:"pullRequestId" db:"pull_request_id"`
	PullRequestURL    string     `json:"pullRequestUrl" db:"pull_request_url"`
	Title             string     `json:"title" db:"title"`
	SourceBranch      string     `json:"sourceBranch" db:"source_branch"`
	TargetBranch      string     `json:"targetBranch" db:"target_branch"`
	AuthorID          string     `json:"authorId" db:"author_id"`
	AuthorName        string     `json:"authorName" db:"author_name"`
	Status            string     `json:"status" db:"status"`
	ReviewState       string     `json:"reviewState,omitempty" db:"review_state"`
	PolicyState       string     `json:"policyState,omitempty" db:"policy_state"`
	IsDraft           bool       `json:"isDraft" db:"is_draft"`
	LastSyncedAt      *time.Time `json:"lastSyncedAt,omitempty" db:"last_synced_at"`
	CreatedAt         time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time  `json:"updatedAt" db:"updated_at"`
}

// TaskPRsResponse groups workspace associations by task ID.
type TaskPRsResponse struct {
	TaskPRs map[string][]*TaskPR `json:"taskPrs"`
}

// SecretKeyForWorkspace returns the workspace-isolated encrypted PAT key.
func SecretKeyForWorkspace(workspaceID string) string {
	return "azure_devops:" + workspaceID + ":pat"
}
