package runtime

import (
	"errors"
	"net/url"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func TestParseListTasksQuery_EmptyQueryDefaultsToDescByUpdatedAt(t *testing.T) {
	opts, err := parseListTasksQuery(url.Values{})
	if err != nil {
		t.Fatalf("parseListTasksQuery: %v", err)
	}
	if len(opts.Status) != 0 || len(opts.Priority) != 0 {
		t.Fatalf("filters = %+v, want none", opts)
	}
	if !opts.SortDesc {
		t.Fatalf("SortDesc = false, want true (default)")
	}
	if opts.SortField != "" {
		t.Fatalf("SortField = %q, want empty (caller defaults it)", opts.SortField)
	}
}

func TestParseListTasksQuery_EmptyValueMeansAbsentForSingleValued(t *testing.T) {
	opts, err := parseListTasksQuery(url.Values{"assignee": {""}, "project": {""}})
	if err != nil {
		t.Fatalf("parseListTasksQuery: %v", err)
	}
	if opts.AssigneeID != "" || opts.ProjectID != "" {
		t.Fatalf("opts = %+v, want both empty", opts)
	}
}

func TestParseListTasksQuery_EmptyValueDroppedFromRepeatable(t *testing.T) {
	opts, err := parseListTasksQuery(url.Values{"status": {"TODO", ""}})
	if err != nil {
		t.Fatalf("parseListTasksQuery: %v", err)
	}
	if len(opts.Status) != 1 || opts.Status[0] != "TODO" {
		t.Fatalf("Status = %v, want [TODO]", opts.Status)
	}
}

func TestParseListTasksQuery_RepeatedSingleValuedParamIsInvalid(t *testing.T) {
	_, err := parseListTasksQuery(url.Values{"assignee": {"agent-1", "agent-2"}})
	if !errors.Is(err, ErrInvalidListParams) {
		t.Fatalf("error = %v, want ErrInvalidListParams", err)
	}
}

func TestParseListTasksQuery_RepeatableParamUnion(t *testing.T) {
	opts, err := parseListTasksQuery(url.Values{
		"status":   {"TODO", "IN_PROGRESS"},
		"priority": {"high", "medium"},
	})
	if err != nil {
		t.Fatalf("parseListTasksQuery: %v", err)
	}
	if len(opts.Status) != 2 || opts.Status[0] != "TODO" || opts.Status[1] != "IN_PROGRESS" {
		t.Fatalf("Status = %v", opts.Status)
	}
	if len(opts.Priority) != 2 || opts.Priority[0] != "high" || opts.Priority[1] != "medium" {
		t.Fatalf("Priority = %v", opts.Priority)
	}
}

func TestParseListTasksQuery_SortFieldExactEnumMatchCaseSensitive(t *testing.T) {
	for _, valid := range []string{"updated_at", "created_at", "priority"} {
		opts, err := parseListTasksQuery(url.Values{"sort": {valid}})
		if err != nil {
			t.Fatalf("sort=%s: %v", valid, err)
		}
		if string(opts.SortField) != valid {
			t.Fatalf("SortField = %q, want %q", opts.SortField, valid)
		}
	}
	if _, err := parseListTasksQuery(url.Values{"sort": {"Updated_At"}}); !errors.Is(err, ErrInvalidListParams) {
		t.Fatalf("case-mismatched sort: error = %v, want ErrInvalidListParams", err)
	}
	if _, err := parseListTasksQuery(url.Values{"sort": {"bogus"}}); !errors.Is(err, ErrInvalidListParams) {
		t.Fatalf("unknown sort: error = %v, want ErrInvalidListParams", err)
	}
}

func TestParseListTasksQuery_OrderAscOverridesDefaultDesc(t *testing.T) {
	opts, err := parseListTasksQuery(url.Values{"order": {"asc"}})
	if err != nil {
		t.Fatalf("parseListTasksQuery: %v", err)
	}
	if opts.SortDesc {
		t.Fatalf("SortDesc = true, want false for order=asc")
	}
}

func TestParseListTasksQuery_OrderInvalidValueRejected(t *testing.T) {
	if _, err := parseListTasksQuery(url.Values{"order": {"descending"}}); !errors.Is(err, ErrInvalidListParams) {
		t.Fatalf("error = %v, want ErrInvalidListParams", err)
	}
}

func TestParseListTasksQuery_LimitParsesInteger(t *testing.T) {
	opts, err := parseListTasksQuery(url.Values{"limit": {"50"}})
	if err != nil {
		t.Fatalf("parseListTasksQuery: %v", err)
	}
	if opts.Limit != 50 {
		t.Fatalf("Limit = %d, want 50", opts.Limit)
	}
}

func TestParseListTasksQuery_LimitNonIntegerRejected(t *testing.T) {
	if _, err := parseListTasksQuery(url.Values{"limit": {"fifty"}}); !errors.Is(err, ErrInvalidListParams) {
		t.Fatalf("error = %v, want ErrInvalidListParams", err)
	}
}

func TestParseListTasksQuery_CursorAndCursorIDMustBothBePresentOrAbsent(t *testing.T) {
	if _, err := parseListTasksQuery(url.Values{}); err != nil {
		t.Fatalf("neither present: %v", err)
	}
	if _, err := parseListTasksQuery(url.Values{"cursor": {"v1"}, "cursor_id": {"task-1"}}); err != nil {
		t.Fatalf("both present: %v", err)
	}
	if _, err := parseListTasksQuery(url.Values{"cursor": {"v1"}}); !errors.Is(err, ErrInvalidListParams) {
		t.Fatalf("cursor only: error = %v, want ErrInvalidListParams", err)
	}
	if _, err := parseListTasksQuery(url.Values{"cursor_id": {"task-1"}}); !errors.Is(err, ErrInvalidListParams) {
		t.Fatalf("cursor_id only: error = %v, want ErrInvalidListParams", err)
	}
}

func TestParseListTasksQuery_CursorValuesPassThroughVerbatim(t *testing.T) {
	opts, err := parseListTasksQuery(url.Values{"cursor": {"2026-01-01"}, "cursor_id": {"task-9"}})
	if err != nil {
		t.Fatalf("parseListTasksQuery: %v", err)
	}
	if opts.CursorValue != "2026-01-01" || opts.CursorID != "task-9" {
		t.Fatalf("cursor = %q/%q", opts.CursorValue, opts.CursorID)
	}
}

func TestParseListTasksQuery_IncludeSystemBooleanStrict(t *testing.T) {
	opts, err := parseListTasksQuery(url.Values{"include_system": {"true"}})
	if err != nil || !opts.IncludeSystem {
		t.Fatalf("include_system=true: opts=%+v err=%v", opts, err)
	}
	opts, err = parseListTasksQuery(url.Values{"include_system": {"false"}})
	if err != nil || opts.IncludeSystem {
		t.Fatalf("include_system=false: opts=%+v err=%v", opts, err)
	}
	if _, err := parseListTasksQuery(url.Values{"include_system": {"1"}}); !errors.Is(err, ErrInvalidListParams) {
		t.Fatalf("include_system=1: error = %v, want ErrInvalidListParams", err)
	}
}

func TestToTaskListItems_MapsAllFields(t *testing.T) {
	rows := []*sqlite.TaskRow{{
		ID: "task-1", WorkspaceID: "ws-1", Identifier: "KAN-1", Title: "Title",
		Description: "Desc", Status: "TODO", Priority: "high", ParentID: "task-0",
		ProjectID: "proj-1", AssigneeAgentProfileID: "agent-1", Labels: `["a"]`,
		CreatedAt: "2026-01-01", UpdatedAt: "2026-01-02", IsSystem: true,
	}}
	items := toTaskListItems(rows)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	got := items[0]
	want := TaskListItem{
		ID: "task-1", WorkspaceID: "ws-1", Identifier: "KAN-1", Title: "Title",
		Description: "Desc", Status: "TODO", Priority: "high", ParentID: "task-0",
		ProjectID: "proj-1", AssigneeID: "agent-1", Labels: `["a"]`,
		CreatedAt: "2026-01-01", UpdatedAt: "2026-01-02", IsSystem: true,
	}
	if got != want {
		t.Fatalf("item = %+v, want %+v", got, want)
	}
}

func TestToTaskListItems_EmptyInputYieldsEmptySlice(t *testing.T) {
	items := toTaskListItems(nil)
	if items == nil || len(items) != 0 {
		t.Fatalf("items = %#v, want empty non-nil slice", items)
	}
}
