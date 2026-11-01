package linear

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func writeSearchPage(t *testing.T, w http.ResponseWriter, nodes []map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{"data": map[string]any{"issues": map[string]any{
		"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": false},
	}}}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Errorf("encode search page: %v", err)
	}
}

func relation(relType, identifier, stateType string) map[string]any {
	return map[string]any{
		"type":  relType,
		"issue": map[string]any{"identifier": identifier, "state": map[string]any{"type": stateType}},
	}
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.4
func TestGraphQLClient_SearchIssues_BlockersOnlyWhenRequested(t *testing.T) {
	var queries []string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		req := readReq(t, r)
		queries = append(queries, req["query"].(string))
		vars := req["variables"].(map[string]any)
		if _, ok := vars["filter"].(map[string]any)["requireBlockersDone"]; ok {
			t.Errorf("requireBlockersDone leaked into the Linear filter: %v", vars["filter"])
		}
		writeSearchPage(t, w, []map[string]any{{"identifier": "ENG-1"}})
	})
	c := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL)
	ctx := context.Background()
	if _, err := c.SearchIssues(ctx, SearchFilter{TeamKey: "ENG"}, "", 50); err != nil {
		t.Fatalf("plain search: %v", err)
	}
	if _, err := c.SearchIssues(ctx, SearchFilter{TeamKey: "ENG", RequireBlockersDone: true}, "", 50); err != nil {
		t.Fatalf("blocker search: %v", err)
	}
	if strings.Contains(queries[0], "inverseRelations") {
		t.Fatalf("plain search must not request relations:\n%s", queries[0])
	}
	if !strings.Contains(queries[1], "inverseRelations(first: 25)") {
		t.Fatalf("blocker search must request bounded relations:\n%s", queries[1])
	}
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.1
// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.3
func TestGraphQLClient_SearchIssues_MapsBlockers(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeSearchPage(t, w, []map[string]any{
			{"identifier": "ENG-2", "inverseRelations": map[string]any{
				"nodes": []map[string]any{
					relation("blocks", "ENG-1", "completed"),
					relation("related", "ENG-9", "started"),
					relation("blocks", "OPS-4", "started"),
				},
				"pageInfo": map[string]any{"hasNextPage": false},
			}},
			{"identifier": "ENG-3", "inverseRelations": map[string]any{
				"nodes":    []map[string]any{},
				"pageInfo": map[string]any{"hasNextPage": true},
			}},
		})
	})
	res, err := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL).
		SearchIssues(context.Background(), SearchFilter{RequireBlockersDone: true}, "", 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	want := []LinearIssueBlocker{{Identifier: "ENG-1", StateType: "completed"}, {Identifier: "OPS-4", StateType: "started"}}
	if !reflect.DeepEqual(res.Issues[0].Blockers, want) || res.Issues[0].BlockersTruncated {
		t.Fatalf("ENG-2 blockers = %#v truncated=%v", res.Issues[0].Blockers, res.Issues[0].BlockersTruncated)
	}
	if !res.Issues[1].BlockersTruncated {
		t.Fatalf("ENG-3 must report a truncated relation list")
	}
}

func TestLinearIssueBlocker_Done(t *testing.T) {
	for stateType, want := range map[string]bool{
		"completed": true, "canceled": false, "started": false, "unstarted": false,
		"backlog": false, "triage": false, "": false,
	} {
		if got := (LinearIssueBlocker{StateType: stateType}).Done(); got != want {
			t.Errorf("Done(%q) = %v, want %v", stateType, got, want)
		}
	}
}

func newBlockerWatchFixture(t *testing.T, require bool, sortBy IssueSortBy, pages [][]LinearIssue) (*svcFixture, *IssueWatch, *[]SearchFilter) {
	t.Helper()
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SetConfigForWorkspace(ctx, "ws-1", &SetConfigRequest{
		AuthMethod: AuthMethodAPIKey, Secret: "lin_api",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	var filters []SearchFilter
	f.client.searchIssuesFn = func(filter SearchFilter, pageToken string, _ int) (*SearchResult, error) {
		filters = append(filters, filter)
		page := 0
		if pageToken != "" {
			page = int(pageToken[0] - '0')
		}
		res := &SearchResult{Issues: pages[page], IsLast: page == len(pages)-1}
		if !res.IsLast {
			res.NextPageToken = string(rune('0' + page + 1))
		}
		return res, nil
	}
	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{TeamKey: "ENG", RequireBlockersDone: require},
		SortBy: sortBy,
	})
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	return f, w, &filters
}

