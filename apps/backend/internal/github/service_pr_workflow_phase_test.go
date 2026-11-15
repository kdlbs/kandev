package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

type workflowPhaseCountingClient struct {
	*MockClient
	runCalls   atomic.Int64
	jobCalls   atomic.Int64
	getPRCalls atomic.Int64
	deferRuns  atomic.Int64
	deferJobs  atomic.Int64
}

func (c *workflowPhaseCountingClient) GetPR(
	ctx context.Context, owner, repo string, number int,
) (*PR, error) {
	c.getPRCalls.Add(1)
	return c.MockClient.GetPR(ctx, owner, repo, number)
}

func (c *workflowPhaseCountingClient) ListWorkflowRuns(
	ctx context.Context, owner, repo, headSHA string,
) ([]WorkflowRun, error) {
	c.runCalls.Add(1)
	if c.deferRuns.Load() > 0 {
		c.deferRuns.Add(-1)
		return nil, &AdmissionDeferredError{Delay: time.Second, Reason: "test REST pacing"}
	}
	return c.MockClient.ListWorkflowRuns(ctx, owner, repo, headSHA)
}

func (c *workflowPhaseCountingClient) ListWorkflowRunJobs(
	ctx context.Context, owner, repo string, runID int64, attempt int,
) ([]WorkflowJob, error) {
	c.jobCalls.Add(1)
	if c.deferJobs.Load() > 0 {
		c.deferJobs.Add(-1)
		return nil, &AdmissionDeferredError{Delay: time.Second, Reason: "test REST job pacing"}
	}
	return c.MockClient.ListWorkflowRunJobs(ctx, owner, repo, runID, attempt)
}

type workflowTestSuite struct {
	id         string
	status     string
	conclusion string
	event      string
	runID      int64
	workflowID int64
	name       string
	jobCount   int
}

type workflowTestPR struct {
	number       int
	storedHead   string
	queryHead    string
	branch       string
	headOwner    string
	headName     string
	suites       []workflowTestSuite
	pageOverflow bool
}

type workflowTestExecutor struct {
	mu           sync.Mutex
	prs          map[int]*workflowTestPR
	step1Calls   int
	step2Calls   int
	deferStep1   int
	deferStep2   int
	invalidStep1 bool
	invalidStep2 bool
	// onStep1 runs inside the executor lock before a suites response.
	onStep1 func()
	// aliasErrors maps a PR number to a per-alias GraphQL error message.
	aliasErrors map[int]string
	// jobErrors maps a suite ID to a per-node GraphQL error message.
	jobErrors map[string]string
}

var (
	workflowTestRepoPattern = regexp.MustCompile(`repo(\d+): repository\(owner: "([^"]+)", name: "([^"]+)"\)`)
	workflowTestPRPattern   = regexp.MustCompile(`wp(\d+): pullRequest\(number: (\d+)\)`)
	workflowTestSuiteID     = regexp.MustCompile(`"(CS_[^"]+)"`)
)

func newWorkflowTestExecutor() *workflowTestExecutor {
	return &workflowTestExecutor{prs: make(map[int]*workflowTestPR)}
}

func (e *workflowTestExecutor) setPR(pr workflowTestPR) {
	e.mu.Lock()
	defer e.mu.Unlock()
	copy := pr
	if copy.branch == "" {
		copy.branch = fmt.Sprintf("feature-%d", pr.number)
	}
	if copy.headOwner == "" {
		copy.headOwner = "contributor"
	}
	if copy.headName == "" {
		copy.headName = "widget-fork"
	}
	e.prs[pr.number] = &copy
}

func (e *workflowTestExecutor) ExecuteGraphQL(
	_ context.Context, query string, _ map[string]any, out any,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case strings.Contains(query, "query WorkflowSuites"):
		e.step1Calls++
		if hook := e.onStep1; hook != nil {
			hook()
		}
		if e.deferStep1 > 0 {
			e.deferStep1--
			return &AdmissionDeferredError{Delay: time.Second, Reason: "test workflow pacing"}
		}
		if e.invalidStep1 {
			return errors.New("graphql validation failed: Cannot query field 'checkSuites' on type 'Commit'")
		}
		return e.respondWorkflowSuites(query, out)
	case strings.Contains(query, "query WorkflowSuiteJobs"):
		e.step2Calls++
		if e.deferStep2 > 0 {
			e.deferStep2--
			return &AdmissionDeferredError{Delay: time.Second, Reason: "test workflow pacing"}
		}
		if e.invalidStep2 {
			return errors.New("graphql validation failed: Cannot query field 'checkRuns' on type 'CheckSuite'")
		}
		return e.respondWorkflowSuiteJobs(query, out)
	default:
		return fmt.Errorf("unexpected workflow test query: %s", query)
	}
}

