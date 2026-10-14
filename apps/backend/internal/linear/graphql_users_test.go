package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func memberFixture(n int) LinearUser {
	return LinearUser{ID: fmt.Sprintf("member-%d", n), Name: fmt.Sprintf("Member %d", n), DisplayName: fmt.Sprintf("Creator%d", n), Email: fmt.Sprintf("member%d@example.test", n), AvatarURL: fmt.Sprintf("https://example.test/%d.png", n)}
}

func writeMembersPage(t *testing.T, w http.ResponseWriter, users []LinearUser, pageInfo any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"users": map[string]any{"nodes": users, "pageInfo": pageInfo}}}); err != nil {
		t.Errorf("encode members: %v", err)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-MEMBERS-001.1, AC-INTEGRATIONS-LINEAR-MEMBERS-001.2
func TestGraphQLClient_ListUsers_Pagination(t *testing.T) {
	want := make([]LinearUser, 251)
	for i := range want {
		want[i] = memberFixture(i + 1)
	}
	requests := 0
	var queries []string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 2 {
			http.Error(w, "unexpected extra page", http.StatusInternalServerError)
			return
		}
		req := readReq(t, r)
		query, _ := req["query"].(string)
		queries = append(queries, query)
		vars := req["variables"].(map[string]any)
		filter := map[string]any{"teamMemberships": map[string]any{"some": map[string]any{"team": map[string]any{"key": map[string]any{"eq": "ENG"}}}}}
		if !reflect.DeepEqual(vars["filter"], filter) || vars["first"] != float64(250) {
			t.Errorf("page %d scope/size: %v", requests, vars)
		}
		if r.Header.Get("Authorization") != "lin_api_fixture" {
			t.Error("credential changed")
		}
		if !strings.Contains(query, "orderBy: updatedAt") {
			t.Error("member ordering changed")
		}
		if requests == 1 {
			if vars["after"] != nil {
				t.Errorf("first page after=%v", vars["after"])
			}
			writeMembersPage(t, w, want[:250], map[string]any{"hasNextPage": true, "endCursor": "opaque-250"})
		} else {
			if vars["after"] != "opaque-250" {
				t.Errorf("continuation=%v", vars["after"])
			}
			writeMembersPage(t, w, want[250:], map[string]any{"hasNextPage": false})
		}
	})
	got, err := pointTo(NewGraphQLClient(&LinearConfig{}, "lin_api_fixture"), ts.URL).ListUsers(context.Background(), "ENG")
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(got) != 251 {
		t.Fatalf("Creator inventory has %d members after %d requests; want 251 including Creator251", len(got), requests)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("member identity/order/fields changed: got last=%+v", got[len(got)-1])
	}
	if requests != 2 {
		t.Fatalf("requests=%d, want 2", requests)
	}
	for _, query := range queries {
		if !strings.Contains(query, "$after: String") || !strings.Contains(query, "after: $after") || !strings.Contains(query, "pageInfo") || !strings.Contains(query, "hasNextPage") || !strings.Contains(query, "endCursor") {
			t.Errorf("missing cursor wire contract: %s", query)
		}
	}
}

// @covers AC-INTEGRATIONS-LINEAR-MEMBERS-001.2
func TestGraphQLClient_ListUsers_Controls(t *testing.T) {
	for _, n := range []int{0, 3, 250} {
		for _, cursor := range []any{nil, "", "terminal-cursor"} {
			t.Run(fmt.Sprintf("members%d/cursor%v", n, cursor), func(t *testing.T) {
				want := make([]LinearUser, n)
				for i := range want {
					want[i] = memberFixture(i + 1)
				}
				requests := 0
				ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
					requests++
					if requests > 1 {
						http.Error(w, "extra request", http.StatusInternalServerError)
						return
					}
					writeMembersPage(t, w, want, map[string]any{"hasNextPage": false, "endCursor": cursor})
				})
				got, err := pointTo(NewGraphQLClient(&LinearConfig{}, "key"), ts.URL).ListUsers(context.Background(), "ENG")
				if err != nil || !reflect.DeepEqual(got, want) || requests != 1 {
					t.Fatalf("got=%v err=%v requests=%d; want %d members", got, err, requests, n)
				}
			})
		}
	}
	t.Run("empty team", func(t *testing.T) {
		ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
			t.Error("empty team reached HTTP")
			w.WriteHeader(http.StatusInternalServerError)
		})
		got, err := pointTo(NewGraphQLClient(&LinearConfig{}, "key"), ts.URL).ListUsers(context.Background(), "")
		if got != nil || err == nil {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
}

