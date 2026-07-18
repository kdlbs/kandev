package azuredevops

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	restAPIVersion       = "7.1"
	previewAPIVersion    = "7.1-preview.1"
	maxErrorBodyBytes    = 4096
	maxResponseBodyBytes = 16 << 20
	workItemBatchSize    = 200
	defaultPRPageSize    = 50
)

// APIError is a bounded, credential-redacted Azure DevOps response error.
type APIError struct {
	StatusCode int
	Endpoint   string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("azure devops API %s returned %d: %s", e.Endpoint, e.StatusCode, e.Body)
}

func AsAPIError(err error, target **APIError) bool { return errors.As(err, target) }

// RESTClient reads Azure DevOps Services directly over HTTP.
type RESTClient struct {
	organizationURL string
	pat             string
	httpClient      *http.Client
}

func NewRESTClient(organizationURL, pat string, httpClient *http.Client) *RESTClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &RESTClient{
		organizationURL: strings.TrimRight(organizationURL, "/"),
		pat:             pat,
		httpClient:      httpClient,
	}
}

func (c *RESTClient) TestAuth(ctx context.Context) (*TestConnectionResult, error) {
	var raw struct {
		AuthenticatedUser struct {
			ID                  string `json:"id"`
			ProviderDisplayName string `json:"providerDisplayName"`
			Properties          map[string]struct {
				Value string `json:"$value"`
			} `json:"properties"`
		} `json:"authenticatedUser"`
	}
	endpoint := "/_apis/connectionData?connectOptions=1&lastChangeId=-1&lastChangeId64=-1&api-version=" + previewAPIVersion
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &raw); err != nil {
		return nil, err
	}
	user := raw.AuthenticatedUser
	return &TestConnectionResult{
		OK:          user.ID != "",
		ID:          user.ID,
		DisplayName: user.ProviderDisplayName,
		Email:       user.Properties["Account"].Value,
	}, nil
}

func (c *RESTClient) ListProjects(ctx context.Context) ([]Project, error) {
	var response struct {
		Value []Project `json:"value"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/_apis/projects?api-version="+restAPIVersion, nil, &response); err != nil {
		return nil, err
	}
	return response.Value, nil
}

func (c *RESTClient) ListRepositories(ctx context.Context, projectID string) ([]Repository, error) {
	var response struct {
		Value []rawRepository `json:"value"`
	}
	endpoint := fmt.Sprintf("/%s/_apis/git/repositories?api-version=%s", pathPart(projectID), restAPIVersion)
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	result := make([]Repository, 0, len(response.Value))
	for _, repository := range response.Value {
		result = append(result, convertRepository(repository))
	}
	return result, nil
}

func (c *RESTClient) QueryWIQL(ctx context.Context, projectID, wiql string, top int) (*WorkItemSearchResult, error) {
	if top <= 0 {
		top = workItemBatchSize
	}
	endpoint := fmt.Sprintf("/%s/_apis/wit/wiql?$top=%d&api-version=%s", pathPart(projectID), top, restAPIVersion)
	var response struct {
		WorkItems []struct {
			ID int `json:"id"`
		} `json:"workItems"`
	}
	if err := c.doJSON(ctx, http.MethodPost, endpoint, map[string]string{"query": wiql}, &response); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(response.WorkItems))
	for _, ref := range response.WorkItems {
		ids = append(ids, ref.ID)
	}
	items, err := c.hydrateWorkItems(ctx, projectID, ids)
	if err != nil {
		return nil, err
	}
	return &WorkItemSearchResult{Items: items, Count: len(items)}, nil
}

func (c *RESTClient) GetWorkItem(ctx context.Context, projectID string, id int) (*WorkItem, error) {
	endpoint := fmt.Sprintf("/%s/_apis/wit/workitems/%d?$expand=all&api-version=%s", pathPart(projectID), id, restAPIVersion)
	var raw rawWorkItem
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &raw); err != nil {
		return nil, err
	}
	item := convertWorkItem(raw)
	return &item, nil
}

func (c *RESTClient) ListPullRequests(ctx context.Context, filter PullRequestFilter) (*PullRequestPage, error) {
	values := url.Values{"api-version": {restAPIVersion}}
	setPRFilter(values, "searchCriteria.status", filter.Status)
	setPRFilter(values, "searchCriteria.creatorId", filter.CreatorID)
	setPRFilter(values, "searchCriteria.reviewerId", filter.ReviewerID)
	setPRFilter(values, "searchCriteria.sourceRefName", normalizeRefForAPI(filter.SourceBranch))
	setPRFilter(values, "searchCriteria.targetRefName", normalizeRefForAPI(filter.TargetBranch))
	if filter.Skip > 0 {
		values.Set("$skip", strconv.Itoa(filter.Skip))
	}
	top := filter.Top
	if top <= 0 || top > 100 {
		top = defaultPRPageSize
	}
	values.Set("$top", strconv.Itoa(top))
	endpoint := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests?%s",
		pathPart(filter.ProjectID), pathPart(filter.RepositoryID), values.Encode())
	var response struct {
		Count int              `json:"count"`
		Value []rawPullRequest `json:"value"`
	}
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	items := make([]PullRequest, 0, len(response.Value))
	for _, raw := range response.Value {
		items = append(items, convertPullRequest(raw))
	}
	return &PullRequestPage{Items: items, Count: response.Count, Skip: filter.Skip, Top: top}, nil
}

func (c *RESTClient) GetPullRequest(ctx context.Context, projectID, repositoryID string, id int) (*PullRequest, error) {
	endpoint := pullRequestEndpoint(projectID, repositoryID, id) + "?api-version=" + restAPIVersion
	var raw rawPullRequest
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &raw); err != nil {
		return nil, err
	}
	result := convertPullRequest(raw)
	return &result, nil
}

func (c *RESTClient) ListReviewers(ctx context.Context, projectID, repositoryID string, id int) ([]Reviewer, error) {
	var response struct {
		Value []Reviewer `json:"value"`
	}
	endpoint := pullRequestEndpoint(projectID, repositoryID, id) + "/reviewers?api-version=" + restAPIVersion
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	return response.Value, nil
}

func (c *RESTClient) ListThreads(ctx context.Context, projectID, repositoryID string, id int) ([]Thread, error) {
	var response struct {
		Value []Thread `json:"value"`
	}
	endpoint := pullRequestEndpoint(projectID, repositoryID, id) + "/threads?api-version=" + restAPIVersion
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	return response.Value, nil
}

func (c *RESTClient) ListLinkedWorkItems(ctx context.Context, projectID, repositoryID string, id int) ([]WorkItemRef, error) {
	var response struct {
		Value []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"value"`
	}
	endpoint := pullRequestEndpoint(projectID, repositoryID, id) + "/workitems?api-version=" + previewAPIVersion
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	refs := make([]WorkItemRef, 0, len(response.Value))
	for _, raw := range response.Value {
		workItemID, err := strconv.Atoi(raw.ID)
		if err == nil {
			refs = append(refs, WorkItemRef{ID: workItemID, URL: raw.URL})
		}
	}
	return refs, nil
}

