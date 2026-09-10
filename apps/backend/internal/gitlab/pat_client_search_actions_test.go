package gitlab

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
)

func TestPATClientSearchMRsPagedClampsPaginationAndReadsTotal(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/merge_requests"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		query := r.URL.Query()
		if got, want := query.Get("page"), "1"; got != want {
			t.Errorf("page = %q, want %q", got, want)
		}
		if got, want := query.Get("per_page"), "100"; got != want {
			t.Errorf("per_page = %q, want %q", got, want)
		}
		if got, want := query.Get("state"), "opened"; got != want {
			t.Errorf("state = %q, want %q", got, want)
		}
		w.Header().Set("X-Total", "137")
		_ = json.NewEncoder(w).Encode([]rawMR{{IID: 8, Title: "Review me", State: "opened"}})
	}))
	t.Cleanup(stop)

	page, err := NewPATClient(host, "tok").SearchMRsPaged(t.Context(), "labels=backend", "", 0, 500)
	if err != nil {
		t.Fatalf("SearchMRsPaged() error = %v", err)
	}
	if page.TotalCount != 137 || page.Page != 1 || page.PerPage != 100 {
		t.Fatalf("pagination = (%d, %d, %d), want (137, 1, 100)", page.TotalCount, page.Page, page.PerPage)
	}
	if len(page.MRs) != 1 || page.MRs[0].IID != 8 || page.MRs[0].State != "open" {
		t.Fatalf("MRs = %#v, want converted MR !8", page.MRs)
	}
}

func TestPATClientListIssuesUsesDefaultPagination(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got, want := query.Get("page"), "1"; got != want {
			t.Errorf("page = %q, want %q", got, want)
		}
		if got, want := query.Get("per_page"), "50"; got != want {
			t.Errorf("per_page = %q, want %q", got, want)
		}
		w.Header().Set("X-Total", "1")
		_ = json.NewEncoder(w).Encode([]rawIssue{{IID: 3, Title: "Fix it", State: "opened"}})
	}))
	t.Cleanup(stop)

	issues, err := NewPATClient(host, "tok").ListIssues(t.Context(), "open", "assignee_username=alice", "")
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(issues) != 1 || issues[0].IID != 3 || issues[0].State != "opened" {
		t.Fatalf("issues = %#v, want converted issue #3", issues)
	}
}

func TestPATClientProjectDiscovery(t *testing.T) {
	tests := []struct {
		name     string
		call     func(*PATClient) ([]Project, error)
		wantPath string
		want     map[string]string
	}{
		{
			name: "group projects with subgroup and search",
			call: func(c *PATClient) ([]Project, error) {
				return c.SearchGroupProjects(t.Context(), "acme/team", "api client", 0)
			},
			wantPath: "/groups/acme%2Fteam/projects",
			want:     map[string]string{"per_page": "20", "simple": "true", "include_subgroups": "true", "search": "api client"},
		},
		{
			name: "member projects",
			call: func(c *PATClient) ([]Project, error) {
				return c.ListUserProjects(t.Context())
			},
			wantPath: "/projects",
			want:     map[string]string{"membership": "true", "simple": "true", "per_page": "100", "order_by": "last_activity_at"},
		},
		{
			name: "global search trims blank query",
			call: func(c *PATClient) ([]Project, error) {
				return c.SearchProjects(t.Context(), "   ", -1)
			},
			wantPath: "/projects",
			want:     map[string]string{"membership": "true", "simple": "true", "per_page": "20"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.EscapedPath(); got != tc.wantPath {
					t.Errorf("path = %q, want %q", got, tc.wantPath)
				}
				for key, want := range tc.want {
					if got := r.URL.Query().Get(key); got != want {
						t.Errorf("query %s = %q, want %q", key, got, want)
					}
				}
				if _, ok := tc.want["search"]; !ok && r.URL.Query().Has("search") {
					t.Errorf("unexpected search query %q", r.URL.Query().Get("search"))
				}
				_, _ = w.Write([]byte(`[{"id":12,"path_with_namespace":"acme/widget","name":"Widget","web_url":"https://gitlab.example/acme/widget","default_branch":"main"}]`))
			}))
			t.Cleanup(stop)

			projects, err := tc.call(NewPATClient(host, "tok"))
			if err != nil {
				t.Fatalf("project discovery error = %v", err)
			}
			want := []Project{{ID: 12, PathWithNamespace: "acme/widget", Namespace: "acme", Name: "Widget", WebURL: "https://gitlab.example/acme/widget", DefaultBranch: "main"}}
			if !reflect.DeepEqual(projects, want) {
				t.Fatalf("projects = %#v, want %#v", projects, want)
			}
		})
	}
}

