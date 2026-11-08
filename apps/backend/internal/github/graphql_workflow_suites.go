package github

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// graphQLWorkflowSuitesChunkSize bounds the number of PRs per dedicated
	// Actions-suite query. The status batch stays at its own larger chunk.
	graphQLWorkflowSuitesChunkSize = 20
	// graphQLWorkflowSuiteJobsChunkSize bounds suite IDs per job-count query.
	graphQLWorkflowSuiteJobsChunkSize = 50
	// graphQLWorkflowSuitePageSize is the maximum number of suites read per
	// head. A page with more suites is treated as incomplete evidence.
	graphQLWorkflowSuitePageSize = 100
)

// workflowSuiteObservation is the GraphQL evidence for one pull-request head.
// A missing node, a head mismatch, or an overflow page must not become a
// positive classification.
type workflowSuiteObservation struct {
	Owner, Repo   string
	Number        int
	HeadSHA       string
	CommitSHA     string
	HeadBranch    string
	HeadRepoOwner string
	HeadRepoName  string
	HeadRepoURL   string
	Runs          []WorkflowRun
	SuiteIDs      []string
	RunSuiteIDs   map[int64]string
	HasNextPage   bool
	Missing       bool
}

// buildWorkflowSuitesQuery emits the dedicated Actions-suite query. It is
// deliberately separate from the status batch: adding checkSuites there either
// returns 502s or exceeds GitHub's resource limits.
func buildWorkflowSuitesQuery(refs []graphQLPRRef) (string, map[string]any) {
	var b strings.Builder
	b.WriteString("query WorkflowSuites { ")
	for repoIdx, group := range groupGraphQLPRRefs(refs) {
		fmt.Fprintf(&b, `repo%d: repository(owner: %q, name: %q) { `, repoIdx, group.Owner, group.Repo)
		for prIdx, ref := range group.Refs {
			fmt.Fprintf(&b, `wp%d: pullRequest(number: %d) { %s } `,
				prIdx, ref.Number, workflowSuitesFieldsBlock())
		}
		b.WriteString(`} `)
	}
	b.WriteString(`rateLimit { limit remaining resetAt cost } `)
	b.WriteString(`}`)
	return b.String(), nil
}

func workflowSuitesFieldsBlock() string {
	return `headRefOid headRefName ` +
		`headRepository { name nameWithOwner url owner { login } } ` +
		`headRepositoryOwner { login } ` +
		`commits(last: 1) { nodes { commit { oid ` +
		fmt.Sprintf(`checkSuites(first: %d) { pageInfo { hasNextPage } `, graphQLWorkflowSuitePageSize) +
		`nodes { id status conclusion createdAt ` +
		`workflowRun { databaseId event url workflow { databaseId name } } } } } } }`
}

// buildWorkflowSuiteJobsQuery reads the latest check-run count for each suite.
// A zero count is the jobless fork-approval signature.
func buildWorkflowSuiteJobsQuery(suiteIDs []string) (string, map[string]any) {
	var b strings.Builder
	b.WriteString("query WorkflowSuiteJobs { nodes(ids: [")
	for index, id := range suiteIDs {
		if index > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", id)
	}
	b.WriteString(`]) { ... on CheckSuite { checkRuns(first: 0, filterBy: {checkType: LATEST}) { totalCount } } } `)
	b.WriteString(`rateLimit { limit remaining resetAt cost } `)
	b.WriteString(`}`)
	return b.String(), nil
}