// @covers AC-INTEGRATIONS-LINEAR-MEMBERS-001.1, AC-INTEGRATIONS-LINEAR-MEMBERS-001.4
func TestGraphQLClient_ListUsers_EmptyIntermediatePage(t *testing.T) {
	requests := 0
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 3 {
			http.Error(w, "extra page", http.StatusInternalServerError)
			return
		}
		vars := readReq(t, r)["variables"].(map[string]any)
		switch requests {
		case 1:
			writeMembersPage(t, w, []LinearUser{memberFixture(1)}, map[string]any{"hasNextPage": true, "endCursor": "cursor A"})
		case 2:
			if vars["after"] != "cursor A" {
				t.Errorf("after=%v", vars["after"])
			}
			writeMembersPage(t, w, []LinearUser{}, map[string]any{"hasNextPage": true, "endCursor": "cursor B"})
		case 3:
			if vars["after"] != "cursor B" {
				t.Errorf("after=%v", vars["after"])
			}
			writeMembersPage(t, w, []LinearUser{memberFixture(2)}, map[string]any{"hasNextPage": false, "endCursor": "cursor B"})
		}
	})
	got, err := pointTo(NewGraphQLClient(&LinearConfig{}, "key"), ts.URL).ListUsers(context.Background(), "ENG")
	if err != nil || !reflect.DeepEqual(got, []LinearUser{memberFixture(1), memberFixture(2)}) || requests != 3 {
		t.Fatalf("got=%v err=%v requests=%d", got, err, requests)
	}
}

