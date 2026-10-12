package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSidebarTaskViewQueryValidation(t *testing.T) {
	valid := SidebarTaskViewQuery{
		Filters: []SidebarTaskViewClause{{Dimension: "titleMatch", Op: "matches", Value: json.RawMessage(`"needle"`)}},
		Sort:    SidebarTaskViewSort{Key: "lastActivityAt", Direction: "desc"}, Group: "repository",
		Page: 1, PageSize: 100, Locale: "en",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}
	korean := valid
	korean.Locale = "ko"
	if err := korean.Validate(); err != nil {
		t.Fatalf("korean query rejected: %v", err)
	}
	for _, direction := range []string{"asc", "desc"} {
		runningFirst := valid
		runningFirst.Sort = SidebarTaskViewSort{Key: "runningFirstActivity", Direction: direction}
		if err := runningFirst.Validate(); err != nil {
			t.Errorf("running-first query with %s direction rejected: %v", direction, err)
		}
	}

	tests := []struct {
		name  string
		query SidebarTaskViewQuery
	}{
		{name: "zero page", query: func() SidebarTaskViewQuery { q := valid; q.Page = 0; return q }()},
		{name: "oversized page", query: func() SidebarTaskViewQuery { q := valid; q.PageSize = 101; return q }()},
		{name: "unknown dimension", query: func() SidebarTaskViewQuery {
			q := valid
			q.Filters = []SidebarTaskViewClause{{Dimension: "sql", Op: "is", Value: json.RawMessage(`"x"`)}}
			return q
		}()},
		{name: "unknown operator", query: func() SidebarTaskViewQuery {
			q := valid
			q.Filters = []SidebarTaskViewClause{{Dimension: "titleMatch", Op: "regex", Value: json.RawMessage(`"x"`)}}
			return q
		}()},
		{name: "text operator on boolean dimension", query: func() SidebarTaskViewQuery {
			q := valid
			q.Filters = []SidebarTaskViewClause{{Dimension: "archived", Op: "matches", Value: json.RawMessage(`"true"`)}}
			return q
		}()},
		{name: "boolean dimension type", query: func() SidebarTaskViewQuery {
			q := valid
			q.Filters = []SidebarTaskViewClause{{Dimension: "archived", Op: "is", Value: json.RawMessage(`"true"`)}}
			return q
		}()},
		{name: "invalid state bucket", query: func() SidebarTaskViewQuery {
			q := valid
			q.Filters = []SidebarTaskViewClause{{Dimension: "state", Op: "is", Value: json.RawMessage(`"DONE"`)}}
			return q
		}()},
		{name: "invalid locale", query: func() SidebarTaskViewQuery { q := valid; q.Locale = "xx"; return q }()},
		{name: "too many clauses", query: func() SidebarTaskViewQuery {
			q := valid
			q.Filters = make([]SidebarTaskViewClause, MaxSidebarViewClauses+1)
			return q
		}()},
		{name: "too many collapsed ids", query: func() SidebarTaskViewQuery {
			q := valid
			q.CollapsedTaskIDs = make([]string, MaxSidebarViewPreferenceIDs+1)
			return q
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.query.Validate(); err == nil {
				t.Fatal("invalid query accepted")
			}
		})
	}
}

