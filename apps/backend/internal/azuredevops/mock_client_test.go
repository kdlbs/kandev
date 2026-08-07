package azuredevops

import (
	"context"
	"testing"
)

func TestMockClientSeedsWorkspaceReadState(t *testing.T) {
	mock := NewMockClient()
	mock.Seed(MockState{
		Authenticated: true,
		User:          TestConnectionResult{OK: true, ID: "me", DisplayName: "Ada"},
		Projects:      []Project{{ID: "p1", Name: "Platform"}},
		WorkItems:     []WorkItem{{ID: 101, Title: "Fix build"}},
		PullRequests:  []PullRequest{{ID: 42, ProjectID: "p1", RepositoryID: "r1", Title: "Ship it"}},
		Feedback:      map[int]PullRequestFeedback{42: {ReviewState: "approved"}},
	})

	auth, err := mock.TestAuth(context.Background())
	if err != nil || !auth.OK || auth.ID != "me" {
		t.Fatalf("auth = %+v, %v", auth, err)
	}
	projects, _ := mock.ListProjects(context.Background())
	work, _ := mock.QueryWIQL(context.Background(), "p1", "SELECT", 20)
	prs, _ := mock.ListPullRequests(context.Background(), PullRequestFilter{ProjectID: "p1", RepositoryID: "r1"})
	feedback, ok := mock.Feedback(42)
	if len(projects) != 1 || len(work.Items) != 1 || len(prs.Items) != 1 || !ok || feedback.ReviewState != "approved" {
		t.Fatalf("mock state projects=%+v work=%+v prs=%+v feedback=%+v", projects, work, prs, feedback)
	}
}

func TestMockClientProvidesWorkItemDetailDiscussionAndIdentity(t *testing.T) {
	mock := NewMockClient()
	mock.Seed(MockState{
		Authenticated: true,
		User:          TestConnectionResult{OK: true, ID: "me", DisplayName: "Ada", Email: "ada@example.com"},
		WorkItems: []WorkItem{{
			ID: 101, Description: "<p>Useful</p><script>alert('xss')</script>",
			Fields: map[string]any{"Microsoft.VSTS.Scheduling.Effort": 5},
		}},
		WorkItemComments: map[int][]WorkItemComment{101: {{ID: 2, Content: "Newest"}, {ID: 1, Content: "Older"}}},
	})

	detail, err := mock.GetWorkItemDetail(t.Context(), "p1", 101)
	if err != nil || detail.Description != "Useful" || len(detail.PlanningFields) != 1 {
		t.Fatalf("detail = %+v, %v", detail, err)
	}
	comments, err := mock.ListWorkItemComments(t.Context(), "p1", 101, "opaque")
	if err != nil || len(comments.Comments) != 2 || comments.Comments[0].ID != 2 {
		t.Fatalf("comments = %+v, %v", comments, err)
	}
	identity, err := mock.GetCurrentIdentity(t.Context())
	if err != nil || identity.ID != "me" || identity.UniqueName != "ada@example.com" {
		t.Fatalf("identity = %+v, %v", identity, err)
	}
}

