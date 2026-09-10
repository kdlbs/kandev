package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	workflowConclusionActionRequired = "action_required"
	workflowEventPullRequest         = "pull_request"
	workflowStatusCompleted          = "completed"
)

type workflowRunKey struct {
	Owner   string
	Repo    string
	HeadSHA string
}

type workflowJobKey struct {
	Owner   string
	Repo    string
	RunID   int64
	Attempt int
}

// ghWorkflowRun and its nested values are shared by the gh CLI and PAT
// clients because both consume the same Actions REST response shape.
type ghWorkflowRun struct {
	ID             int64                     `json:"id"`
	RunAttempt     int                       `json:"run_attempt"`
	WorkflowID     int64                     `json:"workflow_id"`
	Name           string                    `json:"name"`
	Event          string                    `json:"event"`
	Status         string                    `json:"status"`
	Conclusion     *string                   `json:"conclusion"`
	HeadBranch     string                    `json:"head_branch"`
	HeadSHA        string                    `json:"head_sha"`
	HTMLURL        string                    `json:"html_url"`
	CreatedAt      time.Time                 `json:"created_at"`
	UpdatedAt      time.Time                 `json:"updated_at"`
	HeadRepository *ghWorkflowRepository     `json:"head_repository"`
	PullRequests   []ghWorkflowPRAssociation `json:"pull_requests"`
}

type ghWorkflowRepository struct {
	FullName string              `json:"full_name"`
	Name     string              `json:"name"`
	Owner    ghWorkflowRepoOwner `json:"owner"`
}

type ghWorkflowRepoOwner struct {
	Login string `json:"login"`
}

type ghWorkflowPRAssociation struct {
	Number int `json:"number"`
	Head   struct {
		Ref  string                `json:"ref"`
		SHA  string                `json:"sha"`
		Repo *ghWorkflowRepository `json:"repo"`
	} `json:"head"`
}

type ghWorkflowJob struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Conclusion *string `json:"conclusion"`
}

func convertRawWorkflowRun(raw ghWorkflowRun) WorkflowRun {
	run := WorkflowRun{
		ID:         raw.ID,
		RunAttempt: raw.RunAttempt,
		WorkflowID: raw.WorkflowID,
		Name:       raw.Name,
		Event:      raw.Event,
		Status:     raw.Status,
		HeadSHA:    raw.HeadSHA,
		HeadBranch: raw.HeadBranch,
		HTMLURL:    raw.HTMLURL,
		CreatedAt:  raw.CreatedAt,
		UpdatedAt:  raw.UpdatedAt,
	}
	if raw.Conclusion != nil {
		run.Conclusion = *raw.Conclusion
	}
	if raw.HeadRepository != nil {
		run.HeadRepoOwner = raw.HeadRepository.Owner.Login
		run.HeadRepoName = raw.HeadRepository.Name
		if run.HeadRepoName == "" && run.HeadRepoOwner != "" {
			parts := strings.SplitN(raw.HeadRepository.FullName, "/", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], run.HeadRepoOwner) {
				run.HeadRepoName = parts[1]
			}
		}
	}
	for _, association := range raw.PullRequests {
		converted := WorkflowRunPullRequest{
			Number:     association.Number,
			HeadSHA:    association.Head.SHA,
			HeadBranch: association.Head.Ref,
		}
		if association.Head.Repo != nil {
			converted.HeadRepoOwner = association.Head.Repo.Owner.Login
			converted.HeadRepoName = association.Head.Repo.Name
		}
		run.PullRequests = append(run.PullRequests, converted)
	}
	return run
}

func convertRawWorkflowJobs(raw []ghWorkflowJob) []WorkflowJob {
	jobs := make([]WorkflowJob, 0, len(raw))
	for _, job := range raw {
		converted := WorkflowJob{ID: job.ID, Name: job.Name, Status: job.Status}
		if job.Conclusion != nil {
			converted.Conclusion = *job.Conclusion
		}
		jobs = append(jobs, converted)
	}
	return jobs
}

func decodeGHWorkflowRuns(out string) ([]ghWorkflowRun, error) {
	return decodeJSONStream[ghWorkflowRun](out)
}