func blockerTestIDs(issues []*LinearIssue) []string {
	out := make([]string, 0, len(issues))
	for _, issue := range issues {
		out = append(out, issue.Identifier)
	}
	return out
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.1
// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.2
// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.3
func TestService_CheckIssueWatch_HoldsIssuesWithOpenBlockers(t *testing.T) {
	f, w, filters := newBlockerWatchFixture(t, true, SortByCreatedAsc, [][]LinearIssue{{
		{Identifier: "ENG-1", Created: "2026-01-01T00:00:00Z"},
		{Identifier: "ENG-2", Created: "2026-01-02T00:00:00Z", Blockers: []LinearIssueBlocker{{Identifier: "ENG-1", StateType: "started"}}},
		{Identifier: "ENG-3", Created: "2026-01-03T00:00:00Z", Blockers: []LinearIssueBlocker{{Identifier: "ENG-0", StateType: "completed"}}},
		{Identifier: "ENG-4", Created: "2026-01-04T00:00:00Z", Blockers: []LinearIssueBlocker{{Identifier: "ENG-0", StateType: "canceled"}}},
		{Identifier: "ENG-5", Created: "2026-01-05T00:00:00Z", Blockers: []LinearIssueBlocker{{Identifier: "OPS-7", StateType: "backlog"}}},
		{Identifier: "ENG-6", Created: "2026-01-06T00:00:00Z", BlockersTruncated: true},
	}})
	got, err := f.svc.CheckIssueWatch(context.Background(), w)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if want := []string{"ENG-1", "ENG-3"}; !reflect.DeepEqual(blockerTestIDs(got), want) {
		t.Fatalf("published %v, want %v", blockerTestIDs(got), want)
	}
	if !(*filters)[0].RequireBlockersDone {
		t.Fatalf("search must ask the client for blockers")
	}
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.2
func TestService_CheckIssueWatch_ReleasesIssueOnceBlockerCompletes(t *testing.T) {
	blocked := []LinearIssue{{Identifier: "ENG-2", Blockers: []LinearIssueBlocker{{Identifier: "ENG-1", StateType: "started"}}}}
	f, w, _ := newBlockerWatchFixture(t, true, SortByDefault, [][]LinearIssue{blocked})
	ctx := context.Background()
	got, err := f.svc.CheckIssueWatch(ctx, w)
	if err != nil || len(got) != 0 {
		t.Fatalf("first poll: got %v err %v", blockerTestIDs(got), err)
	}
	blocked[0].Blockers[0].StateType = "completed"
	got, err = f.svc.CheckIssueWatch(ctx, w)
	if err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if want := []string{"ENG-2"}; !reflect.DeepEqual(blockerTestIDs(got), want) {
		t.Fatalf("second poll published %v, want %v", blockerTestIDs(got), want)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.2
func TestService_CheckIssueWatch_BlockerWatchPagesPastBlockedIssues(t *testing.T) {
	open := []LinearIssueBlocker{{Identifier: "ENG-0", StateType: "started"}}
	pages := [][]LinearIssue{
		{{Identifier: "ENG-1", Blockers: open}, {Identifier: "ENG-2", Blockers: open}},
		{{Identifier: "ENG-3"}},
	}
	f, w, filters := newBlockerWatchFixture(t, true, SortByDefault, pages)
	got, err := f.svc.CheckIssueWatch(context.Background(), w)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if want := []string{"ENG-3"}; !reflect.DeepEqual(blockerTestIDs(got), want) {
		t.Fatalf("published %v, want %v", blockerTestIDs(got), want)
	}
	if len(*filters) != 2 {
		t.Fatalf("searched %d pages, want 2", len(*filters))
	}
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.5
func TestService_CheckIssueWatch_WithoutSettingIgnoresBlockers(t *testing.T) {
	open := []LinearIssueBlocker{{Identifier: "ENG-0", StateType: "started"}}
	pages := [][]LinearIssue{{{Identifier: "ENG-1", Blockers: open}}, {{Identifier: "ENG-2"}}}
	f, w, filters := newBlockerWatchFixture(t, false, SortByDefault, pages)
	got, err := f.svc.CheckIssueWatch(context.Background(), w)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if want := []string{"ENG-1"}; !reflect.DeepEqual(blockerTestIDs(got), want) {
		t.Fatalf("published %v, want %v", blockerTestIDs(got), want)
	}
	if len(*filters) != 1 || (*filters)[0].RequireBlockersDone {
		t.Fatalf("default watch must keep its single plain search: %#v", *filters)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.5
func TestService_IssueWatch_RequireBlockersDoneIsNotAFilterCondition(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	_, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{RequireBlockersDone: true},
	})
	if err == nil {
		t.Fatalf("a filter with only requireBlockersDone must be rejected as empty")
	}
	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{TeamKey: "ENG", RequireBlockersDone: true},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	stored, err := f.store.GetIssueWatch(ctx, w.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !stored.Filter.RequireBlockersDone {
		t.Fatalf("requireBlockersDone not persisted")
	}
}
