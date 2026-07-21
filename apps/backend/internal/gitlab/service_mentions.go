package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	defaultMentionLimit = 5
	maxMentionLimit     = 10
	mentionPageSize     = 10
	maxMentionPages     = 10
	maxMentionProjects  = 50
	mentionHTTPScheme   = "http"
	mentionHTTPSScheme  = "https"
)

var (
	ErrMentionWorkspaceRequired = errors.New("gitlab mention search: workspace ID required")
	ErrMentionQueryRequired     = errors.New("gitlab mention search: query required")
	ErrMentionUnsupportedScope  = errors.New("gitlab mention search: workspace scope unsupported")
	ErrMentionInvalidScope      = errors.New("gitlab mention search: invalid workspace scope")
	ErrMentionMalformedResponse = errors.New("gitlab mention search: malformed response")
)

// MentionItem preserves GitLab's immutable object ID and project-local display
// identity without exposing native query/filter details to the registry.
type MentionItem struct {
	ID          int64
	IID         int
	ProjectID   int64
	ProjectPath string
	Title       string
	URL         string
	Host        string
}

type mentionSearchRecord struct {
	id          int64
	iid         int
	projectID   int64
	projectPath string
	title       string
	url         string
}

type mentionSearchPage struct {
	records     []mentionSearchRecord
	resultCount int
	totalCount  int
}

type mentionSearchPageLoader func(context.Context, int, int) (mentionSearchPage, error)

var mentionProjectSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ConfigureMentionScopeForWorkspace explicitly binds the legacy global client
// to one workspace's host and project allowlist. No binding is inferred.
func (s *Service) ConfigureMentionScopeForWorkspace(
	ctx context.Context,
	workspaceID, host string,
	projects []MentionProjectScope,
) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return ErrMentionWorkspaceRequired
	}
	if strings.TrimSpace(host) == "" {
		return ErrMentionUnsupportedScope
	}
	normalizedHost, err := normalizeMentionHost(host)
	if err != nil {
		return err
	}
	serviceHost, err := normalizeMentionHost(s.Host())
	if err != nil || normalizedHost != serviceHost {
		return ErrMentionUnsupportedScope
	}
	normalizedProjects, err := normalizeMentionProjects(projects)
	if err != nil {
		return err
	}
	store := s.requireStore()
	if store == nil {
		return ErrMentionUnsupportedScope
	}
	return store.UpsertMentionScope(ctx, &MentionScope{
		WorkspaceID: workspaceID,
		Host:        normalizedHost,
		Projects:    normalizedProjects,
	})
}

// MentionScopeForWorkspace loads only an explicit binding and verifies it
// still matches the installed client's host.
func (s *Service) MentionScopeForWorkspace(ctx context.Context, workspaceID string) (*MentionScope, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, ErrMentionWorkspaceRequired
	}
	store := s.requireStore()
	if store == nil {
		return nil, ErrMentionUnsupportedScope
	}
	scope, err := store.GetMentionScope(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load GitLab mention scope: %w", err)
	}
	if scope == nil || scope.WorkspaceID != workspaceID {
		return nil, ErrMentionUnsupportedScope
	}
	normalizedHost, err := normalizeMentionHost(scope.Host)
	if err != nil {
		return nil, err
	}
	serviceHost, err := normalizeMentionHost(s.Host())
	if err != nil || normalizedHost != serviceHost {
		return nil, ErrMentionUnsupportedScope
	}
	projects, err := normalizeMentionProjects(scope.Projects)
	if err != nil {
		return nil, err
	}
	return &MentionScope{WorkspaceID: workspaceID, Host: normalizedHost, Projects: projects}, nil
}

// SearchMentionIssuesForWorkspace searches titles through a fixed query shape
// and filters every hit against the workspace project binding.
func (s *Service) SearchMentionIssuesForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]MentionItem, error) {
	scope, client, query, limit, err := s.prepareMentionSearch(ctx, workspaceID, query, limit)
	if err != nil {
		return nil, err
	}
	titleQuery := buildMentionTitleQuery(query)
	return collectMentionSearchPages(ctx, scope, limit, MentionIssueURL, func(
		ctx context.Context, pageNumber, pageSize int,
	) (mentionSearchPage, error) {
		page, err := client.ListIssuesPaged(ctx, "", titleQuery, pageNumber, pageSize)
		if err != nil {
			return mentionSearchPage{}, fmt.Errorf("search GitLab issue mentions: %w", err)
		}
		if page == nil {
			return mentionSearchPage{}, ErrMentionMalformedResponse
		}
		return mentionSearchPage{
			records: issueMentionSearchRecords(page.Issues), resultCount: len(page.Issues), totalCount: page.TotalCount,
		}, nil
	})
}

