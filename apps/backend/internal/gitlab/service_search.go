package gitlab

import "context"

// SearchUserMRs returns MRs matching a filter for the configured user.
func (s *Service) SearchUserMRs(ctx context.Context, filter, customQuery string) ([]*MR, error) {
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	return client.SearchMRs(ctx, filter, customQuery)
}

// SearchUserMRsPaged returns paginated MRs.
func (s *Service) SearchUserMRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*MRSearchPage, error) {
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	return client.SearchMRsPaged(ctx, filter, customQuery, page, perPage)
}

// SearchUserIssues returns issues matching a filter for the configured user.
func (s *Service) SearchUserIssues(ctx context.Context, filter, customQuery string) ([]*Issue, error) {
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	return client.ListIssues(ctx, filter, customQuery)
}

// SearchUserIssuesPaged returns paginated issues.
func (s *Service) SearchUserIssuesPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error) {
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	return client.ListIssuesPaged(ctx, filter, customQuery, page, perPage)
}

// GetStats aggregates open MR / awaiting-review / open-issue counts.
func (s *Service) GetStats(ctx context.Context) (*Stats, error) {
	client := s.Client()
	if client == nil {
		return &Stats{}, nil
	}
	username, err := client.GetAuthenticatedUser(ctx)
	if err != nil || username == "" {
		return &Stats{}, nil
	}
	open, err := client.SearchMRsPaged(ctx, "scope=created_by_me&state=opened", "", 1, 1)
	openMRs := 0
	if err == nil && open != nil {
		openMRs = open.TotalCount
	}
	awaiting, err := client.SearchMRsPaged(ctx, "reviewer_username="+username+"&state=opened", "", 1, 1)
	awaitingMRs := 0
	if err == nil && awaiting != nil {
		awaitingMRs = awaiting.TotalCount
	}
	issues, err := client.ListIssuesPaged(ctx, "scope=assigned_to_me&state=opened", "", 1, 1)
	openIssues := 0
	if err == nil && issues != nil {
		openIssues = issues.TotalCount
	}
	return &Stats{
		OpenMRs:              openMRs,
		MRsAwaitingMyReview:  awaitingMRs,
		OpenIssuesAssignedMe: openIssues,
	}, nil
}