func TestPATClientMergeMRRequestAndResponse(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPut; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.EscapedPath(), "/projects/acme%2Fwidget/merge_requests/9/merge"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		var request MergeMRRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !request.Squash || request.SquashCommitMessage != "squashed changes" || request.ShouldRemoveSourceBranch {
			t.Errorf("request = %#v, want squash without source removal", request)
		}
		_ = json.NewEncoder(w).Encode(rawMR{IID: 9, State: "merged", Title: "Feature"})
	}))
	t.Cleanup(stop)

	mr, err := NewPATClient(host, "tok").MergeMR(t.Context(), "acme/widget", 9, true, "squashed changes")
	if err != nil {
		t.Fatalf("MergeMR() error = %v", err)
	}
	if mr.IID != 9 || mr.State != "merged" {
		t.Fatalf("MR = %#v, want merged MR !9", mr)
	}
}

func TestPATClientProtectedBranchResponses(t *testing.T) {
	t.Run("not protected", func(t *testing.T) {
		host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
		}))
		t.Cleanup(stop)

		branch, err := NewPATClient(host, "tok").GetProtectedBranch(t.Context(), "acme/widget", "feature/x")
		if err != nil || branch != nil {
			t.Fatalf("GetProtectedBranch() = (%#v, %v), want (nil, nil)", branch, err)
		}
	})

	t.Run("protected", func(t *testing.T) {
		host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got, want := r.URL.EscapedPath(), "/projects/acme%2Fwidget/protected_branches/feature%2Fx"; got != want {
				t.Errorf("path = %q, want %q", got, want)
			}
			_, _ = w.Write([]byte(`{"name":"feature/x","push_access_levels":[{"access_level":30}],"merge_access_levels":[{"access_level":40}],"code_owner_approval_required":true}`))
		}))
		t.Cleanup(stop)

		branch, err := NewPATClient(host, "tok").GetProtectedBranch(t.Context(), "acme/widget", "feature/x")
		if err != nil {
			t.Fatalf("GetProtectedBranch() error = %v", err)
		}
		want := &ProtectedBranch{Name: "feature/x", PushAccessLevel: 30, MergeAccessLevel: 40, CodeOwnerApprovalRequired: true}
		if !reflect.DeepEqual(branch, want) {
			t.Fatalf("branch = %#v, want %#v", branch, want)
		}
	})
}

func TestPATClientGetProjectMergeMethods(t *testing.T) {
	tests := []struct {
		name string
		body string
		want ProjectMergeMethods
	}{
		{name: "merge", body: `{"merge_method":"merge","squash_option":"never"}`, want: ProjectMergeMethods{Merge: true}},
		{name: "rebase and squash", body: `{"merge_method":"rebase_merge","squash_option":"default_on"}`, want: ProjectMergeMethods{RebaseMerge: true, AllowSquash: true}},
		{name: "fast forward", body: `{"merge_method":"ff","squash_option":"always"}`, want: ProjectMergeMethods{FastForward: true, AllowSquash: true}},
		{name: "unknown defaults to merge", body: `{"merge_method":"future"}`, want: ProjectMergeMethods{Merge: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(stop)

			got, err := NewPATClient(host, "tok").GetProjectMergeMethods(t.Context(), "acme/widget")
			if err != nil {
				t.Fatalf("GetProjectMergeMethods() error = %v", err)
			}
			if !reflect.DeepEqual(*got, tc.want) {
				t.Fatalf("methods = %#v, want %#v", *got, tc.want)
			}
		})
	}
}

