package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// pendingWatchFixture serves a paged search window from window and answers
// re-checks by ID from byID, recording every request's filter.
type pendingWatchFixture struct {
	*svcFixture
	watch    *IssueWatch
	window   []LinearIssue
	byID     map[string]LinearIssue
	revisits [][]string
	failNext error
}

func newPendingWatchFixture(t *testing.T) *pendingWatchFixture {
	t.Helper()
	h := &pendingWatchFixture{svcFixture: newSvcFixture(t), byID: map[string]LinearIssue{}}
	ctx := context.Background()
	if _, err := h.svc.SetConfigForWorkspace(ctx, "ws-1", &SetConfigRequest{
		AuthMethod: AuthMethodAPIKey, Secret: "lin_api",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	h.client.searchIssuesFn = func(filter SearchFilter, _ string, _ int) (*SearchResult, error) {
		if len(filter.IssueIDs) == 0 {
			return &SearchResult{Issues: h.window, IsLast: true}, nil
		}
		if filter.TeamKey != "ENG" || !filter.RequireBlockersDone {
			t.Errorf("re-check must keep the watch filter: %#v", filter)
		}
		h.revisits = append(h.revisits, filter.IssueIDs)
		if err := h.failNext; err != nil {
			h.failNext = nil
			return nil, err
		}
		res := &SearchResult{IsLast: true}
		for _, id := range filter.IssueIDs {
			if issue, ok := h.byID[id]; ok {
				res.Issues = append(res.Issues, issue)
			}
		}
		return res, nil
	}
	w, err := h.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{TeamKey: "ENG", RequireBlockersDone: true},
	})
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	h.watch = w
	return h
}

func (h *pendingWatchFixture) poll(t *testing.T) []string {
	t.Helper()
	got, err := h.svc.CheckIssueWatch(context.Background(), h.watch)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	return blockerTestIDs(got)
}

func (h *pendingWatchFixture) pending(t *testing.T) []string {
	t.Helper()
	ids, err := h.store.ListPendingIssueIDs(context.Background(), h.watch.ID)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	return ids
}