// SearchMentionMRsForWorkspace is the merge-request counterpart to issue
// mention search.
func (s *Service) SearchMentionMRsForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]MentionItem, error) {
	scope, client, query, limit, err := s.prepareMentionSearch(ctx, workspaceID, query, limit)
	if err != nil {
		return nil, err
	}
	titleQuery := buildMentionTitleQuery(query)
	return collectMentionSearchPages(ctx, scope, limit, MentionMRURL, func(
		ctx context.Context, pageNumber, pageSize int,
	) (mentionSearchPage, error) {
		page, err := client.SearchMRsPaged(ctx, "", titleQuery, pageNumber, pageSize)
		if err != nil {
			return mentionSearchPage{}, fmt.Errorf("search GitLab MR mentions: %w", err)
		}
		if page == nil {
			return mentionSearchPage{}, ErrMentionMalformedResponse
		}
		return mentionSearchPage{
			records: mrMentionSearchRecords(page.MRs), resultCount: len(page.MRs), totalCount: page.TotalCount,
		}, nil
	})
}

func collectMentionSearchPages(
	ctx context.Context,
	scope *MentionScope,
	limit int,
	canonicalURL func(string, string, int) string,
	load mentionSearchPageLoader,
) ([]MentionItem, error) {
	items := make([]MentionItem, 0, limit)
	for pageNumber := 1; pageNumber <= maxMentionPages; pageNumber++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := load(ctx, pageNumber, mentionPageSize)
		if err != nil {
			return nil, err
		}
		items = append(items, filterMentionSearchRecords(
			scope, page.records, limit-len(items), canonicalURL,
		)...)
		if len(items) == limit || !hasNextMentionPage(
			page.resultCount, page.totalCount, pageNumber, mentionPageSize,
		) {
			break
		}
	}
	return items, nil
}

func (s *Service) prepareMentionSearch(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) (*MentionScope, Client, string, int, error) {
	query, err := normalizeMentionQuery(query)
	if err != nil {
		return nil, nil, "", 0, err
	}
	scope, client, limit, err := s.mentionSearchBoundary(ctx, workspaceID, limit)
	return scope, client, query, limit, err
}

func issueMentionSearchRecords(issues []*Issue) []mentionSearchRecord {
	records := make([]mentionSearchRecord, 0, len(issues))
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		records = append(records, mentionSearchRecord{
			id: issue.ID, iid: issue.IID, projectID: issue.ProjectID,
			projectPath: issue.ProjectPath, title: issue.Title, url: firstMentionURL(issue.WebURL, issue.URL),
		})
	}
	return records
}

func mrMentionSearchRecords(mrs []*MR) []mentionSearchRecord {
	records := make([]mentionSearchRecord, 0, len(mrs))
	for _, mr := range mrs {
		if mr == nil {
			continue
		}
		records = append(records, mentionSearchRecord{
			id: mr.ID, iid: mr.IID, projectID: mr.ProjectID,
			projectPath: mr.ProjectPath, title: mr.Title, url: firstMentionURL(mr.WebURL, mr.URL),
		})
	}
	return records
}

func firstMentionURL(webURL, fallback string) string {
	if webURL != "" {
		return webURL
	}
	return fallback
}

func hasNextMentionPage(resultCount, totalCount, page, perPage int) bool {
	if resultCount < perPage {
		return false
	}
	return totalCount <= 0 || page*perPage < totalCount
}

func filterMentionSearchRecords(
	scope *MentionScope,
	records []mentionSearchRecord,
	limit int,
	canonicalURL func(string, string, int) string,
) []MentionItem {
	allowed := mentionProjectsByID(scope.Projects)
	items := make([]MentionItem, 0, min(len(records), limit))
	for _, record := range records {
		if len(items) == limit {
			break
		}
		if !allowedMentionProject(allowed, record.projectID, record.projectPath) ||
			record.id <= 0 || record.iid <= 0 || record.url != canonicalURL(scope.Host, record.projectPath, record.iid) {
			continue
		}
		items = append(items, MentionItem{
			ID: record.id, IID: record.iid, ProjectID: record.projectID,
			ProjectPath: record.projectPath, Title: record.title, URL: record.url, Host: scope.Host,
		})
	}
	return items
}

