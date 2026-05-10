package gitlab

import (
	"net/url"
	"strings"
	"time"
)

// rawMR is the JSON shape of a GitLab merge request as returned by the
// REST v4 API.
type rawMR struct {
	ID             int64  `json:"id"`
	IID            int    `json:"iid"`
	ProjectID      int64  `json:"project_id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	State          string `json:"state"` // opened, closed, merged, locked
	WebURL         string `json:"web_url"`
	Draft          bool   `json:"draft"`
	WorkInProgress bool   `json:"work_in_progress"`
	MergeStatus    string `json:"merge_status"`
	HasConflicts   bool   `json:"has_conflicts"`
	SourceBranch   string `json:"source_branch"`
	TargetBranch   string `json:"target_branch"`
	SHA            string `json:"sha"`
	References     struct {
		Full string `json:"full"`
	} `json:"references"`
	Author       rawUser    `json:"author"`
	Reviewers    []rawUser  `json:"reviewers"`
	Assignees    []rawUser  `json:"assignees"`
	ChangesCount string     `json:"changes_count"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	MergedAt     *time.Time `json:"merged_at"`
	ClosedAt     *time.Time `json:"closed_at"`
}

type rawUser struct {
	Username  string `json:"username"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Bot       bool   `json:"bot"`
}

type rawProject struct {
	ID                int64  `json:"id"`
	Path              string `json:"path"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	Visibility        string `json:"visibility"`
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

func convertRawMR(raw *rawMR) *MR {
	state := normalizeMRState(raw.State)
	namespace, projectPath := splitFullReference(raw.References.Full)
	mr := &MR{
		ID:               raw.ID,
		IID:              raw.IID,
		ProjectID:        raw.ProjectID,
		Title:            raw.Title,
		URL:              raw.WebURL,
		WebURL:           raw.WebURL,
		State:            state,
		HeadBranch:       raw.SourceBranch,
		HeadSHA:          raw.SHA,
		BaseBranch:       raw.TargetBranch,
		AuthorUsername:   raw.Author.Username,
		ProjectNamespace: namespace,
		ProjectPath:      projectPath,
		Body:             raw.Description,
		Draft:            raw.Draft || raw.WorkInProgress,
		MergeStatus:      raw.MergeStatus,
		HasConflicts:     raw.HasConflicts,
		Reviewers:        convertReviewers(raw.Reviewers),
		Assignees:        convertReviewers(raw.Assignees),
		CreatedAt:        raw.CreatedAt,
		UpdatedAt:        raw.UpdatedAt,
		MergedAt:         raw.MergedAt,
		ClosedAt:         raw.ClosedAt,
	}
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
	if namespace == "" && strings.Contains(raw.PathWithNamespace, "/") {
		namespace = strings.SplitN(raw.PathWithNamespace, "/", 2)[0]
	}
	return Project{
		ID:                raw.ID,
		PathWithNamespace: raw.PathWithNamespace,
		Namespace:         namespace,
		Path:              raw.Path,
		Name:              raw.Name,
		Visibility:        raw.Visibility,
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

// normalizeMRState converts GitLab's "opened" to "open" and leaves the rest
// alone so the UI shares the GitHub vocabulary.
func normalizeMRState(state string) string {
	switch state {
	case "opened":
		return "open"
	case "merged", "closed", "locked":
		return state
	default:
		return state
	}
}

// splitFullReference parses GitLab's "namespace/path!iid" or
// "namespace/path#iid" form into (namespace, projectPath). It is best-effort:
// when the reference does not match it returns ("", "").
func splitFullReference(full string) (string, string) {
	for _, sep := range []string{"!", "#"} {
		if idx := strings.Index(full, sep); idx > 0 {
			full = full[:idx]
			break
		}
	}
	if !strings.Contains(full, "/") {
		return "", ""
	}
	parts := strings.SplitN(full, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	rest := parts[1]
	last := strings.LastIndex(rest, "/")
	if last < 0 {
		return parts[0], rest
	}
	return parts[0] + "/" + rest[:last], rest[last+1:]
}

func hasOpenDiscussions(discussions []MRDiscussion) bool {
	for _, d := range discussions {
		if d.Resolvable && !d.Resolved {
			return true
		}
	}
	return false
}

func pipelineFailing(pipelines []Pipeline) bool {
	state, _, _ := summarizePipelines(pipelines)
	return state == "failure"
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
	case "success":
		state = "success"
	case "failed", "canceled":
		state = "failure"
	case "skipped":
		state = ""
	default:
		state = statusPending
	}
	return state, jobsTotal, jobsPassing
}

func summarizeApprovals(have, required int) string {
	if required == 0 {
		if have > 0 {
			return "approved"
		}
		return ""
	}
	if have >= required {
		return "approved"
	}
	return statusPending
}

// --- Search query builders ---

// buildReviewMRQuery builds a query string for "MRs needing my review".
// GitLab's /merge_requests endpoint scopes to the authenticated user when
// `scope=assigned_to_me` or `reviewer_username=<me>`; we pass
// `reviewer_username=` resolution to the caller via filter (e.g.
// "reviewer_username=octocat"). state defaults to "opened".
func buildReviewMRQuery(filter, customQuery string) string {
	if customQuery != "" {
		return customQuery
	}
	values := url.Values{}
	values.Set("state", "opened")
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
	values.Set("state", "opened")
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
	values.Set("state", "opened")
	values.Set("scope", "all")
	if filter != "" {
		appendFilter(values, filter)
	}
	return values.Encode()
}

// appendFilter parses a `key=value&key2=value2` filter and merges it into
// values. Unparseable filters are ignored — callers that need stricter
// validation should use customQuery instead.
func appendFilter(values url.Values, filter string) {
	parsed, err := url.ParseQuery(filter)
	if err != nil {
		return
	}
	for k, vs := range parsed {
		for _, v := range vs {
			values.Add(k, v)
		}
	}
}

// countDiffLines returns (additions, deletions) by counting lines starting
// with "+" or "-" (excluding the diff header lines that start with
// "+++"/"---"). Best-effort; matches the GitLab UI's own counting.
func countDiffLines(diff string) (int, int) {
	additions, deletions := 0, 0
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			additions++
		case strings.HasPrefix(line, "-"):
			deletions++
		}
	}
	return additions, deletions
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
