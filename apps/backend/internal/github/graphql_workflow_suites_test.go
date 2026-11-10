package github

import (
	"encoding/json"
	"strings"
	"testing"
)

func workflowTestJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal workflow fixture: %v", err)
	}
	return encoded
}

func TestGraphQLWorkflowSuites_QueryShapeAndChunking(t *testing.T) {
	refs := []graphQLPRRef{
		{Owner: "acme", Repo: "widget", Number: 1},
		{Owner: "acme", Repo: "widget", Number: 2},
		{Owner: "acme", Repo: "gadget", Number: 3},
	}
	query, _ := buildWorkflowSuitesQuery(refs)
	for _, fragment := range []string{
		"query WorkflowSuites", "repo0: repository(owner: \"acme\", name: \"gadget\")",
		"repo1: repository(owner: \"acme\", name: \"widget\")", "wp0: pullRequest(number: 1)",
		"checkSuites(first: 100)", "workflowRun { databaseId event url workflow { databaseId name } }",
		"headRepository { name nameWithOwner url owner { login } }",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("workflow suites query missing %q:\n%s", fragment, query)
		}
	}
	if strings.Contains(query, "checkRuns(") {
		t.Fatalf("step 1 must not select checkRuns:\n%s", query)
	}

	chunks := chunkRefs(refs, graphQLWorkflowSuitesChunkSize)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want one for three refs", len(chunks))
	}
	many := make([]graphQLPRRef, graphQLWorkflowSuitesChunkSize+1)
	for index := range many {
		many[index] = graphQLPRRef{Owner: "acme", Repo: "widget", Number: index + 1}
	}
	if chunks = chunkRefs(many, graphQLWorkflowSuitesChunkSize); len(chunks) != 2 {
		t.Fatalf("chunks = %d, want two for %d refs", len(chunks), len(many))
	}

	jobsQuery, _ := buildWorkflowSuiteJobsQuery([]string{"CS_1", "CS_2"})
	for _, fragment := range []string{
		"query WorkflowSuiteJobs", `nodes(ids: ["CS_1", "CS_2"])`,
		"checkRuns(first: 0, filterBy: {checkType: LATEST}) { totalCount }",
	} {
		if !strings.Contains(jobsQuery, fragment) {
			t.Fatalf("workflow jobs query missing %q:\n%s", fragment, jobsQuery)
		}
	}
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.1
// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.10
func TestGraphQLWorkflowSuites_DecodesForkProbeFixture(t *testing.T) {
	suites := make([]any, 0, 19)
	for index := 0; index < 10; index++ {
		suites = append(suites, map[string]any{
			"id": "CS_APPROVAL_" + string(rune('A'+index)), "status": "COMPLETED", "conclusion": "ACTION_REQUIRED",
			"createdAt": "2026-10-29T10:00:00Z",
			"workflowRun": map[string]any{
				"databaseId": 100 + index, "event": "pull_request",
				"url":      "https://github.com/acme/widget/actions/runs/" + string(rune('1'+index)),
				"workflow": map[string]any{"databaseId": 10 + index, "name": "CI " + string(rune('A'+index))},
			},
		})
	}
	suites = append(suites, map[string]any{
		"id": "CS_WITHOUT_RUN", "status": "COMPLETED", "conclusion": "ACTION_REQUIRED",
		"createdAt": "2026-10-29T10:00:00Z",
	})
	for index := 0; index < 8; index++ {
		suites = append(suites, map[string]any{
			"id": "CS_SUCCESS_" + string(rune('A'+index)), "status": "COMPLETED", "conclusion": "SUCCESS",
			"createdAt": "2026-10-29T10:00:00Z",
			"workflowRun": map[string]any{
				"databaseId": 200 + index, "event": "push",
				"url":      "https://github.com/acme/widget/actions/runs/200",
				"workflow": map[string]any{"databaseId": 20 + index, "name": "CI success"},
			},
		})
	}
	node := map[string]any{
		"headRefOid": "head-sha", "headRefName": "feature/approval",
		"headRepository": map[string]any{
			"name": "widget-fork", "nameWithOwner": "contributor/widget-fork",
			"url": "https://github.com/contributor/widget-fork", "owner": map[string]any{"login": "contributor"},
		},
		"headRepositoryOwner": map[string]any{"login": "contributor"},
		"commits": map[string]any{"nodes": []any{map[string]any{"commit": map[string]any{
			"oid": "head-sha",
			"checkSuites": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false},
				"nodes":    suites,
			},
		}}}},
	}
	out := workflowSuitesResponse{Data: map[string]json.RawMessage{
		"repo0":     workflowTestJSON(t, map[string]any{"wp0": node}),
		"rateLimit": workflowTestJSON(t, map[string]any{"limit": 5000, "remaining": 4999, "resetAt": "2030-01-01T00:00:00Z", "cost": 1}),
	}}
	observations, retryAt, err := decodeWorkflowSuitesResponse(
		out, []graphQLPRRef{{Owner: "acme", Repo: "widget", Number: 4720}},
	)
	if err != nil {
		t.Fatalf("decodeWorkflowSuitesResponse: %v", err)
	}
	if retryAt != nil {
		t.Fatalf("retryAt = %v, want nil while quota remains", retryAt)
	}
	observation, ok := observations[workflowSuiteTargetKey("acme", "widget", 4720)]
	if !ok {
		t.Fatal("decoded observation missing the requested PR")
	}
	if observation.Missing || observation.HasNextPage || observation.HeadSHA != "head-sha" || observation.CommitSHA != "head-sha" {
		t.Fatalf("observation head = %#v", observation)
	}
	if observation.HeadRepoOwner != "contributor" || observation.HeadRepoName != "widget-fork" {
		t.Fatalf("head repository = %s/%s, want contributor/widget-fork",
			observation.HeadRepoOwner, observation.HeadRepoName)
	}
	if len(observation.Runs) != 18 {
		t.Fatalf("decoded runs = %d, want 18 suites with an Actions run", len(observation.Runs))
	}
	if len(observation.SuiteIDs) != 10 {
		t.Fatalf("job-count suites = %d, want the ten jobless approval candidates", len(observation.SuiteIDs))
	}
}

