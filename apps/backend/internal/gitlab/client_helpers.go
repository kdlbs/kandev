package gitlab

import (
	"net/url"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/unidiff"
)

// rawMR is the JSON shape of a GitLab merge request as returned by the
// REST v4 API.
type rawMR struct {
	ID                          int64  `json:"id"`
	IID                         int    `json:"iid"`
	ProjectID                   int64  `json:"project_id"`
	Title                       string `json:"title"`
	Description                 string `json:"description"`
	State                       string `json:"state"` // opened, closed, merged, locked
	WebURL                      string `json:"web_url"`
	Draft                       bool   `json:"draft"`
	WorkInProgress              bool   `json:"work_in_progress"`
	MergeStatus                 string `json:"merge_status"`
	DetailedMergeStatus         string `json:"detailed_merge_status"`
	BlockingDiscussionsResolved bool   `json:"blocking_discussions_resolved"`
	HasConflicts                bool   `json:"has_conflicts"`
	SourceBranch                string `json:"source_branch"`
	TargetBranch                string `json:"target_branch"`
	SHA                         string `json:"sha"`
	References                  struct {
		Full string `json:"full"`
	} `json:"references"`
	Author             rawUser    `json:"author"`
	Reviewers          []rawUser  `json:"reviewers"`
	Assignees          []rawUser  `json:"assignees"`
	Labels             []string   `json:"labels"`
	ChangesCount       string     `json:"changes_count"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	MergedAt           *time.Time `json:"merged_at"`
	ClosedAt           *time.Time `json:"closed_at"`
	AllowCollaboration bool       `json:"allow_collaboration"`
	SourceProjectID    int64      `json:"source_project_id"`
	TargetProjectID    int64      `json:"target_project_id"`
	SourceProject      rawProject `json:"source_project"`
	TargetProject      rawProject `json:"target_project"`
}

type rawUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Bot       bool   `json:"bot"`
}

type rawProject struct {
	HTTPURLToRepo     string `json:"http_url_to_repo"`
	SSHURLToRepo      string `json:"ssh_url_to_repo"`
	ID                int64  `json:"id"`
	Path              string `json:"path"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"web_url"`
	DefaultBranch     string `json:"default_branch"`
	Namespace         struct {
		FullPath string `json:"full_path"`
	} `json:"namespace"`
}

type rawIssue struct {
	ID          int64     `json:"id"`
	IID         int       `json:"iid"`
	ProjectID   int64     `json:"project_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	State       string    `json:"state"`
	WebURL      string    `json:"web_url"`
	Author      rawUser   `json:"author"`
	Labels      []string  `json:"labels"`
	Assignees   []rawUser `json:"assignees"`
	References  struct {
		Full string `json:"full"`
	} `json:"references"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

type rawDiscussion struct {
	ID             string    `json:"id"`
	IndividualNote bool      `json:"individual_note"`
	Notes          []rawNote `json:"notes"`
}

type rawNote struct {
	ID         int64     `json:"id"`
	Body       string    `json:"body"`
	Type       string    `json:"type"`
	System     bool      `json:"system"`
	Resolvable bool      `json:"resolvable"`
	Resolved   bool      `json:"resolved"`
	Author     rawUser   `json:"author"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Position   *struct {
		NewPath string `json:"new_path"`
		OldPath string `json:"old_path"`
		NewLine int    `json:"new_line"`
		OldLine int    `json:"old_line"`
	} `json:"position"`
}