func (e *workflowTestExecutor) respondWorkflowSuites(query string, out any) error {
	repoAlias := "repo0"
	if match := workflowTestRepoPattern.FindStringSubmatch(query); len(match) > 0 {
		repoAlias = "repo" + match[1]
	}
	repo := make(map[string]any)
	graphErrors := make([]any, 0)
	for _, match := range workflowTestPRPattern.FindAllStringSubmatch(query, -1) {
		number, _ := strconv.Atoi(match[2])
		pr := e.prs[number]
		if pr == nil {
			repo["wp"+match[1]] = nil
			continue
		}
		repo["wp"+match[1]] = workflowTestPRNode(*pr)
		if message, ok := e.aliasErrors[number]; ok {
			graphErrors = append(graphErrors, map[string]any{
				"message": message, "path": []string{repoAlias, "wp" + match[1]},
			})
		}
	}
	payload, err := json.Marshal(map[string]any{
		"data": map[string]any{repoAlias: repo}, "errors": graphErrors,
	})
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func workflowTestPRNode(pr workflowTestPR) map[string]any {
	head := pr.storedHead
	if pr.queryHead != "" {
		head = pr.queryHead
	}
	suites := make([]any, 0, len(pr.suites))
	for _, suite := range pr.suites {
		node := map[string]any{
			"id": suite.id, "status": suite.status, "conclusion": suite.conclusion,
			"createdAt": "2026-10-29T00:00:00Z",
		}
		if suite.runID != 0 {
			node["workflowRun"] = map[string]any{
				"databaseId": suite.runID, "event": suite.event,
				"url":      fmt.Sprintf("https://github.com/acme/widget/actions/runs/%d", suite.runID),
				"workflow": map[string]any{"databaseId": suite.workflowID, "name": suite.name},
			}
		}
		suites = append(suites, node)
	}
	return map[string]any{
		"headRefOid": head, "headRefName": pr.branch,
		"headRepository": map[string]any{
			"name": pr.headName, "nameWithOwner": pr.headOwner + "/" + pr.headName,
			"url":   "https://github.com/" + pr.headOwner + "/" + pr.headName,
			"owner": map[string]any{"login": pr.headOwner},
		},
		"headRepositoryOwner": map[string]any{"login": pr.headOwner},
		"commits": map[string]any{"nodes": []any{map[string]any{"commit": map[string]any{
			"oid": head,
			"checkSuites": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": pr.pageOverflow},
				"nodes":    suites,
			},
		}}}},
	}
}