func decodeGHWorkflowJobs(out string) ([]ghWorkflowJob, error) {
	return decodeJSONStream[ghWorkflowJob](out)
}

func decodeJSONStream[T any](out string) ([]T, error) {
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(strings.NewReader(out))
	var values []T
	for {
		var value T
		if err := decoder.Decode(&value); err != nil {
			if err == io.EOF {
				return values, nil
			}
			return nil, err
		}
		values = append(values, value)
	}
}

// workflowAttentionUnknown creates an observation that cannot make a
// positive claim. Callers may replace it with a same-head stale observation
// when a later provider read is unavailable.
func workflowAttentionUnknown(headSHA string) *WorkflowAttention {
	return &WorkflowAttention{
		State:      WorkflowAttentionUnknown,
		HeadSHA:    headSHA,
		ObservedAt: time.Now().UTC(),
		Stale:      false,
		Runs:       []WorkflowAttentionRun{},
	}
}

// collectWorkflowAttention reads and classifies Actions evidence for one PR.
// A failed Actions read is returned alongside an unknown observation so
// callers can preserve an older same-head positive observation without
// treating a permission failure as authoritative absence.
func collectWorkflowAttention(
	ctx context.Context, client Client, owner, repo string, pr *PR,
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

	runs, err := client.ListWorkflowRuns(ctx, owner, repo, pr.HeadSHA)
	if err != nil {
		return workflowAttentionUnknown(pr.HeadSHA), err
	}
	return classifyWorkflowAttention(ctx, client, owner, repo, pr, runs), nil
}

func classifyWorkflowAttention(
	ctx context.Context, client Client, owner, repo string, pr *PR, runs []WorkflowRun,
) *WorkflowAttention {
	return classifyWorkflowAttentionWithJobs(ctx, owner, repo, pr, runs,
		func(ctx context.Context, runID int64, attempt int) ([]WorkflowJob, error) {
			return client.ListWorkflowRunJobs(ctx, owner, repo, runID, attempt)
		},
	)
}

type workflowAttentionJobReader func(context.Context, int64, int) ([]WorkflowJob, error)

func classifyWorkflowAttentionWithJobs(
	ctx context.Context, owner, repo string, pr *PR, runs []WorkflowRun, readJobs workflowAttentionJobReader,
) *WorkflowAttention {
	if pr == nil {
		return workflowAttentionUnknown("")
	}
	if isTerminalPR(pr) {
		return workflowAttentionNone(pr.HeadSHA)
	}
	if pr.HeadSHA == "" {
		return workflowAttentionUnknown("")
	}
	attention := &WorkflowAttention{
		State:      WorkflowAttentionNone,
		HeadSHA:    pr.HeadSHA,
		ObservedAt: time.Now().UTC(),
		Runs:       []WorkflowAttentionRun{},
	}

	selected := selectCurrentWorkflowRuns(runs, pr)
	for _, run := range selected {
		if !strings.EqualFold(run.Conclusion, workflowConclusionActionRequired) {
			continue
		}
		attempt := run.RunAttempt
		if attempt <= 0 {
			attempt = 1
		}
		jobs, jobsErr := readJobs(ctx, run.ID, attempt)
		reason := workflowAttentionActionRequiredReason
		if jobsErr == nil && isForkWorkflowRun(run, owner, repo) &&
			strings.EqualFold(run.Event, workflowEventPullRequest) &&
			strings.EqualFold(run.Status, workflowStatusCompleted) && len(jobs) == 0 {
			reason = workflowAttentionApprovalRequiredReason
		}
		attention.Runs = append(attention.Runs, WorkflowAttentionRun{
			RunID:      run.ID,
			RunAttempt: attempt,
			WorkflowID: run.WorkflowID,
			Name:       run.Name,
			URL:        run.HTMLURL,
			Reason:     reason,
		})
		if reason == workflowAttentionActionRequiredReason {
			attention.State = WorkflowAttentionActionRequired
		}
	}
	if attention.State != WorkflowAttentionActionRequired && len(attention.Runs) > 0 {
		attention.State = WorkflowAttentionApprovalRequired
	}
	sort.SliceStable(attention.Runs, func(i, j int) bool {
		return workflowAttentionRunLess(attention.Runs[i], attention.Runs[j])
	})
	return attention
}