func TestSidebarTaskViewSortChainValidation(t *testing.T) {
	base := SidebarTaskViewQuery{
		Sort: SidebarTaskViewSort{
			Key: "running", Direction: "desc",
			ThenBy: []SidebarTaskViewSortCriterion{
				{Key: "color", Color: "red", Direction: "desc"},
				{Key: "lastActivityAt", Direction: "desc"},
			},
		},
		Group: "none", Page: 1, PageSize: 100, Locale: "en",
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid chain rejected: %v", err)
	}

	invalid := base
	invalid.Sort.ThenBy = append(invalid.Sort.ThenBy, SidebarTaskViewSortCriterion{Key: "running", Direction: "asc"})
	err := invalid.Validate()
	var validation *SidebarQueryValidationError
	if !errors.As(err, &validation) || validation.SortIndex == nil || *validation.SortIndex != 3 {
		t.Fatalf("expected safe zero-based rule index 3, got %#v (%v)", validation, err)
	}

	invalid = base
	invalid.Sort.ThenBy = make([]SidebarTaskViewSortCriterion, MaxSidebarViewSortRules)
	if err := invalid.Validate(); err == nil {
		t.Fatal("accepted more than ten sort rules")
	}

	legacy := base
	legacy.Sort = SidebarTaskViewSort{Key: "runningFirstActivity", Direction: "asc"}
	if got := legacy.Sort.Criteria(); len(got) != 2 || got[0].Key != "running" || got[1].Key != "lastActivityAt" {
		t.Fatalf("legacy preset was not expanded: %+v", got)
	}
}

func TestSidebarTaskViewQueryNegativeFilterSemanticsRemainExplicit(t *testing.T) {
	for operator, value := range map[string]string{
		"is_not":      `"needle"`,
		"not_in":      `["needle"]`,
		"not_matches": `"needle"`,
	} {
		query := SidebarTaskViewQuery{
			Filters: []SidebarTaskViewClause{{Dimension: "titleMatch", Op: operator, Value: json.RawMessage(value)}},
			Sort:    SidebarTaskViewSort{Key: "title", Direction: "asc"}, Group: "none", Page: 1, PageSize: 100, Locale: "en",
		}
		if err := query.Validate(); err != nil {
			t.Errorf("negative operator %q rejected: %v", operator, err)
		}
	}
}

// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.14
func TestSidebarTaskViewQueryMembershipLimits(t *testing.T) {
	for _, dimension := range []string{"workflow", "repository"} {
		for _, count := range []int{0, 6, 7, 1000, 1001} {
			t.Run(fmt.Sprintf("%s/%d", dimension, count), func(t *testing.T) {
				values := make([]string, count)
				for i := range values {
					values[i] = fmt.Sprintf("organization/repository-with-long-name-%04d", i)
					if dimension == "workflow" {
						values[i] = fmt.Sprintf("00000000-0000-0000-0000-%012d", i)
					}
				}
				raw, _ := json.Marshal(values)
				q := SidebarTaskViewQuery{Filters: []SidebarTaskViewClause{{Dimension: dimension, Op: "in", Value: raw}}, Sort: SidebarTaskViewSort{Key: "state", Direction: "asc"}, Group: "none", Page: 1, PageSize: 100, Locale: "en"}
				if err := q.Validate(); (err != nil) != (count > 1000) {
					t.Fatalf("count %d: %v", count, err)
				}
			})
		}
	}
	for _, value := range []string{strings.Repeat("a", 256), strings.Repeat("界", 85), strings.Repeat("\"", 256)} {
		raw, _ := json.Marshal(value)
		q := SidebarTaskViewQuery{Filters: []SidebarTaskViewClause{{Dimension: "titleMatch", Op: "matches", Value: raw}}, Sort: SidebarTaskViewSort{Key: "state", Direction: "asc"}, Group: "none", Page: 1, PageSize: 100, Locale: "en"}
		if err := q.Validate(); err != nil {
			t.Errorf("valid decoded string length %d: %v", len(value), err)
		}
	}
}

func TestSidebarTaskViewQueryScalarBoundaries(t *testing.T) {
	for _, raw := range []string{`null`, `[null]`, `[true]`, `12`, `["` + strings.Repeat("a", 257) + `"]`, `["` + strings.Repeat("界", 86) + `"]`} {
		q := SidebarTaskViewQuery{Filters: []SidebarTaskViewClause{{Dimension: "repository", Op: "in", Value: json.RawMessage(raw)}}, Sort: SidebarTaskViewSort{Key: "state", Direction: "asc"}, Group: "none", Page: 1, PageSize: 100, Locale: "en"}
		if err := q.Validate(); err == nil {
			t.Errorf("invalid value accepted: %s", raw)
		}
	}
}