func (s *Service) mentionSearchBoundary(
	ctx context.Context,
	workspaceID string,
	limit int,
) (*MentionScope, Client, int, error) {
	scope, err := s.MentionScopeForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, 0, err
	}
	client := s.Client()
	if client == nil {
		return nil, nil, 0, ErrNoClient
	}
	clientHost, err := normalizeMentionHost(client.Host())
	if err != nil || clientHost != scope.Host {
		return nil, nil, 0, ErrMentionUnsupportedScope
	}
	return scope, client, normalizeMentionLimit(limit), nil
}

func normalizeMentionQuery(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" || !utf8.ValidString(query) || utf8.RuneCountInString(query) > 200 {
		return "", ErrMentionQueryRequired
	}
	return query, nil
}

func buildMentionTitleQuery(query string) string {
	values := url.Values{}
	values.Set("in", "title")
	values.Set("order_by", "updated_at")
	values.Set("scope", "all")
	values.Set("search", query)
	values.Set("sort", "desc")
	values.Set("state", gitlabStateOpened)
	return values.Encode()
}

func normalizeMentionLimit(limit int) int {
	if limit <= 0 {
		return defaultMentionLimit
	}
	if limit > maxMentionLimit {
		return maxMentionLimit
	}
	return limit
}

func normalizeMentionHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrMentionInvalidScope
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.Host == "" ||
		(parsed.Scheme != mentionHTTPScheme && parsed.Scheme != mentionHTTPSScheme) || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(raw, "|") {
		return "", ErrMentionInvalidScope
	}
	trimmedPath := strings.TrimRight(parsed.Path, "/")
	if trimmedPath == "." || (trimmedPath != "" && path.Clean(trimmedPath) != trimmedPath) {
		return "", ErrMentionInvalidScope
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = trimmedPath
	parsed.RawPath = ""
	return parsed.String(), nil
}

func normalizeMentionProjects(projects []MentionProjectScope) ([]MentionProjectScope, error) {
	if len(projects) == 0 || len(projects) > maxMentionProjects {
		return nil, ErrMentionInvalidScope
	}
	byID := make(map[int64]struct{}, len(projects))
	byPath := make(map[string]struct{}, len(projects))
	normalized := make([]MentionProjectScope, 0, len(projects))
	for _, project := range projects {
		project.Path = strings.TrimSpace(project.Path)
		if project.ID <= 0 || !validMentionProjectPath(project.Path) {
			return nil, ErrMentionInvalidScope
		}
		if _, exists := byID[project.ID]; exists {
			return nil, ErrMentionInvalidScope
		}
		if _, exists := byPath[project.Path]; exists {
			return nil, ErrMentionInvalidScope
		}
		byID[project.ID] = struct{}{}
		byPath[project.Path] = struct{}{}
		normalized = append(normalized, project)
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].ID == normalized[j].ID {
			return normalized[i].Path < normalized[j].Path
		}
		return normalized[i].ID < normalized[j].ID
	})
	return normalized, nil
}

func validMentionProjectPath(projectPath string) bool {
	if projectPath == "" || !utf8.ValidString(projectPath) || utf8.RuneCountInString(projectPath) > 300 {
		return false
	}
	for _, r := range projectPath {
		if unicode.IsControl(r) {
			return false
		}
	}
	segments := strings.Split(projectPath, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || !mentionProjectSegmentPattern.MatchString(segment) {
			return false
		}
	}
	return true
}

func mentionProjectsByID(projects []MentionProjectScope) map[int64]string {
	allowed := make(map[int64]string, len(projects))
	for _, project := range projects {
		allowed[project.ID] = project.Path
	}
	return allowed
}

func allowedMentionProject(allowed map[int64]string, projectID int64, projectPath string) bool {
	return allowed[projectID] == projectPath && projectPath != ""
}

// MentionProjectScopeKey combines the configured host and immutable project ID.
func MentionProjectScopeKey(host string, projectID int64) string {
	return host + "|project:" + strconv.FormatInt(projectID, 10)
}

// MentionIssueURL returns the one canonical web destination for an issue.
func MentionIssueURL(host, projectPath string, iid int) string {
	return mentionProjectURL(host, projectPath, "issues", iid)
}

// MentionMRURL returns the one canonical web destination for a merge request.
func MentionMRURL(host, projectPath string, iid int) string {
	return mentionProjectURL(host, projectPath, "merge_requests", iid)
}

func mentionProjectURL(host, projectPath, kind string, iid int) string {
	segments := strings.Split(projectPath, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return host + "/" + strings.Join(segments, "/") + "/-/" + kind + "/" + strconv.Itoa(iid)
}