func TestGraphQLWorkflowSuiteJobs_DecodesCountsAndMissingNodes(t *testing.T) {
	ids := []string{"CS_ZERO", "CS_MISSING", "CS_JOBS"}
	out := workflowSuiteJobsResponse{Data: map[string]json.RawMessage{
		"nodes": workflowTestJSON(t, []any{
			map[string]any{"checkRuns": map[string]any{"totalCount": 0}},
			nil,
			map[string]any{"checkRuns": map[string]any{"totalCount": 3}},
		}),
	}}
	counts, missing, _, err := decodeWorkflowSuiteJobCounts(out, ids)
	if err != nil {
		t.Fatalf("decodeWorkflowSuiteJobCounts: %v", err)
	}
	if counts["CS_ZERO"] != 0 || counts["CS_JOBS"] != 3 {
		t.Fatalf("counts = %#v", counts)
	}
	if _, ok := missing["CS_MISSING"]; !ok {
		t.Fatalf("missing nodes = %#v, want CS_MISSING", missing)
	}
}

func TestGraphQLWorkflowSuites_PartialErrorsKeepResolvedAliases(t *testing.T) {
	node := map[string]any{
		"headRefOid": "head-sha", "headRefName": "feature",
		"headRepository": map[string]any{
			"name": "widget-fork", "nameWithOwner": "contributor/widget-fork",
			"url": "https://github.com/contributor/widget-fork", "owner": map[string]any{"login": "contributor"},
		},
		"headRepositoryOwner": map[string]any{"login": "contributor"},
		"commits": map[string]any{"nodes": []any{map[string]any{"commit": map[string]any{
			"oid": "head-sha",
			"checkSuites": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false},
				"nodes": []any{map[string]any{
					"id": "CS_1", "status": "COMPLETED", "conclusion": "ACTION_REQUIRED",
					"createdAt": "2026-10-29T00:00:00Z",
					"workflowRun": map[string]any{
						"databaseId": 1, "event": "pull_request",
						"url":      "https://github.com/acme/widget/actions/runs/1",
						"workflow": map[string]any{"databaseId": 10, "name": "CI"},
					},
				}},
			},
		}}}},
	}
	out := workflowSuitesResponse{
		Data: map[string]json.RawMessage{
			"repo0": workflowTestJSON(t, map[string]any{"wp0": node, "wp1": nil}),
		},
		Errors: []graphQLError{{
			Message: "Could not resolve to a PullRequest with the number of 1.",
			Path:    []string{"repo0", "wp0"},
		}},
	}
	observations, _, err := decodeWorkflowSuitesResponse(out, []graphQLPRRef{
		{Owner: "acme", Repo: "widget", Number: 1},
		{Owner: "acme", Repo: "widget", Number: 2},
	})
	if err != nil {
		t.Fatalf("decodeWorkflowSuitesResponse: %v", err)
	}
	failed := observations[workflowSuiteTargetKey("acme", "widget", 1)]
	if !failed.Missing {
		t.Fatalf("failed alias = %#v, want Missing despite resolved data", failed)
	}
	if len(observations) != 2 {
		t.Fatalf("observations = %d, want a partial decode for both aliases", len(observations))
	}
}

