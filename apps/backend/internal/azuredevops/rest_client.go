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
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	restAPIVersion       = "7.1"
	previewAPIVersion    = "7.1-preview.1"
	maxErrorBodyBytes    = 4096
	maxResponseBodyBytes = 16 << 20
	workItemBatchSize    = 200
	branchResultLimit    = 1000
	defaultPRPageSize    = 50
	workItemCommentLimit = 100
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
	organization string
	pat          string
	httpClient   *http.Client
	initErr      error
}

func NewRESTClient(organizationURL, pat string, httpClient *http.Client) *RESTClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	validatedURL, err := ValidateOrganizationURL(organizationURL)
	if err != nil {
		return &RESTClient{
			pat: pat, httpClient: httpClient,
			initErr: fmt.Errorf("invalid azure devops organization URL: %w", err),
		}
	}
	organization := strings.TrimPrefix(validatedURL, "https://dev.azure.com/")
	return &RESTClient{
		organization: organization,
		pat:          pat,
		httpClient:   httpClient,
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

func (c *RESTClient) ListBranches(ctx context.Context, projectID, repositoryID string) ([]Branch, error) {
	var response struct {
		Value []struct {
			Name string `json:"name"`
		} `json:"value"`
	}
	endpoint := fmt.Sprintf("/%s/_apis/git/repositories/%s/refs?filter=heads/&$top=%d&api-version=%s", pathPart(projectID), pathPart(repositoryID), branchResultLimit, restAPIVersion)
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	branches := make([]Branch, 0, len(response.Value))
	for _, ref := range response.Value {
		branches = append(branches, Branch{Name: strings.TrimPrefix(ref.Name, "refs/heads/")})
	}
	return branches, nil
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

func (c *RESTClient) GetWorkItemDetail(ctx context.Context, projectID string, id int) (*WorkItemDetail, error) {
	item, err := c.GetWorkItem(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	detail := &WorkItemDetail{WorkItem: *item, PlanningFields: planningFields(item.Fields)}
	detail.Description = sanitizeDescriptionHTML(detail.Description)
	return detail, nil
}

func (c *RESTClient) ListWorkItemComments(ctx context.Context, projectID string, id int, continuationToken string) (*WorkItemCommentPage, error) {
	query := url.Values{}
	query.Set("$top", strconv.Itoa(workItemCommentLimit))
	query.Set("api-version", "7.1-preview.4")
	if strings.TrimSpace(continuationToken) != "" {
		query.Set("continuationToken", continuationToken)
	}
	endpoint := fmt.Sprintf("/%s/_apis/wit/workItems/%d/comments?%s", pathPart(projectID), id, query.Encode())
	var response struct {
		Comments []struct {
			ID           int        `json:"id"`
			Text         string     `json:"text"`
			IsDeleted    bool       `json:"isDeleted"`
			CreatedDate  *time.Time `json:"createdDate"`
			ModifiedDate *time.Time `json:"modifiedDate"`
			CreatedBy    Identity   `json:"createdBy"`
		} `json:"comments"`
	}
	var headers http.Header
	if err := c.doJSONWithResponseHeaders(ctx, http.MethodGet, endpoint, nil, &response, &headers); err != nil {
		return nil, err
	}
	comments := make([]WorkItemComment, 0, len(response.Comments))
	for _, comment := range response.Comments {
		if comment.IsDeleted {
			continue
		}
		comments = append(comments, WorkItemComment{
			ID: comment.ID, Content: comment.Text, Author: comment.CreatedBy,
			PublishedAt: comment.CreatedDate, UpdatedAt: comment.ModifiedDate,
		})
	}
	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].PublishedAt == nil {
			return false
		}
		if comments[j].PublishedAt == nil {
			return true
		}
		return comments[i].PublishedAt.After(*comments[j].PublishedAt)
	})
	return &WorkItemCommentPage{Comments: comments, ContinuationToken: headers.Get("x-ms-continuationtoken")}, nil
}

func (c *RESTClient) GetCurrentIdentity(ctx context.Context) (*Identity, error) {
	auth, err := c.TestAuth(ctx)
	if err != nil {
		return nil, err
	}
	return &Identity{ID: auth.ID, DisplayName: auth.DisplayName, UniqueName: auth.Email}, nil
}

func (c *RESTClient) ListTeams(ctx context.Context, projectID string) ([]Team, error) {
	var response struct {
		Value []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			ProjectID   string `json:"projectId"`
			ProjectName string `json:"projectName"`
		} `json:"value"`
	}
	endpoint := fmt.Sprintf("/_apis/projects/%s/teams?api-version=%s", pathPart(projectID), restAPIVersion)
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	teams := make([]Team, 0, len(response.Value))
	for _, raw := range response.Value {
		teams = append(teams, Team{ID: raw.ID, Name: raw.Name, ProjectID: raw.ProjectID, ProjectName: raw.ProjectName})
	}
	return teams, nil
}