type workflowSuiteRepository struct {
	Name          string `json:"name"`
	NameWithOwner string `json:"nameWithOwner"`
	URL           string `json:"url"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

type workflowSuiteRun struct {
	DatabaseID *int64 `json:"databaseId"`
	Event      string `json:"event"`
	URL        string `json:"url"`
	Workflow   *struct {
		DatabaseID *int64 `json:"databaseId"`
		Name       string `json:"name"`
	} `json:"workflow"`
}

type workflowSuiteNode struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	Conclusion  string            `json:"conclusion"`
	CreatedAt   time.Time         `json:"createdAt"`
	WorkflowRun *workflowSuiteRun `json:"workflowRun"`
}

type workflowSuitePRNode struct {
	HeadRefOid          string                  `json:"headRefOid"`
	HeadRefName         string                  `json:"headRefName"`
	HeadRepository      workflowSuiteRepository `json:"headRepository"`
	HeadRepositoryOwner struct {
		Login string `json:"login"`
	} `json:"headRepositoryOwner"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				Oid         string `json:"oid"`
				CheckSuites struct {
					PageInfo graphQLPageInfo     `json:"pageInfo"`
					Nodes    []workflowSuiteNode `json:"nodes"`
				} `json:"checkSuites"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type workflowSuitesResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []graphQLError             `json:"errors"`
}

// decodeWorkflowSuitesResponse keeps every alias that resolved, even when the
// response carries per-alias errors. A path-bearing error marks only the
// affected PR unavailable; an error without a usable path fails the chunk.
func decodeWorkflowSuitesResponse(
	out workflowSuitesResponse, refs []graphQLPRRef,
) (map[string]workflowSuiteObservation, *time.Time, error) {
	retryAt := graphQLRetryAtFromData(out.Data)
	failed, err := workflowSuiteFailedRefs(out.Errors, refs)
	if err != nil {
		return nil, retryAt, err
	}
	observations := make(map[string]workflowSuiteObservation, len(refs))
	for repoIndex, group := range groupGraphQLPRRefs(refs) {
		rawRepo, ok := out.Data[fmt.Sprintf("repo%d", repoIndex)]
		if !ok || len(rawRepo) == 0 || string(rawRepo) == jsonNullLiteral {
			for _, ref := range group.Refs {
				observations[workflowSuiteTargetKey(ref.Owner, ref.Repo, ref.Number)] =
					workflowSuiteObservation{Owner: ref.Owner, Repo: ref.Repo, Number: ref.Number, Missing: true}
			}
			continue
		}
		var repo map[string]json.RawMessage
		if err := json.Unmarshal(rawRepo, &repo); err != nil {
			return nil, retryAt, err
		}
		for index, ref := range group.Refs {
			if _, refused := failed[workflowSuiteTargetKey(ref.Owner, ref.Repo, ref.Number)]; refused {
				observations[workflowSuiteTargetKey(ref.Owner, ref.Repo, ref.Number)] =
					workflowSuiteObservation{Owner: ref.Owner, Repo: ref.Repo, Number: ref.Number, Missing: true}
				continue
			}
			var node workflowSuitePRNode
			raw, ok := repo[fmt.Sprintf("wp%d", index)]
			if !ok || len(raw) == 0 || string(raw) == jsonNullLiteral {
				observations[workflowSuiteTargetKey(ref.Owner, ref.Repo, ref.Number)] =
					workflowSuiteObservation{Owner: ref.Owner, Repo: ref.Repo, Number: ref.Number, Missing: true}
				continue
			}
			if err := json.Unmarshal(raw, &node); err != nil {
				return nil, retryAt, err
			}
			observations[workflowSuiteTargetKey(ref.Owner, ref.Repo, ref.Number)] =
				convertWorkflowSuiteObservation(node, ref)
		}
	}
	return observations, retryAt, nil
}

// workflowSuiteFailedRefs maps path-bearing errors to the PR aliases they
// refuse. Anything else is a whole-chunk failure: a missing path cannot be
// attributed, and a partial decode would risk claiming evidence it never read.
func workflowSuiteFailedRefs(
	errs []graphQLError, refs []graphQLPRRef,
) (map[string]struct{}, error) {
	if len(errs) == 0 {
		return map[string]struct{}{}, nil
	}
	groups := groupGraphQLPRRefs(refs)
	failed := make(map[string]struct{}, len(errs))
	var fatal []graphQLError
	for _, graphErr := range errs {
		affected, ok := workflowSuiteErrorRefs(graphErr.Path, groups)
		if !ok {
			fatal = append(fatal, graphErr)
			continue
		}
		for _, ref := range affected {
			failed[workflowSuiteTargetKey(ref.Owner, ref.Repo, ref.Number)] = struct{}{}
		}
	}
	if len(fatal) > 0 {
		return nil, graphQLErrorsToErr(fatal)
	}
	return failed, nil
}

// workflowSuiteErrorRefs resolves repoN/wpM error paths to the PRs they cover.
// A repository-level path covers every PR in that repository block.
func workflowSuiteErrorRefs(
	path []string, groups []graphQLPRRepoGroup,
) ([]graphQLPRRef, bool) {
	if len(path) == 0 {
		return nil, false
	}
	repoIndex, ok := workflowAliasIndex(path[0], "repo")
	if !ok || repoIndex >= len(groups) {
		return nil, false
	}
	group := groups[repoIndex]
	if len(path) < 2 || !strings.HasPrefix(path[1], "wp") {
		return group.Refs, true
	}
	prIndex, ok := workflowAliasIndex(path[1], "wp")
	if !ok || prIndex >= len(group.Refs) {
		return nil, false
	}
	return []graphQLPRRef{group.Refs[prIndex]}, true
}

func workflowAliasIndex(alias, prefix string) (int, bool) {
	if !strings.HasPrefix(alias, prefix) {
		return 0, false
	}
	index, err := strconv.Atoi(strings.TrimPrefix(alias, prefix))
	if err != nil || index < 0 {
		return 0, false
	}
	return index, true
}

func convertWorkflowSuiteObservation(node workflowSuitePRNode, ref graphQLPRRef) workflowSuiteObservation {
	observation := workflowSuiteObservation{
		Owner: ref.Owner, Repo: ref.Repo, Number: ref.Number,
		HeadSHA: node.HeadRefOid, HeadBranch: node.HeadRefName,
		RunSuiteIDs: make(map[int64]string),
	}
	observation.HeadRepoOwner, observation.HeadRepoName = workflowSuiteHeadRepository(node)
	observation.HeadRepoURL = node.HeadRepository.URL
	if len(node.Commits.Nodes) == 0 {
		observation.Missing = true
		return observation
	}
	commit := node.Commits.Nodes[0].Commit
	observation.CommitSHA = commit.Oid
	observation.HasNextPage = commit.CheckSuites.PageInfo.HasNextPage
	for _, suite := range commit.CheckSuites.Nodes {
		run, ok := convertWorkflowSuiteRun(suite, observation)
		if !ok {
			continue
		}
		run.PullRequests = []WorkflowRunPullRequest{{
			Number: ref.Number, HeadSHA: observation.CommitSHA, HeadBranch: observation.HeadBranch,
			HeadRepoOwner: observation.HeadRepoOwner, HeadRepoName: observation.HeadRepoName,
			HeadRepoURL: observation.HeadRepoURL,
		}}
		observation.Runs = append(observation.Runs, run)
		if workflowSuiteNeedsJobCount(run, ref.Owner, ref.Repo) {
			observation.SuiteIDs = append(observation.SuiteIDs, suite.ID)
			observation.RunSuiteIDs[run.ID] = suite.ID
		}
	}
	return observation
}

func workflowSuiteHeadRepository(node workflowSuitePRNode) (string, string) {
	owner := strings.TrimSpace(node.HeadRepositoryOwner.Login)
	name := strings.TrimSpace(node.HeadRepository.Name)
	if owner != "" && name != "" {
		return owner, name
	}
	parsedOwner, parsedName := workflowRepositoryPath(node.HeadRepository.NameWithOwner)
	if owner == "" {
		owner = parsedOwner
	}
	if name == "" {
		name = parsedName
	}
	if owner == "" {
		owner = node.HeadRepository.Owner.Login
	}
	return owner, name
}

func convertWorkflowSuiteRun(
	suite workflowSuiteNode, observation workflowSuiteObservation,
) (WorkflowRun, bool) {
	if suite.WorkflowRun == nil || suite.WorkflowRun.DatabaseID == nil {
		return WorkflowRun{}, false
	}
	run := WorkflowRun{
		ID:            *suite.WorkflowRun.DatabaseID,
		RunAttempt:    0,
		Event:         suite.WorkflowRun.Event,
		Status:        strings.ToLower(suite.Status),
		Conclusion:    strings.ToLower(suite.Conclusion),
		HeadSHA:       observation.CommitSHA,
		HeadBranch:    observation.HeadBranch,
		HeadRepoOwner: observation.HeadRepoOwner,
		HeadRepoName:  observation.HeadRepoName,
		HeadRepoURL:   observation.HeadRepoURL,
		HTMLURL:       suite.WorkflowRun.URL,
		CreatedAt:     suite.CreatedAt,
		UpdatedAt:     suite.CreatedAt,
	}
	if suite.WorkflowRun.Workflow != nil {
		if suite.WorkflowRun.Workflow.DatabaseID != nil {
			run.WorkflowID = *suite.WorkflowRun.Workflow.DatabaseID
		}
		run.Name = suite.WorkflowRun.Workflow.Name
	}
	return run, true
}

func workflowSuiteNeedsJobCount(run WorkflowRun, owner, repo string) bool {
	return strings.EqualFold(run.Conclusion, workflowConclusionActionRequired) &&
		strings.EqualFold(run.Event, workflowEventPullRequest) &&
		strings.EqualFold(run.Status, workflowStatusCompleted) &&
		isForkWorkflowRun(run, owner, repo)
}

func workflowSuiteTargetKey(owner, repo string, number int) string {
	return fmt.Sprintf("%s/%s#%d", strings.ToLower(owner), strings.ToLower(repo), number)
}

type workflowSuiteJobsNode struct {
	CheckRuns *struct {
		TotalCount int `json:"totalCount"`
	} `json:"checkRuns"`
}

type workflowSuiteJobsResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []graphQLError             `json:"errors"`
}

// decodeWorkflowSuiteJobCounts maps suite ID to its latest check-run count.
// Missing nodes are reported separately so callers can treat them as
// unavailable evidence instead of a zero-job approval claim.
func decodeWorkflowSuiteJobCounts(
	out workflowSuiteJobsResponse, suiteIDs []string,
) (map[string]int, map[string]struct{}, *time.Time, error) {
	retryAt := graphQLRetryAtFromData(out.Data)
	failed, err := workflowSuiteJobErrorIndexes(out.Errors, len(suiteIDs))
	if err != nil {
		return nil, nil, retryAt, err
	}
	var nodes []*workflowSuiteJobsNode
	if raw, ok := out.Data["nodes"]; ok && len(raw) > 0 && string(raw) != jsonNullLiteral {
		if err := json.Unmarshal(raw, &nodes); err != nil {
			return nil, nil, retryAt, err
		}
	}
	counts := make(map[string]int, len(suiteIDs))
	missing := make(map[string]struct{})
	for index, id := range suiteIDs {
		if _, refused := failed[index]; refused {
			missing[id] = struct{}{}
			continue
		}
		if index >= len(nodes) || nodes[index] == nil || nodes[index].CheckRuns == nil {
			missing[id] = struct{}{}
			continue
		}
		counts[id] = nodes[index].CheckRuns.TotalCount
	}
	return counts, missing, retryAt, nil
}

// workflowSuiteJobErrorIndexes maps nodes(ids:) error paths to the requested
// suite positions. Only a path-bearing error at one node leaves that suite
// unavailable; anything else fails the whole chunk.
func workflowSuiteJobErrorIndexes(errs []graphQLError, count int) (map[int]struct{}, error) {
	if len(errs) == 0 {
		return map[int]struct{}{}, nil
	}
	failed := make(map[int]struct{}, len(errs))
	var fatal []graphQLError
	for _, graphErr := range errs {
		if len(graphErr.Path) < 2 || graphErr.Path[0] != "nodes" {
			fatal = append(fatal, graphErr)
			continue
		}
		index, err := strconv.Atoi(graphErr.Path[1])
		if err != nil || index < 0 || index >= count {
			fatal = append(fatal, graphErr)
			continue
		}
		failed[index] = struct{}{}
	}
	if len(fatal) > 0 {
		return nil, graphQLErrorsToErr(fatal)
	}
	return failed, nil
}