func TestMockClientPaginatesPullRequests(t *testing.T) {
	mock := NewMockClient()
	mock.Seed(MockState{PullRequests: []PullRequest{
		{ID: 1, ProjectID: "p1", Status: activePullRequestState},
		{ID: 2, ProjectID: "p1", Status: activePullRequestState},
	}})

	page, err := mock.ListPullRequests(t.Context(), PullRequestFilter{
		ProjectID: "p1", Status: activePullRequestState, Skip: 1, Top: 1,
	})
	if err != nil {
		t.Fatalf("list pull requests: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 2 {
		t.Fatalf("items = %#v, want second pull request only", page.Items)
	}
}

func TestMockClientBoardReads(t *testing.T) {
	mock := NewMockClient()
	mock.Seed(MockState{
		Teams:  []Team{{ID: "team-1", Name: "Platform Team", ProjectID: "p1"}},
		Boards: []BoardReference{{ID: "board-1", Name: "Stories"}},
		BoardSnapshots: map[string]BoardSnapshot{
			"board-1": {
				Board: Board{
					ID:      "board-1",
					Columns: []BoardColumn{{ID: "todo", Name: "Original column"}},
					Rows:    []BoardRow{{ID: "row-1", Name: "Original row"}},
				},
				Items: []BoardWorkItem{{
					WorkItem: WorkItem{
						ID: 101,
						Fields: map[string]any{
							"System.Title":  "Original",
							"Custom.Nested": map[string]any{"labels": []any{"initial-label"}},
						},
						Tags: []string{"initial"},
					},
				}},
			},
		},
	})
	teams, err := mock.ListTeams(t.Context(), "p1")
	if err != nil || len(teams) != 1 {
		t.Fatalf("teams = %+v, %v", teams, err)
	}
	boards, err := mock.ListBoards(t.Context(), "p1", "team-1")
	if err != nil || len(boards) != 1 || boards[0].ID != "board-1" {
		t.Fatalf("boards = %+v, %v", boards, err)
	}
	snapshot, err := mock.GetBoardSnapshot(t.Context(), "p1", "team-1", "board-1")
	if err != nil || snapshot == nil || len(snapshot.Items) != 1 {
		t.Fatalf("snapshot = %+v, %v", snapshot, err)
	}
	snapshot.Items[0].Title = "Mutated caller copy"
	snapshot.Items[0].Fields["System.Title"] = "Mutated field"
	snapshot.Items[0].Tags[0] = "mutated-tag"
	snapshot.Board.Columns[0].Name = "Mutated column"
	snapshot.Board.Rows[0].Name = "Mutated row"
	snapshot.Items[0].Fields["Custom.Nested"].(map[string]any)["labels"].([]any)[0] = "mutated-label"
	again, err := mock.GetBoardSnapshot(t.Context(), "p1", "team-1", "board-1")
	if err != nil || again.Items[0].Title == "Mutated caller copy" {
		t.Fatalf("subsequent snapshot = %+v, %v", again, err)
	}
	if again.Items[0].Fields["System.Title"] != "Original" || again.Items[0].Tags[0] != "initial" {
		t.Fatalf("subsequent snapshot mutable data = %+v", again.Items[0])
	}
	if again.Board.Columns[0].Name != "Original column" || again.Board.Rows[0].Name != "Original row" {
		t.Fatalf("subsequent snapshot board data = %+v", again.Board)
	}
	if got := again.Items[0].Fields["Custom.Nested"].(map[string]any)["labels"].([]any)[0]; got != "initial-label" {
		t.Fatalf("subsequent snapshot nested field = %q, want initial-label", got)
	}
}

func TestMockClientUpdateBoardWorkItem(t *testing.T) {
	mock := NewMockClient()
	action := "assign_current_user"
	mock.Seed(MockState{
		Authenticated:  true,
		User:           TestConnectionResult{OK: true, ID: "me", DisplayName: "Ada"},
		BoardSnapshots: map[string]BoardSnapshot{"board-1": {Board: Board{ID: "board-1"}, Items: []BoardWorkItem{{WorkItem: WorkItem{ID: 101, Revision: 7, Title: "Original"}}}}},
	})
	item, err := mock.UpdateBoardWorkItem(t.Context(), "p1", "team-1", "board-1", 101, BoardWorkItemUpdateRequest{Revision: 7, AssigneeAction: &action})
	if err != nil || item == nil || item.AssignedTo != "Ada" || item.Revision != 8 {
		t.Fatalf("updated item = %+v, %v", item, err)
	}
}

func TestMockClientUpdateBoardWorkItemInitializesFieldsForDoneOnlyUpdate(t *testing.T) {
	mock := NewMockClient()
	done := true
	mock.Seed(MockState{
		BoardSnapshots: map[string]BoardSnapshot{"board-1": {
			Board: Board{ID: "board-1", Fields: BoardFields{DoneField: FieldReference{ReferenceName: "System.BoardDone"}}},
			Items: []BoardWorkItem{{WorkItem: WorkItem{ID: 101, Revision: 7}}},
		}},
	})

	item, err := mock.UpdateBoardWorkItem(t.Context(), "p1", "team-1", "board-1", 101, BoardWorkItemUpdateRequest{Revision: 7, ColumnDone: &done})
	if err != nil || item == nil || !item.ColumnDone || item.Fields["System.BoardDone"] != true {
		t.Fatalf("updated item = %+v, %v", item, err)
	}
}

func TestMockClientBoardUpdateKeepsWorkItemReadsInSync(t *testing.T) {
	mock := NewMockClient()
	action := "assign_current_user"
	mock.Seed(MockState{
		Authenticated: true,
		User:          TestConnectionResult{OK: true, ID: "me", DisplayName: "Ada"},
		BoardSnapshots: map[string]BoardSnapshot{"board-1": {
			Board: Board{ID: "board-1"},
			Items: []BoardWorkItem{{WorkItem: WorkItem{ID: 101, Revision: 7, Title: "Original"}}},
		}},
		WorkItems: []WorkItem{{ID: 101, Revision: 7, Title: "Original"}},
	})

	if _, err := mock.UpdateBoardWorkItem(t.Context(), "p1", "team-1", "board-1", 101, BoardWorkItemUpdateRequest{Revision: 7, AssigneeAction: &action}); err != nil {
		t.Fatalf("update board item: %v", err)
	}
	item, err := mock.GetWorkItem(t.Context(), "p1", 101)
	if err != nil || item.AssignedTo != "Ada" || item.Revision != 8 {
		t.Fatalf("work item = %+v, %v", item, err)
	}
}
