package models

import (
	"encoding/json"
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