// @covers AC-INTEGRATIONS-LINEAR-MEMBERS-001.3
func TestGraphQLClient_ListUsers_PageFailure(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		apiStatus int
	}{
		{"unauthorized", 401, `{"error":"expired"}`, 401},
		{"server error", 500, `{"error":"unavailable"}`, 500},
		{"graphql", 200, `{"data":{"users":{"nodes":[]}},"errors":[{"message":"forbidden","extensions":{"type":"forbidden"}}]}`, 403},
		{"invalid envelope", 200, `not JSON`, 200},
		{"invalid data", 200, `{"data":{"users":{"nodes":"wrong"}}}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
				requests++
				if requests == 1 {
					writeMembersPage(t, w, []LinearUser{memberFixture(1)}, map[string]any{"hasNextPage": true, "endCursor": "next"})
					return
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			got, err := pointTo(NewGraphQLClient(&LinearConfig{}, "key"), ts.URL).ListUsers(context.Background(), "ENG")
			if got != nil || err == nil || requests != 2 {
				t.Fatalf("partial inventory=%v err=%v requests=%d", got, err, requests)
			}
			var apiErr *APIError
			if tc.apiStatus != 0 && (!errors.As(err, &apiErr) || apiErr.StatusCode != tc.apiStatus) {
				t.Fatalf("error classification lost: %v", err)
			}
			if tc.apiStatus == 0 {
				var decodeErr *json.UnmarshalTypeError
				if !errors.As(err, &decodeErr) {
					t.Fatalf("decode error lost: %v", err)
				}
			}
		})
	}
}

// @covers AC-INTEGRATIONS-LINEAR-MEMBERS-001.4
func TestGraphQLClient_ListUsers_InvalidPagination(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pages []any
	}{
		{"missing page info", []any{nil}},
		{"missing completion flag", []any{map[string]any{"endCursor": "private-cursor"}}},
		{"null completion flag", []any{map[string]any{"hasNextPage": nil}}},
		{"missing cursor", []any{map[string]any{"hasNextPage": true}}},
		{"null cursor", []any{map[string]any{"hasNextPage": true, "endCursor": nil}}},
		{"empty cursor", []any{map[string]any{"hasNextPage": true, "endCursor": ""}}},
		{"repeated cursor", []any{memberContinuation("private-A"), memberContinuation("private-A")}},
		{"cursor cycle", []any{memberContinuation("private-A"), memberContinuation("private-B"), memberContinuation("private-A")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
				requests++
				if requests > len(tc.pages) {
					http.Error(w, "request budget exceeded", http.StatusInternalServerError)
					return
				}
				if tc.pages[requests-1] == nil {
					_, _ = w.Write([]byte(`{"data":{"users":{"nodes":[{"id":"member-1"}]}}}`))
					return
				}
				writeMembersPage(t, w, []LinearUser{memberFixture(requests)}, tc.pages[requests-1])
			})
			got, err := pointTo(NewGraphQLClient(&LinearConfig{}, "key"), ts.URL).ListUsers(context.Background(), "ENG")
			if got != nil || err == nil || requests != len(tc.pages) {
				t.Fatalf("got=%v err=%v requests=%d, want %d", got, err, requests, len(tc.pages))
			}
			if strings.Contains(err.Error(), "private-") || strings.Contains(err.Error(), "member-") {
				t.Fatalf("diagnostic leaked provider data: %v", err)
			}
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				t.Fatalf("malformed metadata classified as API status: %v", err)
			}
		})
	}
	t.Run("null page info", func(t *testing.T) {
		ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeMembersPage(t, w, []LinearUser{memberFixture(1)}, nil)
		})
		got, err := pointTo(NewGraphQLClient(&LinearConfig{}, "key"), ts.URL).ListUsers(context.Background(), "ENG")
		if got != nil || err == nil {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
}

func memberContinuation(cursor string) map[string]any {
	return map[string]any{"hasNextPage": true, "endCursor": cursor}
}

// @covers AC-INTEGRATIONS-LINEAR-MEMBERS-001.3
func TestGraphQLClient_ListUsers_Context(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprintf("admission/deadline%t", expired), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			wantErr := context.Canceled
			if expired {
				ctx, cancel = context.WithDeadline(context.Background(), time.Unix(1, 0))
				wantErr = context.DeadlineExceeded
			} else {
				cancel()
			}
			defer cancel()
			ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
				t.Error("expired context admitted HTTP")
				w.WriteHeader(500)
			})
			got, err := pointTo(NewGraphQLClient(&LinearConfig{}, "key"), ts.URL).ListUsers(ctx, "ENG")
			if got != nil || !errors.Is(err, wantErr) {
				t.Fatalf("got=%v err=%v", got, err)
			}
		})
	}
	for _, terminal := range []bool{false, true} {
		t.Run(fmt.Sprintf("after response/terminal%t", terminal), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			requests := 0
			ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
				requests++
				writeMembersPage(t, w, []LinearUser{memberFixture(1)}, map[string]any{"hasNextPage": !terminal, "endCursor": "next"})
			})
			c := pointTo(NewGraphQLClient(&LinearConfig{}, "key"), ts.URL)
			c.http.Transport = cancelOnResponseClose{base: http.DefaultTransport, cancel: cancel}
			got, err := c.ListUsers(ctx, "ENG")
			if got != nil || !errors.Is(err, context.Canceled) || requests != 1 {
				t.Fatalf("got=%v err=%v requests=%d", got, err, requests)
			}
		})
	}
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprintf("blocked next page/deadline%t", deadline), func(t *testing.T) { testBlockedMemberPage(t, deadline) })
	}
}

type cancelOnResponseClose struct {
	base   http.RoundTripper
	cancel context.CancelFunc
}

func (c cancelOnResponseClose) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := c.base.RoundTrip(r)
	if err == nil {
		resp.Body = cancelCloseBody{ReadCloser: resp.Body, cancel: c.cancel}
	}
	return resp, err
}

type cancelCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelCloseBody) Close() error { err := c.ReadCloser.Close(); c.cancel(); return err }

func testBlockedMemberPage(t *testing.T, deadline bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	wantErr := context.Canceled
	if deadline {
		ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		wantErr = context.DeadlineExceeded
	}
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	requests := 0
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			writeMembersPage(t, w, []LinearUser{memberFixture(1)}, memberContinuation("next"))
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	type result struct {
		users []LinearUser
		err   error
	}
	done := make(chan result, 1)
	go func() {
		users, err := pointTo(NewGraphQLClient(&LinearConfig{}, "key"), ts.URL).ListUsers(ctx, "ENG")
		done <- result{users, err}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("member request did not stop")
		}
	})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("next page not reached")
	}
	if !deadline {
		cancel()
	}
	select {
	case res := <-done:
		done <- res
		if res.users != nil || !errors.Is(res.err, wantErr) {
			t.Fatalf("got=%v err=%v", res.users, res.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("member request did not stop")
	}
}