func TestGraphQLWorkflowSuiteJobs_PartialErrorsKeepResolvedNodes(t *testing.T) {
	ids := []string{"CS_REFUSED", "CS_OK"}
	out := workflowSuiteJobsResponse{
		Data: map[string]json.RawMessage{
			"nodes": workflowTestJSON(t, []any{
				map[string]any{"checkRuns": map[string]any{"totalCount": 0}},
				map[string]any{"checkRuns": map[string]any{"totalCount": 0}},
			}),
		},
		Errors: []graphQLError{{
			Message: "Could not resolve to a CheckSuite with the global id of CS_REFUSED.",
			Path:    []string{"nodes", "0"},
		}},
	}
	counts, missing, _, err := decodeWorkflowSuiteJobCounts(out, ids)
	if err != nil {
		t.Fatalf("decodeWorkflowSuiteJobCounts: %v", err)
	}
	if _, ok := missing["CS_REFUSED"]; !ok {
		t.Fatalf("missing = %#v, want CS_REFUSED", missing)
	}
	if _, ok := counts["CS_REFUSED"]; ok {
		t.Fatalf("counts = %#v, refused suite must not become a zero-job claim", counts)
	}
	if count, ok := counts["CS_OK"]; !ok || count != 0 {
		t.Fatalf("counts = %#v, want the resolved zero count for CS_OK", counts)
	}
}

func TestGraphQLWorkflowSuites_UnattributableErrorFailsChunk(t *testing.T) {
	out := workflowSuitesResponse{
		Data: map[string]json.RawMessage{
			"repo0": workflowTestJSON(t, map[string]any{"wp0": nil}),
		},
		Errors: []graphQLError{{Message: "schema mismatch"}},
	}
	if _, _, err := decodeWorkflowSuitesResponse(out, []graphQLPRRef{{
		Owner: "acme", Repo: "widget", Number: 1,
	}}); err == nil {
		t.Fatal("an error without a usable path must fail the whole chunk")
	}
}

func TestGraphQLWorkflowSuiteJobs_DecodesNumericErrorPath(t *testing.T) {
	var out workflowSuiteJobsResponse
	payload := `{"data":{"nodes":[{"checkRuns":{"totalCount":0}},{"checkRuns":{"totalCount":0}}]},` +
		`"errors":[{"message":"Could not resolve to a CheckSuite","path":["nodes",1]}]}`
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		t.Fatalf("decode jobs response with numeric path: %v", err)
	}
	counts, missing, _, err := decodeWorkflowSuiteJobCounts(out, []string{"CS_0", "CS_1"})
	if err != nil {
		t.Fatalf("decodeWorkflowSuiteJobCounts: %v", err)
	}
	if _, ok := counts["CS_0"]; !ok {
		t.Fatalf("counts = %#v, want the resolved sibling", counts)
	}
	if _, ok := missing["CS_1"]; !ok {
		t.Fatalf("missing = %#v, want the numeric-path node", missing)
	}
}