func (c *RESTClient) ListBoards(ctx context.Context, projectID, teamID string) ([]BoardReference, error) {
	var response struct {
		Value []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			IsHidden bool   `json:"isHidden"`
		} `json:"value"`
	}
	endpoint := fmt.Sprintf("/%s/%s/_apis/work/backlogs?api-version=%s", pathPart(projectID), pathPart(teamID), restAPIVersion)
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	boards := make([]BoardReference, 0, len(response.Value))
	for _, raw := range response.Value {
		if raw.IsHidden {
			continue
		}
		boards = append(boards, BoardReference{ID: raw.ID, Name: raw.Name})
	}
	return boards, nil
}

func (c *RESTClient) GetBoardSnapshot(ctx context.Context, projectID, teamID, boardID string) (*BoardSnapshot, error) {
	var rawBoard struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Columns []struct {
			ID            string            `json:"id"`
			Name          string            `json:"name"`
			ColumnType    string            `json:"columnType"`
			Description   string            `json:"description"`
			IsSplit       bool              `json:"isSplit"`
			ItemLimit     int               `json:"itemLimit"`
			StateMappings map[string]string `json:"stateMappings"`
		} `json:"columns"`
		Fields struct {
			ColumnField FieldReference `json:"columnField"`
			DoneField   FieldReference `json:"doneField"`
			RowField    FieldReference `json:"rowField"`
		} `json:"fields"`
		Rows []BoardRow `json:"rows"`
	}
	boardEndpoint := fmt.Sprintf("/%s/%s/_apis/work/boards/%s?api-version=%s", pathPart(projectID), pathPart(teamID), pathPart(boardID), restAPIVersion)
	if err := c.doJSON(ctx, http.MethodGet, boardEndpoint, nil, &rawBoard); err != nil {
		return nil, err
	}
	board := Board{ID: rawBoard.ID, Name: rawBoard.Name, Fields: rawBoard.Fields, Rows: rawBoard.Rows, Columns: make([]BoardColumn, 0, len(rawBoard.Columns))}
	for _, column := range rawBoard.Columns {
		board.Columns = append(board.Columns, BoardColumn{ID: column.ID, Name: column.Name, ColumnType: column.ColumnType, Description: column.Description, IsSplit: column.IsSplit, ItemLimit: column.ItemLimit, StateMappings: column.StateMappings})
	}
	var refs struct {
		Value []struct {
			ID     int `json:"id"`
			Target struct {
				ID int `json:"id"`
			} `json:"target"`
		} `json:"value"`
		WorkItems []struct {
			ID     int `json:"id"`
			Target struct {
				ID int `json:"id"`
			} `json:"target"`
		} `json:"workItems"`
	}
	itemsEndpoint := fmt.Sprintf("/%s/%s/_apis/work/backlogs/%s/workItems?api-version=%s", pathPart(projectID), pathPart(teamID), pathPart(boardID), restAPIVersion)
	if err := c.doJSON(ctx, http.MethodGet, itemsEndpoint, nil, &refs); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(refs.Value))
	refsList := refs.Value
	if len(refsList) == 0 {
		refsList = refs.WorkItems
	}
	for _, ref := range refsList {
		id := ref.ID
		if id == 0 {
			id = ref.Target.ID
		}
		if id > 0 {
			ids = append(ids, id)
		}
	}
	items, err := c.hydrateWorkItems(ctx, projectID, ids)
	if err != nil {
		return nil, err
	}
	result := make([]BoardWorkItem, 0, len(items))
	for _, item := range items {
		columnID, done := boardItemPlacement(board, item)
		result = append(result, BoardWorkItem{WorkItem: item, ColumnID: columnID, ColumnDone: done})
	}
	return &BoardSnapshot{Board: board, Items: result}, nil
}

