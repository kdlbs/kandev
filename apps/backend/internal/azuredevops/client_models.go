package azuredevops

import (
	"encoding/json"
	"time"
)

// Project is an Azure DevOps project visible to the authenticated user.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Repository is an Azure Repos Git repository.
type Repository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ProjectID     string `json:"projectId"`
	ProjectName   string `json:"projectName"`
	DefaultBranch string `json:"defaultBranch"`
	WebURL        string `json:"webUrl"`
}

// Identity is the stable subset shared by Azure authors, reviewers, and
// comment authors.
type Identity struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName,omitempty"`
}

// WorkItem is a hydrated Azure Boards work item. Fields retains provider
// extensions while the named fields drive the Kandev browse UI.
type WorkItem struct {
	ID          int            `json:"id"`
	Revision    int            `json:"revision"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	State       string         `json:"state"`
	Type        string         `json:"type"`
	Project     string         `json:"project,omitempty"`
	AreaPath    string         `json:"areaPath,omitempty"`
	AssignedTo  string         `json:"assignedTo,omitempty"`
	WebURL      string         `json:"webUrl,omitempty"`
	APIURL      string         `json:"apiUrl,omitempty"`
	Fields      map[string]any `json:"fields,omitempty"`
}

type WorkItemSearchResult struct {
	Items []WorkItem `json:"items"`
	Count int        `json:"count"`
}

type PullRequestFilter struct {
	ProjectID    string
	RepositoryID string
	Status       string
	CreatorID    string
	ReviewerID   string
	SourceBranch string
	TargetBranch string
	Skip         int
	Top          int
}

// PullRequest is the Azure-native PR summary used by list and task-link flows.
type PullRequest struct {
	ID             int        `json:"id"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	Status         string     `json:"status"`
	IsDraft        bool       `json:"isDraft"`
	SourceBranch   string     `json:"sourceBranch"`
	TargetBranch   string     `json:"targetBranch"`
	MergeStatus    string     `json:"mergeStatus,omitempty"`
	CreationDate   *time.Time `json:"creationDate,omitempty"`
	ClosedDate     *time.Time `json:"closedDate,omitempty"`
	Author         Identity   `json:"author"`
	ProjectID      string     `json:"projectId"`
	ProjectName    string     `json:"projectName"`
	RepositoryID   string     `json:"repositoryId"`
	RepositoryName string     `json:"repositoryName"`
	WebURL         string     `json:"webUrl"`
	APIURL         string     `json:"apiUrl"`
}

type PullRequestPage struct {
	Items []PullRequest `json:"items"`
	Count int           `json:"count"`
	Skip  int           `json:"skip"`
	Top   int           `json:"top"`
}

type Reviewer struct {
	Identity
	Vote        int  `json:"vote"`
	IsRequired  bool `json:"isRequired"`
	HasDeclined bool `json:"hasDeclined"`
}

type Comment struct {
	ID          int       `json:"id"`
	Content     string    `json:"content"`
	Author      Identity  `json:"author"`
	CommentType string    `json:"commentType"`
	PublishedAt time.Time `json:"publishedAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

// UnmarshalJSON translates Azure's wire timestamp names while preserving
// Kandev's public publishedAt/updatedAt response contract.
func (c *Comment) UnmarshalJSON(data []byte) error {
	type commentAlias Comment
	var wire struct {
		commentAlias
		PublishedDate   time.Time `json:"publishedDate"`
		LastUpdatedDate time.Time `json:"lastUpdatedDate"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*c = Comment(wire.commentAlias)
	c.PublishedAt = wire.PublishedDate
	c.UpdatedAt = wire.LastUpdatedDate
	return nil
}

type Thread struct {
	ID       int       `json:"id"`
	Status   string    `json:"status"`
	Comments []Comment `json:"comments"`
}

type WorkItemRef struct {
	ID  int    `json:"id"`
	URL string `json:"url"`
}

type PolicyEvaluation struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Name       string `json:"name"`
	IsBlocking bool   `json:"isBlocking"`
}
