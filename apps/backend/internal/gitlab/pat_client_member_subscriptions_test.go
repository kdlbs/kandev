package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestPATClientListProjectMembersReturnsActiveNumericIDs(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.EscapedPath(), "/projects/group%2Fsub%2Fproject/members/all"; got != want {
			t.Errorf("escaped path = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("query"), "ali ce"; got != want {
			t.Errorf("query = %q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 42, "username": "alice", "name": "Alice", "avatar_url": "https://img/alice", "state": "active"},
			{"id": 7, "username": "away", "name": "Away", "state": "blocked"},
		})
	}))
	defer stop()

	members, err := NewPATClient(host, "tok").ListProjectMembers(t.Context(), "group/sub/project", "ali ce")
	if err != nil {
		t.Fatalf("list project members: %v", err)
	}
	want := []ProjectMember{{ID: 42, Username: "alice", Name: "Alice", AvatarURL: "https://img/alice"}}
	if !reflect.DeepEqual(members, want) {
		t.Fatalf("members = %#v, want %#v", members, want)
	}
}

func TestPATClientSetMRReviewersSendsReplacementIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		ids  []int64
	}{
		{name: "replace", ids: []int64{42, 91}},
		{name: "clear", ids: []int64{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var payload struct {
				ReviewerIDs []int64 `json:"reviewer_ids"`
			}
			host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got, want := r.Method, http.MethodPut; got != want {
					t.Errorf("method = %s, want %s", got, want)
				}
				if got, want := r.URL.EscapedPath(), "/projects/group%2Fproject/merge_requests/12"; got != want {
					t.Errorf("escaped path = %q, want %q", got, want)
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("decode reviewer payload: %v", err)
				}
			}))
			defer stop()

			if err := NewPATClient(host, "tok").SetMRReviewers(t.Context(), "group/project", 12, tc.ids); err != nil {
				t.Fatalf("set MR reviewers: %v", err)
			}
			if !reflect.DeepEqual(payload.ReviewerIDs, tc.ids) {
				t.Fatalf("reviewer_ids = %#v, want %#v", payload.ReviewerIDs, tc.ids)
			}
		})
	}
}

func TestPATClientSubscriptionReadsCorrectResource(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		get  func(*PATClient) (*SubscriptionState, error)
	}{
		{
			name: "merge request",
			path: "/projects/group%2Fproject/merge_requests/12",
			get: func(c *PATClient) (*SubscriptionState, error) {
				return c.GetMRSubscription(context.Background(), "group/project", 12)
			},
		},
		{
			name: "issue",
			path: "/projects/group%2Fproject/issues/12",
			get: func(c *PATClient) (*SubscriptionState, error) {
				return c.GetIssueSubscription(context.Background(), "group/project", 12)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.EscapedPath(); got != tc.path {
					t.Errorf("escaped path = %q, want %q", got, tc.path)
				}
				_, _ = w.Write([]byte(`{"subscribed":true}`))
			}))
			defer stop()

			state, err := tc.get(NewPATClient(host, "tok"))
			if err != nil {
				t.Fatalf("get subscription: %v", err)
			}
			if !state.Subscribed {
				t.Fatal("subscribed = false, want true")
			}
		})
	}
}

func TestPATClientSubscriptionWritesCorrectResource(t *testing.T) {
	for _, tc := range []struct {
		name       string
		path       string
		subscribed bool
		set        func(*PATClient, bool) (*SubscriptionState, error)
	}{
		{"subscribe MR", "/projects/group%2Fproject/merge_requests/12/subscribe", true, func(c *PATClient, value bool) (*SubscriptionState, error) {
			return c.SetMRSubscription(context.Background(), "group/project", 12, value)
		}},
		{"unsubscribe MR", "/projects/group%2Fproject/merge_requests/12/unsubscribe", false, func(c *PATClient, value bool) (*SubscriptionState, error) {
			return c.SetMRSubscription(context.Background(), "group/project", 12, value)
		}},
		{"subscribe issue", "/projects/group%2Fproject/issues/12/subscribe", true, func(c *PATClient, value bool) (*SubscriptionState, error) {
			return c.SetIssueSubscription(context.Background(), "group/project", 12, value)
		}},
		{"unsubscribe issue", "/projects/group%2Fproject/issues/12/unsubscribe", false, func(c *PATClient, value bool) (*SubscriptionState, error) {
			return c.SetIssueSubscription(context.Background(), "group/project", 12, value)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got, want := r.Method, http.MethodPost; got != want {
					t.Errorf("method = %s, want %s", got, want)
				}
				if got := r.URL.EscapedPath(); got != tc.path {
					t.Errorf("escaped path = %q, want %q", got, tc.path)
				}
				_, _ = w.Write([]byte(`{"subscribed":` + map[bool]string{true: "true", false: "false"}[tc.subscribed] + `}`))
			}))
			defer stop()

			state, err := tc.set(NewPATClient(host, "tok"), tc.subscribed)
			if err != nil {
				t.Fatalf("set subscription: %v", err)
			}
			if state.Subscribed != tc.subscribed {
				t.Fatalf("subscribed = %t, want %t", state.Subscribed, tc.subscribed)
			}
		})
	}
}

func TestPATClientSubscriptionWriteTreatsNotModifiedAsIdempotentSuccess(t *testing.T) {
	for _, tc := range []struct {
		name       string
		path       string
		subscribed bool
		set        func(*PATClient, bool) (*SubscriptionState, error)
	}{
		{"subscribe MR", "/projects/group%2Fproject/merge_requests/12/subscribe", true, func(c *PATClient, value bool) (*SubscriptionState, error) {
			return c.SetMRSubscription(context.Background(), "group/project", 12, value)
		}},
		{"unsubscribe MR", "/projects/group%2Fproject/merge_requests/12/unsubscribe", false, func(c *PATClient, value bool) (*SubscriptionState, error) {
			return c.SetMRSubscription(context.Background(), "group/project", 12, value)
		}},
		{"subscribe issue", "/projects/group%2Fproject/issues/12/subscribe", true, func(c *PATClient, value bool) (*SubscriptionState, error) {
			return c.SetIssueSubscription(context.Background(), "group/project", 12, value)
		}},
		{"unsubscribe issue", "/projects/group%2Fproject/issues/12/unsubscribe", false, func(c *PATClient, value bool) (*SubscriptionState, error) {
			return c.SetIssueSubscription(context.Background(), "group/project", 12, value)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.EscapedPath(); got != tc.path {
					t.Errorf("escaped path = %q, want %q", got, tc.path)
				}
				w.WriteHeader(http.StatusNotModified)
			}))
			defer stop()

			state, err := tc.set(NewPATClient(host, "tok"), tc.subscribed)
			if err != nil {
				t.Fatalf("set subscription returned error for idempotent 304: %v", err)
			}
			if state.Subscribed != tc.subscribed {
				t.Fatalf("subscribed = %t, want requested state %t", state.Subscribed, tc.subscribed)
			}
		})
	}
}
