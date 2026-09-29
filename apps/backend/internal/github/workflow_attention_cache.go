package github

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	workflowAttentionCacheMaxEntries = 512
	workflowAttentionShortTTL        = 30 * time.Second
	workflowAttentionLongTTL         = 5 * time.Minute
)

func newWorkflowAttentionCache() *ttlCache {
	c := newTTLCache()
	c.ttl = workflowAttentionShortTTL
	c.maxSize = workflowAttentionCacheMaxEntries
	return c
}

func workflowRunsCacheKey(cacheScope, owner, repo, headSHA string) string {
	return scopedCacheKey(cacheScope, fmt.Sprintf(
		"workflow-runs:%s/%s@%s", strings.ToLower(owner), strings.ToLower(repo), strings.ToLower(headSHA),
	))
}

func workflowJobsCacheKey(cacheScope, owner, repo string, runID int64, attempt int) string {
	return scopedCacheKey(cacheScope, fmt.Sprintf(
		"workflow-jobs:%s/%s#%d/%d", strings.ToLower(owner), strings.ToLower(repo), runID, attempt,
	))
}

func workflowRunsCachePrefix(cacheScope, owner, repo string) string {
	return scopedCacheKey(cacheScope, fmt.Sprintf(
		"workflow-runs:%s/%s@", strings.ToLower(owner), strings.ToLower(repo),
	))
}

func workflowJobsCachePrefix(cacheScope, owner, repo string) string {
	return scopedCacheKey(cacheScope, fmt.Sprintf(
		"workflow-jobs:%s/%s#", strings.ToLower(owner), strings.ToLower(repo),
	))
}

func workflowRunsCacheTTL(value any) time.Duration {
	runs, ok := value.([]WorkflowRun)
	if !ok || len(runs) == 0 {
		return workflowAttentionShortTTL
	}
	for _, run := range runs {
		if !strings.EqualFold(run.Status, workflowStatusCompleted) ||
			run.Conclusion == "" || strings.EqualFold(run.Conclusion, workflowConclusionActionRequired) {
			return workflowAttentionShortTTL
		}
	}
	return workflowAttentionLongTTL
}

func workflowJobsCacheTTL(value any) time.Duration {
	jobs, ok := value.([]WorkflowJob)
	if !ok || len(jobs) == 0 {
		return workflowAttentionShortTTL
	}
	for _, job := range jobs {
		if !strings.EqualFold(job.Status, workflowStatusCompleted) || job.Conclusion == "" {
			return workflowAttentionShortTTL
		}
	}
	return workflowAttentionLongTTL
}

func (s *Service) cachedWorkflowRuns(
	ctx context.Context, client Client, cacheScope, owner, repo, headSHA string,
) ([]WorkflowRun, error) {
	if s == nil || s.workflowRunsCache == nil {
		return client.ListWorkflowRuns(ctx, owner, repo, headSHA)
	}
	key := workflowRunsCacheKey(cacheScope, owner, repo, headSHA)
	value, err := s.workflowRunsCache.doOrFetchWithTTL(key, func() (any, error) {
		return client.ListWorkflowRuns(ctx, owner, repo, headSHA)
	}, workflowRunsCacheTTL)
	if err != nil {
		return nil, err
	}
	runs, ok := value.([]WorkflowRun)
	if !ok {
		return nil, fmt.Errorf("invalid cached workflow runs value")
	}
	return runs, nil
}

func (s *Service) cachedWorkflowJobs(
	ctx context.Context,
	client Client,
	cacheScope, owner, repo string,
	runID int64, attempt int, shortLived bool,
) ([]WorkflowJob, error) {
	if s == nil || s.workflowJobsCache == nil {
		return client.ListWorkflowRunJobs(ctx, owner, repo, runID, attempt)
	}
	key := workflowJobsCacheKey(cacheScope, owner, repo, runID, attempt)
	ttlFor := workflowJobsCacheTTL
	if shortLived {
		ttlFor = func(any) time.Duration { return workflowAttentionShortTTL }
	}
	value, err := s.workflowJobsCache.doOrFetchWithTTL(key, func() (any, error) {
		return client.ListWorkflowRunJobs(ctx, owner, repo, runID, attempt)
	}, ttlFor)
	if err != nil {
		return nil, err
	}
	jobs, ok := value.([]WorkflowJob)
	if !ok {
		return nil, fmt.Errorf("invalid cached workflow jobs value")
	}
	return jobs, nil
}

func (s *Service) collectWorkflowAttention(
	ctx context.Context, client Client, cacheScope, owner, repo string, pr *PR,
) (*WorkflowAttention, error) {
	if pr == nil {
		return workflowAttentionUnknown(""), nil
	}
	if isTerminalPR(pr) {
		return workflowAttentionNone(pr.HeadSHA), nil
	}
	if pr.HeadSHA == "" {
		return workflowAttentionUnknown(""), nil
	}

	runs, err := s.cachedWorkflowRuns(ctx, client, cacheScope, owner, repo, pr.HeadSHA)
	if err != nil {
		return workflowAttentionUnknown(pr.HeadSHA), err
	}
	return classifyWorkflowAttentionWithJobs(ctx, owner, repo, pr, runs,
		func(jobCtx context.Context, runID int64, attempt int) ([]WorkflowJob, error) {
			return s.cachedWorkflowJobs(jobCtx, client, cacheScope, owner, repo, runID, attempt, true)
		},
	), nil
}

func (s *Service) invalidateWorkflowAttentionForPR(
	cacheScope, owner, repo string, number int, headSHA string,
) {
	if s == nil {
		return
	}
	key := scopedCacheKey(cacheScope, prStatusCacheKey(owner, repo, number))
	if number > 0 {
		if s.prStatusCache != nil {
			s.prStatusCache.invalidateKey(key)
		}
		if s.prFeedbackCache != nil {
			s.prFeedbackCache.invalidateKey(key)
		}
	}
	if s.workflowRunsCache != nil {
		if headSHA == "" {
			s.workflowRunsCache.invalidatePrefix(workflowRunsCachePrefix(cacheScope, owner, repo))
		} else {
			s.workflowRunsCache.invalidateKey(workflowRunsCacheKey(cacheScope, owner, repo, headSHA))
		}
	}
	if s.workflowJobsCache != nil {
		s.workflowJobsCache.invalidatePrefix(workflowJobsCachePrefix(cacheScope, owner, repo))
	}
}

// invalidateWorkflowAttentionForResolvedPR clears the automation namespace
// and the personal-read namespace for the same resolved credential. The
// status synchronizer uses the former, while the feedback endpoint uses the
// latter, so an explicit refresh must invalidate both views together.
func (s *Service) invalidateWorkflowAttentionForResolvedPR(
	resolved *resolvedServiceClient, owner, repo string, number int, headSHA string,
) {
	if resolved == nil {
		return
	}
	s.invalidateWorkflowAttentionForPR(resolved.CacheScope, owner, repo, number, headSHA)
	personalScope := resolved.cacheScopeForPurpose(CredentialPurposePersonalRead)
	if personalScope != "" && personalScope != resolved.CacheScope {
		s.invalidateWorkflowAttentionForPR(personalScope, owner, repo, number, headSHA)
	}
}

func (s *Service) invalidateWorkflowAttentionForResolvedWatches(
	resolved *resolvedServiceClient, watches []*PRWatch,
) {
	if resolved == nil {
		return
	}
	for _, watch := range watches {
		if watch == nil {
			continue
		}
		s.invalidateWorkflowAttentionForResolvedPR(
			resolved, watch.Owner, watch.Repo, watch.PRNumber, "",
		)
	}
}