func (e *workflowTestExecutor) respondWorkflowSuiteJobs(query string, out any) error {
	counts := make(map[string]int)
	for _, pr := range e.prs {
		for _, suite := range pr.suites {
			counts[suite.id] = suite.jobCount
		}
	}
	nodes := make([]any, 0)
	graphErrors := make([]any, 0)
	matches := workflowTestSuiteID.FindAllStringSubmatch(query, -1)
	for index, match := range matches {
		count, ok := counts[match[1]]
		if !ok || count < 0 {
			nodes = append(nodes, nil)
			continue
		}
		nodes = append(nodes, map[string]any{"checkRuns": map[string]any{"totalCount": count}})
		if message, refused := e.jobErrors[match[1]]; refused {
			graphErrors = append(graphErrors, map[string]any{
				"message": message, "path": []string{"nodes", strconv.Itoa(index)},
			})
		}
	}
	payload, err := json.Marshal(map[string]any{
		"data": map[string]any{"nodes": nodes}, "errors": graphErrors,
	})
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func (e *workflowTestExecutor) counts() (step1, step2 int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.step1Calls, e.step2Calls
}

func forkApprovalTestPR(number int, headSHA string) workflowTestPR {
	return workflowTestPR{
		number: number, storedHead: headSHA, branch: fmt.Sprintf("feature-%d", number),
		suites: []workflowTestSuite{{
			id: fmt.Sprintf("CS_APPROVAL_%d", number), status: "COMPLETED", conclusion: "ACTION_REQUIRED",
			event: "pull_request", runID: int64(1000 + number), workflowID: int64(10 + number), name: "CI",
			jobCount: 0,
		}},
	}
}

func sameRepoActionRequiredTestPR(number int, headSHA string) workflowTestPR {
	return workflowTestPR{
		number: number, storedHead: headSHA, branch: fmt.Sprintf("feature-%d", number),
		headOwner: "acme", headName: "widget",
		suites: []workflowTestSuite{{
			id: fmt.Sprintf("CS_ACTION_%d", number), status: "COMPLETED", conclusion: "ACTION_REQUIRED",
			event: "pull_request", runID: int64(2000 + number), workflowID: int64(20 + number), name: "CI",
			jobCount: 2,
		}},
	}
}

func completedSuccessTestPR(number int, headSHA string) workflowTestPR {
	return workflowTestPR{
		number: number, storedHead: headSHA, branch: fmt.Sprintf("feature-%d", number),
		suites: []workflowTestSuite{{
			id: fmt.Sprintf("CS_SUCCESS_%d", number), status: "COMPLETED", conclusion: "SUCCESS",
			event: "pull_request", runID: int64(3000 + number), workflowID: int64(30 + number), name: "CI",
		}},
	}
}

func seedWorkflowPhaseTarget(
	t *testing.T, store *Store, number int, pr workflowTestPR, prior *WorkflowAttention,
) *PRWatch {
	t.Helper()
	taskID := fmt.Sprintf("workflow-phase-task-%d", number)
	row := &TaskPR{
		WorkspaceID: testWorkspaceID, TaskID: taskID, RepositoryID: "repo-1",
		Owner: "acme", Repo: "widget", PRNumber: number,
		PRURL:   fmt.Sprintf("https://github.com/acme/widget/pull/%d", number),
		PRTitle: "workflow phase PR", HeadBranch: pr.branch, BaseBranch: "main",
		State: "open", HeadSHA: pr.storedHead, WorkflowAttention: prior,
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, store.CreateTaskPR(context.Background(), row))
	return &PRWatch{
		WorkspaceID: testWorkspaceID, TaskID: taskID, SessionID: "session-" + taskID,
		RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: number, Branch: pr.branch,
	}
}

func countWorkflowPhaseTaskPREvents(t *testing.T, eventBus bus.EventBus) func() int {
	t.Helper()
	var calls atomic.Int64
	subscription, err := eventBus.Subscribe(events.GitHubTaskPRUpdated, func(context.Context, *bus.Event) error {
		calls.Add(1)
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	return func() int { return int(calls.Load()) }
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.1
// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.1
func TestWorkflowPhase_ClassifiesManyHeadsWithoutRESTActions(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	executor := newWorkflowTestExecutor()
	watches := make([]*PRWatch, 0, 30)
	for number := 1; number <= 30; number++ {
		head := fmt.Sprintf("head-%d", number)
		var pr workflowTestPR
		switch {
		case number <= 28:
			pr = forkApprovalTestPR(number, head)
		case number == 29:
			pr = sameRepoActionRequiredTestPR(number, head)
		default:
			pr = completedSuccessTestPR(number, head)
		}
		executor.setPR(pr)
		watches = append(watches, seedWorkflowPhaseTarget(t, store, number, pr, nil))
	}

	result := service.runWorkflowPhase(context.Background(), client, executor, "test-scope", watches, false)
	require.NoError(t, result.Err)
	require.Nil(t, result.Deferred)
	require.Empty(t, result.Unfinished)

	step1, step2 := executor.counts()
	require.Equal(t, 2, step1, "thirty PRs must split into two dedicated suite queries")
	require.Equal(t, 1, step2, "thirty gated suites fit one job-count query")
	require.Zero(t, client.runCalls.Load(), "the GraphQL phase must not issue REST Actions run reads")
	require.Zero(t, client.jobCalls.Load(), "the GraphQL phase must not issue REST Actions job reads")

	for number := 1; number <= 30; number++ {
		row, err := store.GetTaskPRByRepoAndNumber(
			context.Background(), fmt.Sprintf("workflow-phase-task-%d", number), "repo-1", number,
		)
		require.NoError(t, err)
		require.NotNil(t, row)
		switch {
		case number <= 28:
			require.Equal(t, WorkflowAttentionApprovalRequired, row.WorkflowAttention.State)
			require.Len(t, row.WorkflowAttention.Runs, 1)
			require.Equal(t, workflowAttentionApprovalRequiredReason, row.WorkflowAttention.Runs[0].Reason)
		case number == 29:
			require.Equal(t, WorkflowAttentionActionRequired, row.WorkflowAttention.State)
			require.Equal(t, workflowAttentionActionRequiredReason, row.WorkflowAttention.Runs[0].Reason)
		default:
			require.Equal(t, WorkflowAttentionNone, row.WorkflowAttention.State)
		}
	}
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.5
func TestWorkflowPhase_DeferralLeavesStoredObservationAndReturnsPending(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	executor := newWorkflowTestExecutor()
	executor.deferStep1 = 1
	prior := &WorkflowAttention{
		State: WorkflowAttentionApprovalRequired, HeadSHA: "head-1", ObservedAt: time.Now().UTC(),
		Runs: []WorkflowAttentionRun{{RunID: 7, Name: "CI", Reason: workflowAttentionApprovalRequiredReason}},
	}
	pr := forkApprovalTestPR(1, "head-1")
	executor.setPR(pr)
	watch := seedWorkflowPhaseTarget(t, store, 1, pr, prior)
	before, err := store.GetTaskPRByRepoAndNumber(context.Background(), watch.TaskID, "repo-1", 1)
	require.NoError(t, err)
	beforeJSON := before.WorkflowAttentionJSON
	events := countWorkflowPhaseTaskPREvents(t, service.eventBus)

	result := service.runWorkflowPhase(context.Background(), client, executor, "test-scope", []*PRWatch{watch}, false)
	require.NotNil(t, result.Deferred, "local admission must surface a typed deferral")
	require.Len(t, result.Unfinished, 1)
	after, err := store.GetTaskPRByRepoAndNumber(context.Background(), watch.TaskID, "repo-1", 1)
	require.NoError(t, err)
	require.Equal(t, beforeJSON, after.WorkflowAttentionJSON, "deferred work must leave the stored observation untouched")
	require.Zero(t, events(), "deferred work must not publish")

	success := completedSuccessTestPR(1, "head-1")
	executor.setPR(success)
	result = service.runWorkflowPhase(context.Background(), client, executor, "test-scope", []*PRWatch{watch}, false)
	require.Nil(t, result.Deferred)
	require.Empty(t, result.Unfinished)
	after, err = store.GetTaskPRByRepoAndNumber(context.Background(), watch.TaskID, "repo-1", 1)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionNone, after.WorkflowAttention.State)
	require.Equal(t, 1, events(), "a completed refresh publishes the cleared observation")
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.5
// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.6
func TestWorkflowPhase_HeadChangeAndPageOverflowAreUnknown(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	executor := newWorkflowTestExecutor()
	prior := func(number int) *WorkflowAttention {
		return &WorkflowAttention{
			State: WorkflowAttentionApprovalRequired, HeadSHA: fmt.Sprintf("head-%d", number),
			ObservedAt: time.Now().UTC(),
			Runs:       []WorkflowAttentionRun{{RunID: int64(number), Name: "CI", Reason: workflowAttentionApprovalRequiredReason}},
		}
	}
	changedHead := forkApprovalTestPR(1, "head-1")
	changedHead.queryHead = "head-2"
	overflow := forkApprovalTestPR(2, "head-2")
	overflow.pageOverflow = true
	missingCount := forkApprovalTestPR(3, "head-3")
	missingCount.suites[0].jobCount = -1
	executor.setPR(changedHead)
	executor.setPR(overflow)
	executor.setPR(missingCount)
	watches := []*PRWatch{
		seedWorkflowPhaseTarget(t, store, 1, changedHead, prior(1)),
		seedWorkflowPhaseTarget(t, store, 2, overflow, nil),
		seedWorkflowPhaseTarget(t, store, 3, missingCount, nil),
	}

	result := service.runWorkflowPhase(context.Background(), client, executor, "test-scope", watches, false)
	require.NoError(t, result.Err)
	require.Empty(t, result.Unfinished)

	first, err := store.GetTaskPRByRepoAndNumber(context.Background(), "workflow-phase-task-1", "repo-1", 1)
	require.NoError(t, err)
	require.True(t, first.WorkflowAttention.Stale, "a head mismatch preserves same-head evidence as stale")
	require.Equal(t, WorkflowAttentionApprovalRequired, first.WorkflowAttention.State)

	second, err := store.GetTaskPRByRepoAndNumber(context.Background(), "workflow-phase-task-2", "repo-1", 2)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionUnknown, second.WorkflowAttention.State)
	require.True(t, second.WorkflowAttention.Stale)

	third, err := store.GetTaskPRByRepoAndNumber(context.Background(), "workflow-phase-task-3", "repo-1", 3)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionUnknown, third.WorkflowAttention.State,
		"a missing job count must not become a zero-job approval claim")
	require.True(t, third.WorkflowAttention.Stale)
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.1
func TestWorkflowPhase_UnsupportedGraphQLUsesSerialREST(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	executor := newWorkflowTestExecutor()
	executor.invalidStep1 = true
	pr := forkApprovalTestPR(1, "head-1")
	executor.setPR(pr)
	watch := seedWorkflowPhaseTarget(t, store, 1, pr, nil)

	client.AddPR(&PR{
		Number: 1, State: "open", HeadSHA: "head-1", HeadBranch: pr.branch,
		RepoOwner: "acme", RepoName: "widget",
		HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	})
	client.ReplaceWorkflowRuns("acme", "widget", "head-1", []WorkflowRun{{
		ID: 1001, RunAttempt: 1, WorkflowID: 11, Name: "CI", Event: "pull_request",
		Status: "completed", Conclusion: "action_required",
		HeadSHA: "head-1", HeadBranch: pr.branch,
		HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	}})
	client.ReplaceWorkflowRunJobs("acme", "widget", 1001, 1, nil)

	result := service.runWorkflowPhase(context.Background(), client, executor, "test-scope", []*PRWatch{watch}, false)
	require.NoError(t, result.Err)
	require.Empty(t, result.Unfinished)
	require.Equal(t, int64(1), client.runCalls.Load(), "invalid GraphQL must fall back to one serial REST run read")
	require.Equal(t, int64(1), client.jobCalls.Load(), "the REST fallback reads jobs once")

	row, err := store.GetTaskPRByRepoAndNumber(context.Background(), watch.TaskID, "repo-1", 1)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionApprovalRequired, row.WorkflowAttention.State)
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
func TestWorkflowPhase_StepTwoDeferralLeavesStoredObservationUntouched(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	executor := newWorkflowTestExecutor()
	executor.deferStep2 = 1
	pr := forkApprovalTestPR(1, "head-1")
	executor.setPR(pr)
	watch := seedWorkflowPhaseTarget(t, store, 1, pr, nil)
	events := countWorkflowPhaseTaskPREvents(t, service.eventBus)

	result := service.runWorkflowPhase(context.Background(), client, executor, "test-scope", []*PRWatch{watch}, false)
	require.NotNil(t, result.Deferred)
	require.Len(t, result.Unfinished, 1)
	row, err := store.GetTaskPRByRepoAndNumber(context.Background(), watch.TaskID, "repo-1", 1)
	require.NoError(t, err)
	require.Nil(t, row.WorkflowAttention, "step-2 deferral must not write unavailable evidence")
	require.Zero(t, events())
	step1, step2 := executor.counts()
	require.Equal(t, 1, step1)
	require.Equal(t, 1, step2)

	result = service.runWorkflowPhase(context.Background(), client, executor, "test-scope", []*PRWatch{watch}, false)
	require.Nil(t, result.Deferred)
	require.Empty(t, result.Unfinished)
	row, err = store.GetTaskPRByRepoAndNumber(context.Background(), watch.TaskID, "repo-1", 1)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionApprovalRequired, row.WorkflowAttention.State)
	require.Equal(t, 1, events())
	step1, step2 = executor.counts()
	require.Equal(t, 1, step1, "the resumed step 2 must reuse retained suite evidence")
	require.Equal(t, 2, step2)
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.1
// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002.4
func TestWorkflowPhase_NilExecutorUsesSerialREST(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	pr := forkApprovalTestPR(1, "head-1")
	watch := seedWorkflowPhaseTarget(t, store, 1, pr, nil)
	client.AddPR(&PR{
		Number: 1, State: "open", HeadSHA: "head-1", HeadBranch: pr.branch,
		RepoOwner: "acme", RepoName: "widget",
		HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	})
	client.ReplaceWorkflowRuns("acme", "widget", "head-1", []WorkflowRun{{
		ID: 1001, RunAttempt: 1, WorkflowID: 11, Name: "CI", Event: "pull_request",
		Status: "completed", Conclusion: "action_required",
		HeadSHA: "head-1", HeadBranch: pr.branch,
		HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	}})
	client.ReplaceWorkflowRunJobs("acme", "widget", 1001, 1, nil)

	result := service.runWorkflowPhase(context.Background(), client, nil, "test-scope", []*PRWatch{watch}, false)
	require.NoError(t, result.Err)
	require.Empty(t, result.Unfinished)
	require.Equal(t, int64(1), client.runCalls.Load())
	require.Equal(t, int64(1), client.jobCalls.Load())

	row, err := store.GetTaskPRByRepoAndNumber(context.Background(), watch.TaskID, "repo-1", 1)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionApprovalRequired, row.WorkflowAttention.State)
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.6
func TestWorkflowPhase_PartialAliasErrorKeepsSiblingApproval(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	executor := newWorkflowTestExecutor()
	executor.aliasErrors = map[int]string{2: "Could not resolve to a PullRequest with the number of 2."}
	watches := make([]*PRWatch, 0, 3)
	for number := 1; number <= 3; number++ {
		pr := forkApprovalTestPR(number, fmt.Sprintf("head-%d", number))
		executor.setPR(pr)
		watches = append(watches, seedWorkflowPhaseTarget(t, store, number, pr, nil))
	}

	result := service.runWorkflowPhase(context.Background(), client, executor, "test-scope", watches, false)
	require.NoError(t, result.Err)
	require.Empty(t, result.Unfinished)

	for _, number := range []int{1, 3} {
		row, err := store.GetTaskPRByRepoAndNumber(
			context.Background(), fmt.Sprintf("workflow-phase-task-%d", number), "repo-1", number,
		)
		require.NoError(t, err)
		require.Equal(t, WorkflowAttentionApprovalRequired, row.WorkflowAttention.State,
			"an unresolvable sibling alias must not hide approval for PR %d", number)
	}
	failed, err := store.GetTaskPRByRepoAndNumber(context.Background(), "workflow-phase-task-2", "repo-1", 2)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionUnknown, failed.WorkflowAttention.State)
	step1, step2 := executor.counts()
	require.Equal(t, 1, step1)
	require.Equal(t, 1, step2, "only the two resolved approval candidates need job counts")
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.10
func TestWorkflowPhase_PartialJobCountErrorKeepsSiblingApproval(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	executor := newWorkflowTestExecutor()
	executor.jobErrors = map[string]string{
		"CS_APPROVAL_2": "Could not resolve to a CheckSuite with the global id of CS_APPROVAL_2.",
	}
	watches := make([]*PRWatch, 0, 3)
	for number := 1; number <= 3; number++ {
		pr := forkApprovalTestPR(number, fmt.Sprintf("head-%d", number))
		executor.setPR(pr)
		watches = append(watches, seedWorkflowPhaseTarget(t, store, number, pr, nil))
	}

	result := service.runWorkflowPhase(context.Background(), client, executor, "test-scope", watches, false)
	require.NoError(t, result.Err)
	require.Empty(t, result.Unfinished)

	for _, number := range []int{1, 3} {
		row, err := store.GetTaskPRByRepoAndNumber(
			context.Background(), fmt.Sprintf("workflow-phase-task-%d", number), "repo-1", number,
		)
		require.NoError(t, err)
		require.Equal(t, WorkflowAttentionApprovalRequired, row.WorkflowAttention.State,
			"an unresolvable sibling suite must not hide approval for PR %d", number)
	}
	failed, err := store.GetTaskPRByRepoAndNumber(context.Background(), "workflow-phase-task-2", "repo-1", 2)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionUnknown, failed.WorkflowAttention.State,
		"the unresolvable suite's own PR must stay unavailable")
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.6
// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.7
func TestWorkflowPhase_GraphQLRunAssociationCoversNonPullRequestEvents(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	executor := newWorkflowTestExecutor()
	sameRepo := workflowTestPR{
		number: 1, storedHead: "head-1", branch: "feature-1",
		headOwner: "acme", headName: "widget",
		suites: []workflowTestSuite{{
			id: "CS_TARGET_1", status: "COMPLETED", conclusion: "ACTION_REQUIRED",
			event: "pull_request_target", runID: 1001, workflowID: 11, name: "CI target",
		}},
	}
	forkApproval := forkApprovalTestPR(2, "head-2")
	executor.setPR(sameRepo)
	executor.setPR(forkApproval)
	watches := []*PRWatch{
		seedWorkflowPhaseTarget(t, store, 1, sameRepo, nil),
		seedWorkflowPhaseTarget(t, store, 2, forkApproval, nil),
	}

	result := service.runWorkflowPhase(context.Background(), client, executor, "test-scope", watches, false)
	require.NoError(t, result.Err)
	require.Empty(t, result.Unfinished)

	generic, err := store.GetTaskPRByRepoAndNumber(context.Background(), "workflow-phase-task-1", "repo-1", 1)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionActionRequired, generic.WorkflowAttention.State,
		"a PR-head pull_request_target suite must keep generic attention")
	require.Equal(t, workflowAttentionActionRequiredReason, generic.WorkflowAttention.Runs[0].Reason)

	approval, err := store.GetTaskPRByRepoAndNumber(context.Background(), "workflow-phase-task-2", "repo-1", 2)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionApprovalRequired, approval.WorkflowAttention.State,
		"the jobless fork approval case must still classify approval_required")
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
func TestWorkflowPhase_GraphQLRateLimitRetainsRetryBoundary(t *testing.T) {
	service := &Service{}
	resetAt := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	response := fmt.Sprintf(
		`{"data":{"rateLimit":{"limit":5000,"remaining":0,"resetAt":%q,"cost":1}},`+
			`"errors":[{"message":"API rate limit exceeded"}]}`, resetAt.Format(time.RFC3339))
	exec := &stubGraphQLExecutor{response: response}

	_, err := service.fetchWorkflowSuiteChunk(context.Background(), exec, []graphQLPRRef{{
		Owner: "acme", Repo: "widget", Number: 1,
	}})
	require.Error(t, err)
	boundary := prDiscoveryRetryAtFromError(err)
	require.NotNil(t, boundary)
	require.True(t, boundary.Equal(resetAt), "suites error boundary = %v, want %v", boundary, resetAt)

	exec = &stubGraphQLExecutor{response: response}
	_, _, err = service.fetchWorkflowSuiteJobCounts(context.Background(), exec, []string{"CS_1"})
	require.Error(t, err)
	boundary = prDiscoveryRetryAtFromError(err)
	require.NotNil(t, boundary)
	require.True(t, boundary.Equal(resetAt), "job-count error boundary = %v, want %v", boundary, resetAt)
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.5
func TestWorkflowPhase_SkipsRowsWithoutHead(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	executor := newWorkflowTestExecutor()
	pr := forkApprovalTestPR(1, "")
	executor.setPR(pr)
	watch := seedWorkflowPhaseTarget(t, store, 1, pr, nil)
	events := countWorkflowPhaseTaskPREvents(t, service.eventBus)

	result := service.runWorkflowPhase(context.Background(), client, executor, "test-scope", []*PRWatch{watch}, false)
	require.NoError(t, result.Err)
	require.Empty(t, result.Unfinished)
	step1, step2 := executor.counts()
	require.Zero(t, step1)
	require.Zero(t, step2)
	row, err := store.GetTaskPRByRepoAndNumber(context.Background(), watch.TaskID, "repo-1", 1)
	require.NoError(t, err)
	require.Nil(t, row.WorkflowAttention, "an empty-head row must never receive head-scoped evidence")
	require.Zero(t, events())
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
func TestWorkflowPhase_RESTFallbackRetainsPRIdentityAcrossDeferral(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	client.deferRuns.Store(1)
	pr := forkApprovalTestPR(1, "head-1")
	watch := seedWorkflowPhaseTarget(t, store, 1, pr, nil)
	client.AddPR(&PR{
		Number: 1, State: "open", HeadSHA: "head-1", HeadBranch: pr.branch,
		RepoOwner: "acme", RepoName: "widget",
		HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	})
	client.ReplaceWorkflowRuns("acme", "widget", "head-1", []WorkflowRun{{
		ID: 1001, RunAttempt: 1, WorkflowID: 11, Name: "CI", Event: "pull_request",
		Status: "completed", Conclusion: "action_required",
		HeadSHA: "head-1", HeadBranch: pr.branch,
		HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	}})
	client.ReplaceWorkflowRunJobs("acme", "widget", 1001, 1, nil)

	result := service.runWorkflowPhase(context.Background(), client, nil, "test-scope", []*PRWatch{watch}, false)
	require.NotNil(t, result.Deferred)
	require.Len(t, result.Unfinished, 1)
	require.Equal(t, int64(1), client.getPRCalls.Load())
	require.Equal(t, int64(1), client.runCalls.Load())

	result = service.runWorkflowPhase(context.Background(), client, nil, "test-scope", []*PRWatch{watch}, false)
	require.Nil(t, result.Deferred)
	require.Empty(t, result.Unfinished)
	require.Equal(t, int64(1), client.getPRCalls.Load(),
		"the deferred wake must reuse the fetched PR identity, not spend another Core read")
	require.Equal(t, int64(2), client.runCalls.Load())

	row, err := store.GetTaskPRByRepoAndNumber(context.Background(), watch.TaskID, "repo-1", 1)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionApprovalRequired, row.WorkflowAttention.State)
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
func TestWorkflowPhase_SharedEvidenceUpdatesEveryCallerTask(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	executor := newWorkflowTestExecutor()
	pr := forkApprovalTestPR(42, "head-42")
	pr.branch = "feature-42"
	executor.setPR(pr)
	first := seedWorkflowPhaseTarget(t, store, 42, pr, nil)
	secondTaskID := "workflow-phase-task-42-secondary"
	secondRow := &TaskPR{
		WorkspaceID: testWorkspaceID, TaskID: secondTaskID, RepositoryID: "repo-1",
		Owner: "acme", Repo: "widget", PRNumber: 42, PRURL: "https://github.com/acme/widget/pull/42",
		PRTitle: "workflow phase PR", HeadBranch: pr.branch, BaseBranch: "main",
		State: "open", HeadSHA: pr.storedHead, CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, store.CreateTaskPR(context.Background(), secondRow))
	second := &PRWatch{
		WorkspaceID: testWorkspaceID, TaskID: secondTaskID, SessionID: "session-" + secondTaskID,
		RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: 42, Branch: pr.branch,
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	executor.onStep1 = func() {
		once.Do(func() { close(started) })
		<-release
	}
	results := make(chan workflowPhaseResult, 2)
	go func() {
		results <- service.runWorkflowPhase(context.Background(), client, executor, "test-scope", []*PRWatch{first}, false)
	}()
	<-started
	go func() {
		results <- service.runWorkflowPhase(context.Background(), client, executor, "test-scope", []*PRWatch{second}, false)
	}()
	time.Sleep(50 * time.Millisecond)
	close(release)
	firstResult, secondResult := <-results, <-results
	require.Nil(t, firstResult.Deferred)
	require.Nil(t, secondResult.Deferred)
	require.Empty(t, firstResult.Unfinished)
	require.Empty(t, secondResult.Unfinished)
	step1, _ := executor.counts()
	require.Equal(t, 1, step1, "concurrent callers must share one suite read")

	for _, taskID := range []string{first.TaskID, second.TaskID} {
		row, err := store.GetTaskPRByRepoAndNumber(context.Background(), taskID, "repo-1", 42)
		require.NoError(t, err)
		require.NotNil(t, row)
		require.Equal(t, WorkflowAttentionApprovalRequired, row.WorkflowAttention.State,
			"shared evidence must be applied to %s", taskID)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.10
func TestWorkflowPhase_RESTJobDeferralLeavesObservationUntilRetry(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	client := &workflowPhaseCountingClient{MockClient: mockClient}
	client.deferJobs.Store(1)
	pr := forkApprovalTestPR(1, "head-1")
	watch := seedWorkflowPhaseTarget(t, store, 1, pr, nil)
	client.AddPR(&PR{
		Number: 1, State: "open", HeadSHA: "head-1", HeadBranch: pr.branch,
		RepoOwner: "acme", RepoName: "widget",
		HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	})
	client.ReplaceWorkflowRuns("acme", "widget", "head-1", []WorkflowRun{{
		ID: 1001, RunAttempt: 1, WorkflowID: 11, Name: "CI", Event: "pull_request",
		Status: "completed", Conclusion: "action_required",
		HeadSHA: "head-1", HeadBranch: pr.branch,
		HeadRepoOwner: "contributor", HeadRepoName: "widget-fork",
	}})
	client.ReplaceWorkflowRunJobs("acme", "widget", 1001, 1, nil)

	result := service.runWorkflowPhase(context.Background(), client, nil, "test-scope", []*PRWatch{watch}, false)
	require.NotNil(t, result.Deferred, "job admission deferral must remain retryable")
	require.Len(t, result.Unfinished, 1)
	row, err := store.GetTaskPRByRepoAndNumber(context.Background(), watch.TaskID, "repo-1", 1)
	require.NoError(t, err)
	require.Nil(t, row.WorkflowAttention, "a deferred job read must not write stale unknown evidence")

	result = service.runWorkflowPhase(context.Background(), client, nil, "test-scope", []*PRWatch{watch}, false)
	require.Nil(t, result.Deferred)
	require.Empty(t, result.Unfinished)
	row, err = store.GetTaskPRByRepoAndNumber(context.Background(), watch.TaskID, "repo-1", 1)
	require.NoError(t, err)
	require.Equal(t, WorkflowAttentionApprovalRequired, row.WorkflowAttention.State)
}