func (c *RESTClient) ListPolicyEvaluations(ctx context.Context, projectID string, id int) ([]PolicyEvaluation, error) {
	artifact := fmt.Sprintf("vstfs:///CodeReview/CodeReviewId/%s/%d", projectID, id)
	values := url.Values{"artifactId": {artifact}, "api-version": {previewAPIVersion}}
	endpoint := fmt.Sprintf("/%s/_apis/policy/evaluations?%s", pathPart(projectID), values.Encode())
	var response struct {
		Value []rawPolicyEvaluation `json:"value"`
	}
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	items := make([]PolicyEvaluation, 0, len(response.Value))
	for _, raw := range response.Value {
		items = append(items, PolicyEvaluation{
			ID: raw.ID, Status: raw.Status, Name: raw.Configuration.Type.DisplayName,
			IsBlocking: raw.Configuration.IsBlocking,
		})
	}
	return items, nil
}

func (c *RESTClient) hydrateWorkItems(ctx context.Context, projectID string, ids []int) ([]WorkItem, error) {
	byID := make(map[int]WorkItem, len(ids))
	for start := 0; start < len(ids); start += workItemBatchSize {
		end := min(start+workItemBatchSize, len(ids))
		items, err := c.getWorkItemBatch(ctx, projectID, ids[start:end])
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			byID[item.ID] = item
		}
	}
	ordered := make([]WorkItem, 0, len(byID))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			ordered = append(ordered, item)
		}
	}
	return ordered, nil
}

func (c *RESTClient) getWorkItemBatch(ctx context.Context, projectID string, ids []int) ([]WorkItem, error) {
	endpoint := fmt.Sprintf("/%s/_apis/wit/workitemsbatch?api-version=%s", pathPart(projectID), restAPIVersion)
	body := map[string]any{
		"ids": ids, "$expand": "all", "errorPolicy": "omit",
	}
	var response struct {
		Value []rawWorkItem `json:"value"`
	}
	if err := c.doJSON(ctx, http.MethodPost, endpoint, body, &response); err != nil {
		return nil, err
	}
	items := make([]WorkItem, 0, len(response.Value))
	for _, raw := range response.Value {
		items = append(items, convertWorkItem(raw))
	}
	return items, nil
}

func (c *RESTClient) doJSON(ctx context.Context, method, endpoint string, requestBody, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encode azure devops request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.organizationURL+endpoint, body)
	if err != nil {
		return fmt.Errorf("create azure devops request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(":"+c.pat)))
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("azure devops request %s: %w", endpointPath(endpoint), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return c.decodeAPIError(resp, endpointPath(endpoint))
	}
	limited := io.LimitReader(resp.Body, maxResponseBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read azure devops response: %w", err)
	}
	if len(data) > maxResponseBodyBytes {
		return errors.New("azure devops response exceeded size limit")
	}
	if responseBody == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return fmt.Errorf("decode azure devops response: %w", err)
	}
	return nil
}