type rawPipeline struct {
	ID         int64      `json:"id"`
	IID        int        `json:"iid"`
	Status     string     `json:"status"`
	Source     string     `json:"source"`
	Ref        string     `json:"ref"`
	SHA        string     `json:"sha"`
	WebURL     string     `json:"web_url"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// rawPipelineJob is the JSON shape of a single entry from
// GET /projects/:id/pipelines/:id/jobs.
type rawPipelineJob struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Stage        string `json:"stage"`
	Status       string `json:"status"`
	AllowFailure bool   `json:"allow_failure"`
	WebURL       string `json:"web_url"`
}

func convertRawMR(raw *rawMR) *MR {
	state := normalizeMRState(raw.State)
	namespace, projectPath := splitFullReference(raw.References.Full)
	targetProjectID := raw.TargetProjectID
	if targetProjectID == 0 {
		targetProjectID = raw.ProjectID
	}
	mr := &MR{
		ID:                          raw.ID,
		IID:                         raw.IID,
		ProjectID:                   raw.ProjectID,
		Title:                       raw.Title,
		URL:                         raw.WebURL,
		WebURL:                      raw.WebURL,
		State:                       state,
		HeadBranch:                  raw.SourceBranch,
		HeadSHA:                     raw.SHA,
		BaseBranch:                  raw.TargetBranch,
		AuthorUsername:              raw.Author.Username,
		ProjectNamespace:            namespace,
		ProjectPath:                 projectPath,
		SourceProjectID:             raw.SourceProjectID,
		TargetProjectID:             targetProjectID,
		AllowCollaboration:          raw.AllowCollaboration,
		Body:                        raw.Description,
		Draft:                       raw.Draft || raw.WorkInProgress,
		MergeStatus:                 raw.MergeStatus,
		DetailedMergeStatus:         raw.DetailedMergeStatus,
		BlockingDiscussionsResolved: raw.BlockingDiscussionsResolved,
		HasConflicts:                raw.HasConflicts,
		Reviewers:                   convertReviewers(raw.Reviewers),
		Assignees:                   convertReviewers(raw.Assignees),
		Labels:                      append([]string(nil), raw.Labels...),
		CreatedAt:                   raw.CreatedAt,
		UpdatedAt:                   raw.UpdatedAt,
		MergedAt:                    raw.MergedAt,
		ClosedAt:                    raw.ClosedAt,
	}
	if raw.SourceProject.PathWithNamespace != "" {
		mr.SourceProjectPath = raw.SourceProject.PathWithNamespace
	}
	// A fork MR can contain only source_project_id and target_project_id. Do
	// not turn that response into a same-project MR by copying the target path;
	// PATClient hydrates the source project by ID before this conversion.
	if mr.SourceProjectPath == "" && (raw.SourceProjectID == 0 ||
		(targetProjectID > 0 && raw.SourceProjectID == targetProjectID)) {
		mr.SourceProjectPath = projectPath
	}
	mr.SourceProjectRemoteURL = raw.SourceProject.HTTPURLToRepo
	if raw.TargetProject.PathWithNamespace != "" {
		mr.TargetProjectPath = raw.TargetProject.PathWithNamespace
	}
	if mr.TargetProjectPath == "" {
		mr.TargetProjectPath = projectPath
	}
	mr.TargetDefaultBranch = raw.TargetProject.DefaultBranch
	return mr
}

func convertRawMRSlice(raw []rawMR) []*MR {
	out := make([]*MR, len(raw))
	for i := range raw {
		out[i] = convertRawMR(&raw[i])
	}
	return out
}

func convertReviewers(raw []rawUser) []MRReviewer {
	out := make([]MRReviewer, 0, len(raw))
	for _, r := range raw {
		if r.Username == "" {
			continue
		}
		out = append(out, MRReviewer{
			ID:       r.ID,
			Username: r.Username,
			Name:     r.Name,
			Type:     "user",
		})
	}
	return out
}

func convertRawIssue(raw *rawIssue) *Issue {
	namespace, projectPath := splitFullReference(raw.References.Full)
	assignees := make([]string, 0, len(raw.Assignees))
	for _, a := range raw.Assignees {
		if a.Username != "" {
			assignees = append(assignees, a.Username)
		}
	}
	return &Issue{
		ID:               raw.ID,
		IID:              raw.IID,
		ProjectID:        raw.ProjectID,
		Title:            raw.Title,
		Body:             raw.Description,
		URL:              raw.WebURL,
		WebURL:           raw.WebURL,
		State:            raw.State,
		AuthorUsername:   raw.Author.Username,
		ProjectNamespace: namespace,
		ProjectPath:      projectPath,
		Labels:           append([]string(nil), raw.Labels...),
		Assignees:        assignees,
		CreatedAt:        raw.CreatedAt,
		UpdatedAt:        raw.UpdatedAt,
		ClosedAt:         raw.ClosedAt,
	}
}

func convertRawProject(raw *rawProject) Project {
	namespace := raw.Namespace.FullPath
	if namespace == "" {
		// GitLab subgroups can be arbitrarily nested (acme/team/squad/repo),
		// so the namespace is everything up to the final "/" — not just the
		// first segment.
		if idx := strings.LastIndex(raw.PathWithNamespace, "/"); idx > 0 {
			namespace = raw.PathWithNamespace[:idx]
		}
	}
	return Project{
		ID:                raw.ID,
		PathWithNamespace: raw.PathWithNamespace,
		Namespace:         namespace,
		Path:              raw.Path,
		Name:              raw.Name,
		Visibility:        raw.Visibility,
		WebURL:            raw.WebURL,
		DefaultBranch:     raw.DefaultBranch,
	}
}

func convertRawDiscussion(raw *rawDiscussion) MRDiscussion {
	d := MRDiscussion{
		ID:    raw.ID,
		Notes: make([]MRNote, 0, len(raw.Notes)),
	}
	for i := range raw.Notes {
		note := convertRawNote(&raw.Notes[i])
		d.Notes = append(d.Notes, note)
		if i == 0 {
			d.Resolvable = raw.Notes[i].Resolvable
			d.Resolved = raw.Notes[i].Resolved
			d.CreatedAt = raw.Notes[i].CreatedAt
			d.UpdatedAt = raw.Notes[i].UpdatedAt
			if raw.Notes[i].Position != nil {
				d.Path = raw.Notes[i].Position.NewPath
				d.Line = raw.Notes[i].Position.NewLine
				d.OldLine = raw.Notes[i].Position.OldLine
			}
		} else if note.UpdatedAt.After(d.UpdatedAt) {
			d.UpdatedAt = note.UpdatedAt
		}
	}
	return d
}

func convertRawNote(raw *rawNote) MRNote {
	return MRNote{
		ID:           raw.ID,
		Author:       raw.Author.Username,
		AuthorAvatar: raw.Author.AvatarURL,
		AuthorIsBot:  raw.Author.Bot,
		Body:         raw.Body,
		Type:         raw.Type,
		System:       raw.System,
		CreatedAt:    raw.CreatedAt,
		UpdatedAt:    raw.UpdatedAt,
	}
}

func convertRawPipeline(raw *rawPipeline) Pipeline {
	return Pipeline{
		ID:         raw.ID,
		IID:        raw.IID,
		Status:     raw.Status,
		Source:     raw.Source,
		Ref:        raw.Ref,
		SHA:        raw.SHA,
		WebURL:     raw.WebURL,
		StartedAt:  raw.StartedAt,
		FinishedAt: raw.FinishedAt,
	}
}

func convertRawPipelineJob(raw *rawPipelineJob) PipelineJob {
	return PipelineJob{
		ID:           raw.ID,
		Name:         raw.Name,
		Stage:        raw.Stage,
		Status:       raw.Status,
		AllowFailure: raw.AllowFailure,
		WebURL:       raw.WebURL,
	}
}

// mrStateOpen is the normalized "open" state value shared with the GitHub
// integration vocabulary. GitLab's API returns "opened"; we expose "open".
const mrStateOpen = "open"

// normalizeMRState converts GitLab's "opened" to "open" and leaves the rest
// alone so the UI shares the GitHub vocabulary.
func normalizeMRState(state string) string {
	switch state {
	case gitlabStateOpened:
		return mrStateOpen
	case gitlabStateMerged, gitlabStateClosed, gitlabStateLocked:
		return state
	default:
		return state
	}
}

// splitFullReference parses GitLab's "namespace/path!iid" or
// "namespace/path#iid" form into (namespace, projectPath). It is best-effort:
// when the reference does not match it returns ("", "").
// splitFullReference parses GitLab's full-reference strings (e.g.
// "group/sub/project!42" for an MR or "group/project#10" for an issue) into
// (namespace, projectPath). projectPath is the *full* path-with-namespace
// — "group/sub/project" — so callers can round-trip it back into API URLs
// via projectRef without having to recombine namespace + name themselves.
// namespace is everything before the final "/".
func splitFullReference(full string) (namespace, projectPath string) {
	for _, sep := range []string{"!", "#"} {
		if idx := strings.Index(full, sep); idx > 0 {
			full = full[:idx]
			break
		}
	}
	last := strings.LastIndex(full, "/")
	if last <= 0 {
		return "", ""
	}
	return full[:last], full
}

func hasOpenDiscussions(discussions []MRDiscussion) bool {
	return countUnresolvedDiscussions(discussions) > 0
}

// countUnresolvedDiscussions counts discussions that are resolvable but not
// yet resolved — the same predicate hasOpenDiscussions checks, exposed as a
// count for the automation snapshot and summary UI.
func countUnresolvedDiscussions(discussions []MRDiscussion) int {
	count := 0
	for _, d := range discussions {
		if d.Resolvable && !d.Resolved {
			count++
		}
	}
	return count
}

func pipelineFailing(pipelines []Pipeline) bool {
	state, _, _ := summarizePipelines(pipelines)
	return state == pipelineStateFailure
}

// Computed status strings shared by pipeline + approval summarizers.
const statusPending = "pending"

// summarizePipelines reduces a list of pipeline runs (most-recent-first per
// the GitLab API) to a single state plus job counts. Only the most recent
// pipeline matters for the rolled-up status.
func summarizePipelines(pipelines []Pipeline) (state string, jobsTotal, jobsPassing int) {
	if len(pipelines) == 0 {
		return "", 0, 0
	}
	latest := pipelines[0]
	jobsTotal = latest.JobsTotal
	jobsPassing = latest.JobsPassing
	switch latest.Status {
	case pipelineStatusSuccess:
		state = pipelineStatusSuccess
	case pipelineStatusFailed, "canceled":
		state = pipelineStateFailure
	case "skipped":
		state = ""
	default:
		state = statusPending
	}
	return state, jobsTotal, jobsPassing
}

// countUnapprovedReviewers counts assigned reviewers who have not yet
// recorded an approval (Q2's "awaiting review" signal). GitLab has no
// distinct "requested but hasn't looked" state, so this only distinguishes
// "approved" from "everyone else assigned".
func countUnapprovedReviewers(reviewers []MRReviewer, approvals []MRApproval) int {
	approved := make(map[string]bool, len(approvals))
	for _, a := range approvals {
		approved[a.Username] = true
	}
	count := 0
	for _, r := range reviewers {
		if !approved[r.Username] {
			count++
		}
	}
	return count
}

func summarizeApprovals(have, required int) string {
	if required == 0 {
		if have > 0 {
			return approvalStateApproved
		}
		return ""
	}
	if have >= required {
		return approvalStateApproved
	}
	return statusPending
}

// --- Search query builders ---

// buildReviewMRQuery builds a query string for "MRs needing my review".
// GitLab's /merge_requests endpoint scopes to the authenticated user when
// `scope=assigned_to_me` or `reviewer_username=<me>`; we pass
// `reviewer_username=` resolution to the caller via filter (e.g.
// "reviewer_username=octocat"). state defaults to GitLab's opened state.
func buildReviewMRQuery(filter, customQuery string) string {
	if customQuery != "" {
		return customQuery
	}
	values := url.Values{}
	values.Set("state", gitlabStateOpened)
	values.Set("scope", "all")
	values.Set("per_page", "50")
	if filter != "" {
		appendFilter(values, filter)
	}
	return values.Encode()
}

func buildMRSearchQuery(filter, customQuery string) string {
	if customQuery != "" {
		return customQuery
	}
	values := url.Values{}
	values.Set("state", gitlabStateOpened)
	values.Set("scope", "all")
	if filter != "" {
		appendFilter(values, filter)
	}
	return values.Encode()
}

func buildIssueSearchQuery(filter, customQuery string) string {
	if customQuery != "" {
		return customQuery
	}
	values := url.Values{}
	values.Set("state", gitlabStateOpened)
	values.Set("scope", "all")
	if filter != "" {
		appendFilter(values, filter)
	}
	return values.Encode()
}

// filterTokenReviewRequested is the /gitlab page tab value that maps to
// GitLab's reviewer_username param. Defined as a constant because it appears
// in multiple spots across the package (translator, MR controller, issues
// controller) and goconst enforces consistency for 3+ occurrences.
const filterTokenReviewRequested = "review_requested"

// userSearchScopeTokens is the set of frontend filter tokens that map
// directly to GitLab's `scope` query param value on /merge_requests and
// /issues. filterTokenReviewRequested is intentionally absent — GitLab has
// no scope=review_requested; that case routes through reviewer_username=<me>
// instead and is handled in translateUserSearchFilter.
var userSearchScopeTokens = map[string]bool{
	"assigned_to_me": true,
	"created_by_me":  true,
}

// translateUserSearchFilter converts the /gitlab page's filter-tab tokens
// into a GitLab API filter string that buildMRSearchQuery /
// buildIssueSearchQuery can splice in via appendFilter. Returns "" when the
// caller should pass the raw filter through unchanged (already in key=value
// form, unknown token, or empty input). The "review_requested" branch needs
// a resolved username because GitLab has no equivalent scope value — the
// controller is responsible for looking the username up and surfacing any
// error before calling here.
func translateUserSearchFilter(token, username string) string {
	if token == "" {
		return ""
	}
	if strings.ContainsAny(token, "=&") {
		return ""
	}
	if userSearchScopeTokens[token] {
		return "scope=" + token
	}
	if token == filterTokenReviewRequested {
		if username == "" {
			return ""
		}
		return "reviewer_username=" + url.QueryEscape(username) + "&scope=all"
	}
	return ""
}

// appendFilter parses a `key=value&key2=value2` filter and merges it into
// values. User-supplied keys override defaults set by the caller (so passing
// "state=closed" actually swaps the default "opened" rather than appending
// a second value). Unparseable filters are ignored — callers that need
// stricter validation should use customQuery instead.
func appendFilter(values url.Values, filter string) {
	parsed, err := url.ParseQuery(filter)
	if err != nil {
		return
	}
	for k, vs := range parsed {
		values.Del(k)
		for _, v := range vs {
			values.Add(k, v)
		}
	}
}

// countDiffLines returns (additions, deletions) for one file's patch from the
// MR /changes payload. GitLab's REST API carries no per-file line counts, so the
// patch body has to be counted here.
//
// It counts only lines inside a `@@` hunk. The previous "+++"/"---" prefix test
// was content-blind: GitLab returns `--- a/<path>` / `+++ b/<path>` headers ahead
// of the first hunk, but so does a removed SQL comment (`-- x` arrives as
// `--- x`) or an added C increment (`++n;` arrives as `+++n;`), and those were
// dropped from the totals.
func countDiffLines(diff string) (int, int) {
	return unidiff.CountLines(diff)
}

func diffStatus(newFile, deletedFile, renamedFile bool) string {
	switch {
	case newFile:
		return "added"
	case deletedFile:
		return "deleted"
	case renamedFile:
		return "renamed"
	default:
		return "modified"
	}
}