func blockedBy(stateType string) []LinearIssueBlocker {
	return []LinearIssueBlocker{{Identifier: "ENG-1", StateType: stateType}}
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.2
func TestService_CheckIssueWatch_ReleasesHeldIssueOutsideSearchWindow(t *testing.T) {
	h := newPendingWatchFixture(t)
	h.window = []LinearIssue{{ID: "id-2", Identifier: "ENG-2", Blockers: blockedBy("started")}}
	if got := h.poll(t); len(got) != 0 {
		t.Fatalf("first poll published %v", got)
	}
	if want := []string{"id-2"}; !reflect.DeepEqual(h.pending(t), want) {
		t.Fatalf("pending = %v, want %v", h.pending(t), want)
	}

	// The held issue has left the paged window; the poll re-checks it by ID.
	h.window = nil
	h.byID["id-2"] = LinearIssue{ID: "id-2", Identifier: "ENG-2", Blockers: blockedBy("started")}
	if got := h.poll(t); len(got) != 0 {
		t.Fatalf("second poll published %v", got)
	}
	if want := [][]string{{"id-2"}}; !reflect.DeepEqual(h.revisits, want) {
		t.Fatalf("re-checks = %v, want %v", h.revisits, want)
	}

	h.byID["id-2"] = LinearIssue{ID: "id-2", Identifier: "ENG-2", Blockers: blockedBy("completed")}
	if got, want := h.poll(t), []string{"ENG-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("third poll published %v, want %v", got, want)
	}
}

// A released issue whose event the in-flight cap defers is not reserved; it
// must stay pending so a later poll publishes it again once a slot frees up.
//
// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.2
func TestService_CheckIssueWatch_KeepsReleasedIssuePendingUntilReserved(t *testing.T) {
	h := newPendingWatchFixture(t)
	h.window = []LinearIssue{{ID: "id-2", Identifier: "ENG-2", Blockers: blockedBy("started")}}
	h.poll(t)
	h.window = nil
	h.byID["id-2"] = LinearIssue{ID: "id-2", Identifier: "ENG-2", Blockers: blockedBy("completed")}
	for poll := 1; poll <= 2; poll++ {
		if got, want := h.poll(t), []string{"ENG-2"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("poll %d published %v, want %v", poll, got, want)
		}
		if want := []string{"id-2"}; !reflect.DeepEqual(h.pending(t), want) {
			t.Fatalf("poll %d: pending = %v, want %v", poll, h.pending(t), want)
		}
	}

	ctx := context.Background()
	if ok, err := h.store.ReserveIssueWatchTask(ctx, h.watch.ID, "ENG-2", ""); err != nil || !ok {
		t.Fatalf("reserve: ok=%v err=%v", ok, err)
	}
	if got := h.poll(t); len(got) != 0 {
		t.Fatalf("reserved issue published again: %v", got)
	}
	if len(h.pending(t)) != 0 {
		t.Fatalf("reserved issue still pending: %v", h.pending(t))
	}
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.2
func TestService_CheckIssueWatch_DropsHeldIssueThatNoLongerMatches(t *testing.T) {
	h := newPendingWatchFixture(t)
	h.window = []LinearIssue{{ID: "id-2", Identifier: "ENG-2", Blockers: blockedBy("started")}}
	h.poll(t)
	h.window = nil
	if got := h.poll(t); len(got) != 0 {
		t.Fatalf("published %v", got)
	}
	if len(h.pending(t)) != 0 {
		t.Fatalf("an issue the filter no longer returns must stop being pending: %v", h.pending(t))
	}
	h.poll(t)
	if len(h.revisits) != 1 {
		t.Fatalf("re-checked %d times, want 1", len(h.revisits))
	}
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.2
func TestService_CheckIssueWatch_KeepsHeldIssuesWhenRecheckFails(t *testing.T) {
	h := newPendingWatchFixture(t)
	h.window = []LinearIssue{{ID: "id-2", Identifier: "ENG-2", Blockers: blockedBy("started")}}
	h.poll(t)
	h.window = []LinearIssue{{ID: "id-3", Identifier: "ENG-3"}}
	h.failNext = errors.New("linear unavailable")
	if got, want := h.poll(t), []string{"ENG-3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("published %v, want %v", got, want)
	}
	if want := []string{"id-2", "id-3"}; !reflect.DeepEqual(h.pending(t), want) {
		t.Fatalf("pending = %v, want %v", h.pending(t), want)
	}

	h.failNext = context.Canceled
	if _, err := h.svc.CheckIssueWatch(context.Background(), h.watch); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled re-check must fail the poll, got %v", err)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.4
func TestService_CheckIssueWatch_RechecksHeldIssuesInBoundedBatches(t *testing.T) {
	h := newPendingWatchFixture(t)
	for i := range maxPendingIssues + 10 {
		id := fmt.Sprintf("id-%03d", i)
		h.window = append(h.window, LinearIssue{ID: id, Identifier: fmt.Sprintf("ENG-%d", i), Blockers: blockedBy("started")})
	}
	h.poll(t)
	if got := len(h.pending(t)); got != maxPendingIssues {
		t.Fatalf("%d pending issues, want the %d cap", got, maxPendingIssues)
	}
	h.window = nil
	h.poll(t)
	if len(h.revisits) != issueWatchMaxPages {
		t.Fatalf("re-check requests = %d, want %d", len(h.revisits), issueWatchMaxPages)
	}
	for _, ids := range h.revisits {
		if len(ids) > issueWatchSearchPageSize {
			t.Fatalf("re-check of %d IDs exceeds the page size", len(ids))
		}
	}
}

func TestStore_PendingIssueIDsFollowWatchLifecycle(t *testing.T) {
	h := newPendingWatchFixture(t)
	ctx := context.Background()
	if err := h.store.ReplacePendingIssueIDs(ctx, h.watch.ID, []string{"b", "a"}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(h.pending(t), want) {
		t.Fatalf("pending = %v, want %v", h.pending(t), want)
	}
	if err := h.store.ResetIssueWatchState(ctx, h.watch.ID); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if len(h.pending(t)) != 0 {
		t.Fatalf("reset must clear pending issues")
	}
	if err := h.store.ReplacePendingIssueIDs(ctx, h.watch.ID, []string{"a"}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := h.store.DeleteIssueWatch(ctx, h.watch.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(h.pending(t)) != 0 {
		t.Fatalf("delete must clear pending issues")
	}
}

// @covers AC-INTEGRATIONS-LINEAR-BLOCKERS-001.4
func TestSearchFilter_IssueIDsAreRequestOnly(t *testing.T) {
	f := SearchFilter{TeamKey: "ENG", IssueIDs: []string{"id-1"}}
	got := buildIssueFilter(f)
	if want := map[string]interface{}{"in": []string{"id-1"}}; !reflect.DeepEqual(got["id"], want) {
		t.Fatalf("id filter = %#v, want %#v", got["id"], want)
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "id-1") {
		t.Fatalf("issue IDs must not be stored with the watch filter: %s", raw)
	}
	var decoded SearchFilter
	if err := json.Unmarshal([]byte(`{"teamKey":"ENG","IssueIDs":["x"],"issueIds":["y"]}`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.IssueIDs) != 0 {
		t.Fatalf("issue IDs must not be read from a request: %v", decoded.IssueIDs)
	}
}