func (c *RESTClient) decodeAPIError(resp *http.Response, endpoint string) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	body := strings.ReplaceAll(string(data), c.pat, "[REDACTED]")
	return &APIError{StatusCode: resp.StatusCode, Endpoint: endpoint, Body: body}
}

type rawRepository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
	WebURL        string `json:"webUrl"`
	Project       struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
}

type rawWorkItem struct {
	ID     int            `json:"id"`
	Rev    int            `json:"rev"`
	URL    string         `json:"url"`
	Fields map[string]any `json:"fields"`
	Links  struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"_links"`
}

type rawPullRequest struct {
	PullRequestID int           `json:"pullRequestId"`
	Title         string        `json:"title"`
	Description   string        `json:"description"`
	Status        string        `json:"status"`
	IsDraft       bool          `json:"isDraft"`
	SourceRefName string        `json:"sourceRefName"`
	TargetRefName string        `json:"targetRefName"`
	MergeStatus   string        `json:"mergeStatus"`
	CreationDate  *time.Time    `json:"creationDate"`
	ClosedDate    *time.Time    `json:"closedDate"`
	URL           string        `json:"url"`
	CreatedBy     Identity      `json:"createdBy"`
	Repository    rawRepository `json:"repository"`
}

type rawPolicyEvaluation struct {
	ID            string `json:"evaluationId"`
	Status        string `json:"status"`
	Configuration struct {
		IsBlocking bool `json:"isBlocking"`
		Type       struct {
			DisplayName string `json:"displayName"`
		} `json:"type"`
	} `json:"configuration"`
}

func convertRepository(raw rawRepository) Repository {
	return Repository{
		ID: raw.ID, Name: raw.Name, ProjectID: raw.Project.ID, ProjectName: raw.Project.Name,
		DefaultBranch: trimBranchRef(raw.DefaultBranch), WebURL: raw.WebURL,
	}
}

func convertWorkItem(raw rawWorkItem) WorkItem {
	return WorkItem{
		ID: raw.ID, Revision: raw.Rev, Title: stringField(raw.Fields, "System.Title"),
		Description: stringField(raw.Fields, "System.Description"),
		State:       stringField(raw.Fields, "System.State"), Type: stringField(raw.Fields, "System.WorkItemType"),
		Project: stringField(raw.Fields, "System.TeamProject"), AreaPath: stringField(raw.Fields, "System.AreaPath"),
		AssignedTo: identityDisplayField(raw.Fields, "System.AssignedTo"), WebURL: raw.Links.HTML.Href,
		APIURL: raw.URL, Fields: raw.Fields,
	}
}

func convertPullRequest(raw rawPullRequest) PullRequest {
	webURL := ""
	if raw.Repository.WebURL != "" && raw.PullRequestID > 0 {
		webURL = strings.TrimRight(raw.Repository.WebURL, "/") + "/pullrequest/" + strconv.Itoa(raw.PullRequestID)
	}
	return PullRequest{
		ID: raw.PullRequestID, Title: raw.Title, Description: raw.Description, Status: raw.Status,
		IsDraft: raw.IsDraft, SourceBranch: trimBranchRef(raw.SourceRefName), TargetBranch: trimBranchRef(raw.TargetRefName),
		MergeStatus: raw.MergeStatus, CreationDate: raw.CreationDate, ClosedDate: raw.ClosedDate,
		Author: raw.CreatedBy, ProjectID: raw.Repository.Project.ID, ProjectName: raw.Repository.Project.Name,
		RepositoryID: raw.Repository.ID, RepositoryName: raw.Repository.Name, WebURL: webURL, APIURL: raw.URL,
	}
}

func stringField(fields map[string]any, key string) string {
	value, _ := fields[key].(string)
	return value
}

func identityDisplayField(fields map[string]any, key string) string {
	switch value := fields[key].(type) {
	case string:
		return value
	case map[string]any:
		display, _ := value["displayName"].(string)
		return display
	default:
		return ""
	}
}

func pullRequestEndpoint(projectID, repositoryID string, id int) string {
	return fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests/%d", pathPart(projectID), pathPart(repositoryID), id)
}

func pathPart(value string) string      { return url.PathEscape(strings.TrimSpace(value)) }
func trimBranchRef(value string) string { return strings.TrimPrefix(value, "refs/heads/") }
func normalizeRefForAPI(value string) string {
	if value == "" || strings.HasPrefix(value, "refs/") {
		return value
	}
	return "refs/heads/" + value
}
func setPRFilter(values url.Values, key, value string) {
	if value != "" {
		values.Set(key, value)
	}
}
func endpointPath(endpoint string) string {
	if index := strings.IndexByte(endpoint, '?'); index >= 0 {
		return endpoint[:index]
	}
	return endpoint
}