const (
	workflowAttentionApprovalRequiredReason = "approval_required"
	workflowAttentionActionRequiredReason   = "action_required"
)

func workflowAttentionNone(headSHA string) *WorkflowAttention {
	return &WorkflowAttention{
		State:      WorkflowAttentionNone,
		HeadSHA:    headSHA,
		ObservedAt: time.Now().UTC(),
		Runs:       []WorkflowAttentionRun{},
	}
}

func isTerminalPR(pr *PR) bool {
	if pr == nil {
		return false
	}
	return strings.EqualFold(pr.State, "closed") || strings.EqualFold(pr.State, "merged") || pr.MergedAt != nil
}

func selectCurrentWorkflowRuns(runs []WorkflowRun, pr *PR) []WorkflowRun {
	latest := make(map[string]WorkflowRun)
	for _, run := range runs {
		if !workflowRunMatchesPR(run, pr) {
			continue
		}
		key := workflowRunGroupKey(run)
		current, ok := latest[key]
		if !ok || workflowRunNewer(run, current) {
			latest[key] = run
		}
	}
	out := make([]WorkflowRun, 0, len(latest))
	for _, run := range latest {
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool {
		return workflowRunLess(out[i], out[j])
	})
	return out
}

func workflowRunMatchesPR(run WorkflowRun, pr *PR) bool {
	if pr == nil || run.HeadSHA == "" || run.HeadSHA != pr.HeadSHA {
		return false
	}
	if len(run.PullRequests) > 0 {
		return workflowRunHasPRAssociation(run, pr)
	}

	// Fork pull_request runs can have an empty pull_requests association. The
	// source repository, branch, event, and SHA together are the conservative
	// fallback identity in that response shape.
	return workflowRunMatchesUnassociatedPR(run, pr)
}

func workflowRunHasPRAssociation(run WorkflowRun, pr *PR) bool {
	for _, association := range run.PullRequests {
		if workflowAssociationMatchesPR(association, run, pr) {
			return true
		}
	}
	return false
}

func workflowAssociationMatchesPR(association WorkflowRunPullRequest, run WorkflowRun, pr *PR) bool {
	if association.Number != pr.Number {
		return false
	}
	if association.HeadSHA != "" && association.HeadSHA != pr.HeadSHA {
		return false
	}
	if association.HeadBranch != "" && association.HeadBranch != pr.HeadBranch {
		return false
	}
	if workflowRepoIdentityMatches(association.HeadRepoOwner, association.HeadRepoName, pr.HeadRepoOwner, pr.HeadRepoName) {
		return true
	}
	return association.HeadRepoOwner == "" && association.HeadRepoName == "" &&
		run.HeadRepoOwner == "" && run.HeadRepoName == ""
}

func workflowRunMatchesUnassociatedPR(run WorkflowRun, pr *PR) bool {
	return strings.EqualFold(run.Event, workflowEventPullRequest) &&
		run.HeadBranch != "" && run.HeadBranch == pr.HeadBranch &&
		workflowRepoIdentityMatches(run.HeadRepoOwner, run.HeadRepoName, pr.HeadRepoOwner, pr.HeadRepoName)
}

func workflowRepoIdentityMatches(ownerA, repoA, ownerB, repoB string) bool {
	if ownerA == "" || repoA == "" || ownerB == "" || repoB == "" {
		return false
	}
	return strings.EqualFold(ownerA, ownerB) && strings.EqualFold(repoA, repoB)
}

func isForkWorkflowRun(run WorkflowRun, owner, repo string) bool {
	return !workflowRepoIdentityMatches(run.HeadRepoOwner, run.HeadRepoName, owner, repo) &&
		run.HeadRepoOwner != "" && run.HeadRepoName != ""
}

