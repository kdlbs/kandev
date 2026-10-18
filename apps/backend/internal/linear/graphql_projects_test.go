package linear

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func writeProjectsPage(t *testing.T, w http.ResponseWriter, projects []LinearProject, pageInfo any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"projects": map[string]any{"nodes": projects, "pageInfo": pageInfo}}}); err != nil {
		t.Errorf("encode projects: %v", err)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-001.3
func TestGraphQLClient_ListProjects_PaginatesTeamScope(t *testing.T) {
	want := make([]LinearProject, 251)
	for i := range want {
		want[i] = LinearProject{ID: fmt.Sprintf("project-%d", i+1), Name: fmt.Sprintf("Project %d", i+1)}
	}
	requests := 0
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 2 {
			http.Error(w, "unexpected extra page", http.StatusInternalServerError)
			return
		}
		req := readReq(t, r)
		vars := req["variables"].(map[string]any)
		filter := map[string]any{"accessibleTeams": map[string]any{"some": map[string]any{"key": map[string]any{"eq": "ENG"}}}}
		if !reflect.DeepEqual(vars["filter"], filter) || vars["first"] != float64(250) {
			t.Errorf("page %d scope/size: %v", requests, vars)
		}
		if requests == 1 {
			if vars["after"] != nil {
				t.Errorf("first page after=%v", vars["after"])
			}
			writeProjectsPage(t, w, want[:250], map[string]any{"hasNextPage": true, "endCursor": "opaque-250"})
			return
		}
		if vars["after"] != "opaque-250" {
			t.Errorf("continuation=%v", vars["after"])
		}
		writeProjectsPage(t, w, want[250:], map[string]any{"hasNextPage": false})
	})
	got, err := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL).ListProjects(context.Background(), "ENG")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %d projects, want %d in provider order", len(got), len(want))
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-001.3
func TestGraphQLClient_ListProjects_RejectsInvalidResponses(t *testing.T) {
	if _, err := NewGraphQLClient(&LinearConfig{}, "x").ListProjects(context.Background(), ""); err == nil {
		t.Fatal("empty team key must fail before any request")
	}
	cases := map[string]any{
		"missing pageInfo": nil,
		"missing hasNext":  map[string]any{"endCursor": "c"},
		"empty cursor":     map[string]any{"hasNextPage": true, "endCursor": ""},
		"repeated cursor":  map[string]any{"hasNextPage": true, "endCursor": "same"},
	}
	for name, pageInfo := range cases {
		t.Run(name, func(t *testing.T) {
			requests := 0
			ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
				requests++
				if requests > 3 {
					http.Error(w, "request budget exceeded", http.StatusInternalServerError)
					return
				}
				writeProjectsPage(t, w, []LinearProject{{ID: "p", Name: "P"}}, pageInfo)
			})
			got, err := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL).ListProjects(context.Background(), "ENG")
			if err == nil || got != nil {
				t.Fatalf("got projects=%v err=%v, want nil projects and an error", got, err)
			}
			if !strings.Contains(err.Error(), "projects pagination") {
				t.Fatalf("err=%v, want a pagination error", err)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-002.2
func TestGraphQLClient_GetProjectContents_BatchesByID(t *testing.T) {
	ids := make([]string, 251)
	for i := range ids {
		ids[i] = fmt.Sprintf("project-%d", i+1)
	}
	var batches [][]any
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		req := readReq(t, r)
		if q, _ := req["query"].(string); !strings.Contains(q, "includeArchived: true") || !strings.Contains(q, "content") {
			t.Errorf("query must request content including archived projects: %s", q)
		}
		vars := req["variables"].(map[string]any)
		in := vars["filter"].(map[string]any)["id"].(map[string]any)["in"].([]any)
		if vars["first"] != float64(len(in)) {
			t.Errorf("first=%v for %d ids", vars["first"], len(in))
		}
		batches = append(batches, in)
		nodes := make([]map[string]any, 0, len(in))
		for _, id := range in {
			if id == "project-2" {
				continue // Linear omits projects the key cannot see.
			}
			nodes = append(nodes, map[string]any{"id": id, "content": "brief of " + id.(string)})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"projects": map[string]any{"nodes": nodes}}})
	})
	got, err := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL).GetProjectContents(context.Background(), ids)
	if err != nil {
		t.Fatalf("GetProjectContents: %v", err)
	}
	if len(batches) != 11 || len(batches[0]) != projectContentsBatchSize || len(batches[10]) != 1 {
		t.Fatalf("batches=%d, want ten of %d then one", len(batches), projectContentsBatchSize)
	}
	if len(got) != 250 || got["project-251"] != "brief of project-251" {
		t.Fatalf("got %d contents, last=%q", len(got), got["project-251"])
	}
	if _, ok := got["project-2"]; ok {
		t.Fatal("an omitted project must be absent from the result")
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-002.1
func TestGraphQLClient_SearchIssues_DecodesProject(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		req := readReq(t, r)
		if q, _ := req["query"].(string); !strings.Contains(q, "project { id name url description }") {
			t.Errorf("issue projection must request the project: %s", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[
			{"id":"i1","identifier":"ENG-1","title":"a","url":"u","state":{"id":"s","name":"S","type":"backlog"},"team":{"id":"t","key":"ENG"},
			 "project":{"id":"p1","name":"Checkout","url":"https://linear.app/acme/project/checkout","description":"Faster checkout"}},
			{"id":"i2","identifier":"ENG-2","title":"b","url":"u","state":{"id":"s","name":"S","type":"backlog"},"team":{"id":"t","key":"ENG"},"project":null}
		],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`))
	})
	res, err := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL).SearchIssues(context.Background(), SearchFilter{TeamKey: "ENG"}, "", 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	withProject, withoutProject := res.Issues[0], res.Issues[1]
	if withProject.ProjectID != "p1" || withProject.ProjectName != "Checkout" ||
		withProject.ProjectURL != "https://linear.app/acme/project/checkout" || withProject.ProjectSummary != "Faster checkout" {
		t.Fatalf("project fields not decoded: %+v", withProject)
	}
	if withProject.ProjectDescription != "" {
		t.Fatal("search results must not carry project content")
	}
	if withoutProject.ProjectID != "" || withoutProject.ProjectName != "" || withoutProject.ProjectURL != "" || withoutProject.ProjectSummary != "" {
		t.Fatalf("issue without a project must have empty project fields: %+v", withoutProject)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-001.1, AC-INTEGRATIONS-LINEAR-PROJECTS-001.2
func TestBuildIssueFilter_Projects(t *testing.T) {
	got := buildIssueFilter(SearchFilter{TeamKey: "ENG", ProjectIDs: []string{"p1", "p2"}, LabelIDs: []string{"l1"}})
	want := map[string]interface{}{
		"team":    map[string]interface{}{"key": map[string]interface{}{"eq": "ENG"}},
		"project": map[string]interface{}{"id": map[string]interface{}{"in": []string{"p1", "p2"}}},
		"labels": map[string]interface{}{
			"some": map[string]interface{}{"id": map[string]interface{}{"in": []string{"l1"}}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if _, ok := buildIssueFilter(SearchFilter{TeamKey: "ENG"})["project"]; ok {
		t.Fatal("a filter without projects must not send a project condition")
	}
}

// @covers AC-INTEGRATIONS-LINEAR-PROJECTS-001.1, AC-INTEGRATIONS-LINEAR-PROJECTS-001.2
func TestGraphQLClient_SearchIssues_SendsProjectFilter(t *testing.T) {
	for _, teamKey := range []string{"", "ENG"} {
		t.Run("team="+teamKey, func(t *testing.T) {
			requests := 0
			ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				req := readReq(t, r)
				vars := req["variables"].(map[string]any)
				want := map[string]any{
					"project": map[string]any{"id": map[string]any{"in": []any{"p1", "p2"}}},
				}
				if teamKey != "" {
					want["team"] = map[string]any{"key": map[string]any{"eq": teamKey}}
				}
				if !reflect.DeepEqual(vars["filter"], want) {
					t.Errorf("request filter = %#v, want %#v", vars["filter"], want)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}`))
			})
			client := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL)
			_, err := client.SearchIssues(context.Background(), SearchFilter{
				TeamKey: teamKey, ProjectIDs: []string{"p1", "p2"},
			}, "", 50)
			if err != nil || requests != 1 {
				t.Fatalf("search: requests=%d err=%v", requests, err)
			}
		})
	}
}
