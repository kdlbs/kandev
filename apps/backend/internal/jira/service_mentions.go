package jira

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	defaultMentionSearchLimit = 5
	maxMentionSearchLimit     = 10
)

var (
	ErrMentionWorkspaceRequired = errors.New("jira: mention workspace ID required")
	ErrMentionInvalidQuery      = errors.New("jira: invalid mention query")
	ErrMentionInvalidSiteURL    = errors.New("jira: invalid mention site URL")
	ErrMentionMalformedResponse = errors.New("jira: malformed mention search response")
)

// MentionTicket is Jira's provider-local projection for normalized references.
type MentionTicket struct {
	ID      string
	Key     string
	Title   string
	URL     string
	SiteURL string
}

var mentionIssueKeyPattern = regexp.MustCompile(`(?i)^[a-z][a-z0-9]+-[0-9]+$`)

// buildMentionJQL translates plain user text into one fixed Jira query shape.
func buildMentionJQL(rawQuery string) (string, error) {
	query := strings.TrimSpace(rawQuery)
	if query == "" || !utf8.ValidString(query) || utf8.RuneCountInString(query) > 200 {
		return "", ErrMentionInvalidQuery
	}
	if mentionIssueKeyPattern.MatchString(query) {
		return "key = " + quoteJQLString(strings.ToUpper(query)) + " ORDER BY updated DESC", nil
	}
	return "summary ~ " + quoteJQLString(query) + " ORDER BY updated DESC", nil
}

func quoteJQLString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

// MentionSiteURLForWorkspace returns only the explicitly configured Jira site.
func (s *Service) MentionSiteURLForWorkspace(ctx context.Context, workspaceID string) (string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return "", ErrMentionWorkspaceRequired
	}
	if s == nil || s.store == nil {
		return "", ErrNotConfigured
	}
	cfg, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("read Jira mention config: %w", err)
	}
	if cfg == nil {
		return "", ErrNotConfigured
	}
	siteURL, err := normalizeMentionSiteURL(cfg.SiteURL)
	if err != nil {
		return "", err
	}
	return siteURL, nil
}

func normalizeMentionSiteURL(raw string) (string, error) {
	siteURL := normalizeSiteURL(raw)
	parsed, err := url.Parse(siteURL)
	if err != nil || parsed.User != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", ErrMentionInvalidSiteURL
	}
	return strings.TrimRight(siteURL, "/"), nil
}

// SearchMentionTicketsForWorkspace accepts only plain text and always starts at
// the first page with a small per-source cap.
func (s *Service) SearchMentionTicketsForWorkspace(
	ctx context.Context,
	workspaceID, query string,
	limit int,
) ([]MentionTicket, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, ErrMentionWorkspaceRequired
	}
	jql, err := buildMentionJQL(query)
	if err != nil {
		return nil, err
	}
	limit = normalizeMentionSearchLimit(limit)
	siteURL, err := s.MentionSiteURLForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	result, err := s.SearchTicketsForWorkspace(ctx, workspaceID, jql, "", limit)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, ErrMentionMalformedResponse
	}
	count := min(len(result.Tickets), limit)
	tickets := make([]MentionTicket, 0, count)
	for i := 0; i < count; i++ {
		ticket := result.Tickets[i]
		tickets = append(tickets, MentionTicket{
			ID:      ticket.ID,
			Key:     ticket.Key,
			Title:   ticket.Summary,
			URL:     ticket.URL,
			SiteURL: siteURL,
		})
	}
	return tickets, nil
}

func normalizeMentionSearchLimit(limit int) int {
	switch {
	case limit == 0:
		return defaultMentionSearchLimit
	case limit < 1:
		return 1
	case limit > maxMentionSearchLimit:
		return maxMentionSearchLimit
	default:
		return limit
	}
}