func workflowRunGroupKey(run WorkflowRun) string {
	workflow := fmt.Sprintf("id:%d", run.WorkflowID)
	if run.WorkflowID == 0 {
		workflow = "name:" + strings.ToLower(run.Name)
	}
	return fmt.Sprintf("%s|event:%s|branch:%s|source:%s/%s",
		workflow, strings.ToLower(run.Event), run.HeadBranch,
		strings.ToLower(run.HeadRepoOwner), strings.ToLower(run.HeadRepoName))
}

func workflowRunNewer(candidate, current WorkflowRun) bool {
	if !candidate.UpdatedAt.Equal(current.UpdatedAt) {
		return candidate.UpdatedAt.After(current.UpdatedAt)
	}
	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.After(current.CreatedAt)
	}
	if candidate.ID != current.ID {
		return candidate.ID > current.ID
	}
	return candidate.RunAttempt > current.RunAttempt
}

func workflowRunLess(a, b WorkflowRun) bool {
	if a.WorkflowID != b.WorkflowID {
		return a.WorkflowID < b.WorkflowID
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	if a.ID != b.ID {
		return a.ID < b.ID
	}
	return a.RunAttempt < b.RunAttempt
}

func workflowAttentionRunLess(a, b WorkflowAttentionRun) bool {
	if a.WorkflowID != b.WorkflowID {
		return a.WorkflowID < b.WorkflowID
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	if a.RunID != b.RunID {
		return a.RunID < b.RunID
	}
	return a.RunAttempt < b.RunAttempt
}

// HasActiveWorkflowAttention reports whether a stored observation blocks
// merge for the TaskPR's currently observed head. A stale positive remains a
// blocker until a fresh complete observation replaces it.
func HasActiveWorkflowAttention(pr *TaskPR) bool {
	if pr == nil || pr.WorkflowAttention == nil || pr.WorkflowAttention.HeadSHA == "" ||
		pr.WorkflowAttention.HeadSHA != pr.HeadSHA {
		return false
	}
	return pr.WorkflowAttention.State == WorkflowAttentionApprovalRequired ||
		pr.WorkflowAttention.State == WorkflowAttentionActionRequired
}

func cloneWorkflowAttention(attention *WorkflowAttention) *WorkflowAttention {
	if attention == nil {
		return nil
	}
	clone := *attention
	clone.Runs = append([]WorkflowAttentionRun{}, attention.Runs...)
	return &clone
}

func workflowAttentionSemanticEqual(left, right *WorkflowAttention) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.State != right.State || left.HeadSHA != right.HeadSHA || left.Stale != right.Stale || len(left.Runs) != len(right.Runs) {
		return false
	}
	for i := range left.Runs {
		if left.Runs[i] != right.Runs[i] {
			return false
		}
	}
	return true
}

// resolveTaskPRWorkflowAttention applies the observation-preservation rules
// at the storage boundary. A missing observation from a legacy or partial
// caller preserves a same-head value, an unavailable fresh read marks the
// same-head value stale, and a head change immediately discards old positive
// evidence.
func resolveTaskPRWorkflowAttention(previous *TaskPR, status *PRStatus, nextHeadSHA string) *WorkflowAttention {
	previousAttention := (*WorkflowAttention)(nil)
	previousSameHead := previous != nil && previous.HeadSHA != "" && previous.HeadSHA == nextHeadSHA &&
		(previous.WorkflowAttention == nil || previous.WorkflowAttention.HeadSHA == nextHeadSHA)
	if previousSameHead {
		previousAttention = previous.WorkflowAttention
	}

	if status == nil || !status.WorkflowAttentionPopulated {
		return cloneWorkflowAttention(previousAttention)
	}

	incoming := cloneWorkflowAttention(status.WorkflowAttention)
	if incoming == nil {
		incoming = workflowAttentionUnknown(nextHeadSHA)
	}
	if incoming.HeadSHA == "" {
		incoming.HeadSHA = nextHeadSHA
	}
	if incoming.HeadSHA != nextHeadSHA {
		return workflowAttentionUnknown(nextHeadSHA)
	}
	if incoming.State == WorkflowAttentionUnknown {
		if previousAttention != nil {
			preserved := cloneWorkflowAttention(previousAttention)
			preserved.Stale = true
			return preserved
		}
		incoming.Stale = true
		return incoming
	}
	incoming.Stale = false
	return incoming
}