func (c *RESTClient) UpdateBoardWorkItem(ctx context.Context, projectID, teamID, boardID string, id int, request BoardWorkItemUpdateRequest) (*BoardWorkItem, error) {
	if request.Revision <= 0 {
		return nil, fmt.Errorf("%w: revision required", ErrInvalidConfig)
	}
	var board struct {
		Columns []BoardColumn `json:"columns"`
		Fields  BoardFields   `json:"fields"`
	}
	boardEndpoint := fmt.Sprintf("/%s/%s/_apis/work/boards/%s?api-version=%s", pathPart(projectID), pathPart(teamID), pathPart(boardID), restAPIVersion)
	if err := c.doJSON(ctx, http.MethodGet, boardEndpoint, nil, &board); err != nil {
		return nil, err
	}
	patch, err := boardWorkItemPatch(board, request)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("/%s/_apis/wit/workitems/%d?api-version=%s", pathPart(projectID), id, restAPIVersion)
	var raw rawWorkItem
	if err := c.doJSONWithContentType(ctx, http.MethodPatch, endpoint, patch, &raw, "application/json-patch+json"); err != nil {
		return nil, err
	}
	item := convertWorkItem(raw)
	columnID, done := boardItemPlacement(Board{Columns: board.Columns, Fields: board.Fields}, item)
	if columnID == "" && request.ColumnID != nil {
		columnID = *request.ColumnID
	}
	if request.ColumnDone != nil {
		done = *request.ColumnDone
	}
	return &BoardWorkItem{WorkItem: item, ColumnID: columnID, ColumnDone: done}, nil
}

func (c *RESTClient) UpdateWorkItem(ctx context.Context, projectID string, id int, request WorkItemAssignmentRequest) (*WorkItem, error) {
	patch, err := workItemAssignmentPatch(request)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("/%s/_apis/wit/workitems/%d?api-version=%s", pathPart(projectID), id, restAPIVersion)
	var raw rawWorkItem
	if err := c.doJSONWithContentType(ctx, http.MethodPatch, endpoint, patch, &raw, "application/json-patch+json"); err != nil {
		return nil, err
	}
	item := convertWorkItem(raw)
	return &item, nil
}

func workItemAssignmentPatch(request WorkItemAssignmentRequest) ([]map[string]any, error) {
	if err := validateWorkItemAssignment(request); err != nil {
		return nil, err
	}
	if !request.hasResolvedAssignee {
		return nil, fmt.Errorf("%w: assignee action must be resolved server-side", ErrInvalidConfig)
	}
	patch := []map[string]any{{"op": "test", "path": "/rev", "value": request.Revision}}
	if request.resolvedAssignee == "" {
		patch = append(patch, map[string]any{"op": "remove", "path": "/fields/System.AssignedTo"})
	} else {
		patch = append(patch, map[string]any{"op": "add", "path": "/fields/System.AssignedTo", "value": request.resolvedAssignee})
	}
	return patch, nil
}

func boardWorkItemPatch(board struct {
	Columns []BoardColumn `json:"columns"`
	Fields  BoardFields   `json:"fields"`
}, request BoardWorkItemUpdateRequest) ([]map[string]any, error) {
	if err := validateBoardWorkItemUpdate(request); err != nil {
		return nil, err
	}
	if request.AssigneeAction != nil && !request.hasResolvedAssignee {
		return nil, fmt.Errorf("%w: assignee action must be resolved server-side", ErrInvalidConfig)
	}
	patch := []map[string]any{{"op": "test", "path": "/rev", "value": request.Revision}}
	appendField := func(path string, value any) {
		patch = append(patch, map[string]any{"op": "replace", "path": path, "value": value})
	}
	appendOptionalField := func(path, value string) {
		if value == "" {
			patch = append(patch, map[string]any{"op": "remove", "path": path})
			return
		}
		patch = append(patch, map[string]any{"op": "add", "path": path, "value": value})
	}
	if request.hasResolvedAssignee {
		appendOptionalField("/fields/System.AssignedTo", request.resolvedAssignee)
	}
	if request.ColumnID != nil {
		columnName := ""
		for _, column := range board.Columns {
			if column.ID == *request.ColumnID {
				columnName = column.Name
				break
			}
		}
		if columnName == "" {
			return nil, fmt.Errorf("%w: unknown board column", ErrInvalidConfig)
		}
		if board.Fields.ColumnField.ReferenceName == "" {
			return nil, errors.New("azure devops: board column field is unavailable")
		}
		appendField("/fields/"+board.Fields.ColumnField.ReferenceName, columnName)
	}
	if request.ColumnDone != nil {
		if board.Fields.DoneField.ReferenceName == "" {
			return nil, errors.New("azure devops: board done field is unavailable")
		}
		appendField("/fields/"+board.Fields.DoneField.ReferenceName, *request.ColumnDone)
	}
	if len(patch) == 1 {
		return nil, fmt.Errorf("%w: at least one board field is required", ErrInvalidConfig)
	}
	return patch, nil
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
	endpoint := fmt.Sprintf("/%s/_apis/git/pullrequests?%s", pathPart(filter.ProjectID), values.Encode())
	if strings.TrimSpace(filter.RepositoryID) != "" {
		endpoint = fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests?%s",
			pathPart(filter.ProjectID), pathPart(filter.RepositoryID), values.Encode())
	}
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
	return c.doJSONWithContentType(ctx, method, endpoint, requestBody, responseBody, "application/json")
}