func TestPATClientActionErrorsPreserveAPIError(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(stop)

	_, err := NewPATClient(host, "tok").MergeMR(t.Context(), "acme/widget", 9, false, "")
	if err == nil {
		t.Fatal("MergeMR() error = nil, want upstream error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("error = %v, want wrapped APIError status 503", err)
	}
}

func TestPATClientMRListQueries(t *testing.T) {
	tests := []struct {
		name  string
		call  func(*PATClient) ([]*MR, error)
		check func(*testing.T, *http.Request)
	}{
		{
			name: "authored",
			call: func(c *PATClient) ([]*MR, error) { return c.ListAuthoredMRs(t.Context(), "acme/widget") },
			check: func(t *testing.T, r *http.Request) {
				if got, want := r.URL.Query().Get("author_username"), "alice"; got != want {
					t.Errorf("author_username = %q, want %q", got, want)
				}
			},
		},
		{
			name: "review requested",
			call: func(c *PATClient) ([]*MR, error) {
				return c.ListReviewRequestedMRs(t.Context(), "", "reviewer_username=alice&state=opened")
			},
			check: func(t *testing.T, r *http.Request) {
				if got, want := r.URL.Query().Get("reviewer_username"), "alice"; got != want {
					t.Errorf("reviewer_username = %q, want %q", got, want)
				}
			},
		},
		{
			name: "search wrapper",
			call: func(c *PATClient) ([]*MR, error) { return c.SearchMRs(t.Context(), "state=closed", "") },
			check: func(t *testing.T, r *http.Request) {
				if got, want := r.URL.Query().Get("state"), "closed"; got != want {
					t.Errorf("state = %q, want %q", got, want)
				}
				if got, want := r.URL.Query().Get("page"), "1"; got != want {
					t.Errorf("page = %q, want %q", got, want)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/user" {
					_, _ = w.Write([]byte(`{"username":"alice"}`))
					return
				}
				tc.check(t, r)
				_, _ = w.Write([]byte(`[{"iid":11,"title":"MR","state":"opened"}]`))
			}))
			t.Cleanup(stop)

			mrs, err := tc.call(NewPATClient(host, "tok"))
			if err != nil {
				t.Fatalf("list MRs error = %v", err)
			}
			if len(mrs) != 1 || mrs[0].IID != 11 || mrs[0].State != "open" {
				t.Fatalf("MRs = %#v, want converted MR !11", mrs)
			}
		})
	}
}

func TestPATClientIssueStateAndGroups(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/projects/acme/widget/issues/6":
			_, _ = w.Write([]byte(`{"state":"closed"}`))
		case "/groups":
			if got, want := r.URL.Query().Get("min_access_level"), "10"; got != want {
				t.Errorf("min_access_level = %q, want %q", got, want)
			}
			_, _ = w.Write([]byte(`[{"id":4,"path":"team","name":"Team","avatar_url":"https://img/team"}]`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(stop)

	client := NewPATClient(host, "tok")
	state, err := client.GetIssueState(t.Context(), "acme/widget", 6)
	if err != nil || state != "closed" {
		t.Fatalf("GetIssueState() = (%q, %v), want closed", state, err)
	}
	groups, err := client.ListUserGroups(t.Context())
	if err != nil {
		t.Fatalf("ListUserGroups() error = %v", err)
	}
	want := []Group{{ID: 4, Path: "team", Name: "Team", AvatarURL: "https://img/team"}}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("groups = %#v, want %#v", groups, want)
	}
}