func (c *RESTClient) doJSONWithResponseHeaders(ctx context.Context, method, endpoint string, requestBody, responseBody any, responseHeaders *http.Header) error {
	return c.doJSONWithContentTypeAndResponseHeaders(ctx, method, endpoint, requestBody, responseBody, "application/json", responseHeaders)
}

func (c *RESTClient) doJSONWithContentType(ctx context.Context, method, endpoint string, requestBody, responseBody any, contentType string) error {
	return c.doJSONWithContentTypeAndResponseHeaders(ctx, method, endpoint, requestBody, responseBody, contentType, nil)
}

func (c *RESTClient) doJSONWithContentTypeAndResponseHeaders(ctx context.Context, method, endpoint string, requestBody, responseBody any, contentType string, responseHeaders *http.Header) error {
	if c.initErr != nil {
		return c.initErr
	}
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encode azure devops request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	requestURL := "https://dev.azure.com/" + url.PathEscape(c.organization) + endpoint
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return fmt.Errorf("create azure devops request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(":"+c.pat)))
	if requestBody != nil {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("azure devops request %s: %w", endpointPath(endpoint), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return c.decodeAPIError(resp, endpointPath(endpoint))
	}
	if responseHeaders != nil {
		*responseHeaders = resp.Header.Clone()
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
	tags := make([]string, 0)
	for _, tag := range strings.Split(stringField(raw.Fields, "System.Tags"), ";") {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	return WorkItem{
		ID: raw.ID, Revision: raw.Rev, Title: stringField(raw.Fields, "System.Title"),
		Description: stringField(raw.Fields, "System.Description"),
		State:       stringField(raw.Fields, "System.State"), Type: stringField(raw.Fields, "System.WorkItemType"),
		Project: stringField(raw.Fields, "System.TeamProject"), AreaPath: stringField(raw.Fields, "System.AreaPath"),
		AssignedTo: identityDisplayField(raw.Fields, "System.AssignedTo"), Tags: tags, WebURL: raw.Links.HTML.Href,
		APIURL: raw.URL, Fields: raw.Fields,
	}
}

var supportedPlanningFields = []struct {
	ReferenceName string
	Label         string
}{
	{ReferenceName: "Microsoft.VSTS.Scheduling.Effort", Label: "Effort"},
	{ReferenceName: "Microsoft.VSTS.Scheduling.StoryPoints", Label: "Story points"},
	{ReferenceName: "Microsoft.VSTS.Scheduling.Size", Label: "Size"},
	{ReferenceName: "Microsoft.VSTS.Scheduling.RemainingWork", Label: "Remaining work"},
	{ReferenceName: "Microsoft.VSTS.Scheduling.OriginalEstimate", Label: "Original estimate"},
	{ReferenceName: "Microsoft.VSTS.Scheduling.CompletedWork", Label: "Completed work"},
}

func planningFields(fields map[string]any) []PlanningField {
	result := make([]PlanningField, 0, len(supportedPlanningFields))
	for _, field := range supportedPlanningFields {
		value, ok := fields[field.ReferenceName]
		if !ok || value == nil {
			continue
		}
		formatted := strings.TrimSpace(fmt.Sprint(value))
		if formatted == "" {
			continue
		}
		result = append(result, PlanningField{ReferenceName: field.ReferenceName, Label: field.Label, Value: formatted})
	}
	return result
}

func sanitizeDescriptionHTML(value string) string {
	node, err := html.Parse(strings.NewReader(value))
	if err != nil {
		return ""
	}
	var text strings.Builder
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.ElementNode && (current.Data == "script" || current.Data == "style") {
			return
		}
		if current.Type == html.TextNode {
			text.WriteString(current.Data)
			text.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return strings.Join(strings.Fields(text.String()), " ")
}

func boardItemPlacement(board Board, item WorkItem) (string, bool) {
	columnValue := stringField(item.Fields, board.Fields.ColumnField.ReferenceName)
	columnID := ""
	for _, column := range board.Columns {
		if column.Name == columnValue {
			columnID = column.ID
			break
		}
	}
	done := false
	if raw, ok := item.Fields[board.Fields.DoneField.ReferenceName]; ok {
		switch value := raw.(type) {
		case bool:
			done = value
		case string:
			done = strings.EqualFold(value, "true")
		}
	}
	return columnID, done
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
