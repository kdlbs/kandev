package github

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
)

func TestTokenClientFetchRateLimitSeedsAllBuckets(t *testing.T) {
	client, requests := newRecordingPATServer(t, map[string]string{
		"/rate_limit": `{"resources":{
			"core":{"limit":5000,"remaining":4321,"reset":2000000000},
			"graphql":{"limit":5000,"remaining":4900,"reset":2000000000},
			"search":{"limit":30,"remaining":25,"reset":2000000000}
		}}`,
	})
	tracker := NewRateTracker(nil, nil)
	client.WithRateTracker(tracker)

	if err := client.FetchRateLimit(context.Background()); err != nil {
		t.Fatalf("FetchRateLimit: %v", err)
	}
	if len(*requests) != 1 || (*requests)[0].Method != http.MethodGet || (*requests)[0].Path != "/rate_limit" {
		t.Fatalf("requests = %+v, want one GET /rate_limit", *requests)
	}

	for resource, wantRemaining := range map[Resource]int{
		ResourceCore:    4321,
		ResourceGraphQL: 4900,
		ResourceSearch:  25,
	} {
		snapshot, ok := tracker.Snapshot(resource)
		if !ok {
			t.Fatalf("missing %s rate-limit snapshot", resource)
		}
		if snapshot.Remaining != wantRemaining {
			t.Errorf("%s remaining = %d, want %d", resource, snapshot.Remaining, wantRemaining)
		}
	}
}

// patRequest records what a PATClient actually put on the wire so a test can
// assert method, path, query, headers and body rather than only the decoded
// result.
type patRequest struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   string
}

// newRecordingPATServer serves the given path→body map and records every
// request. Any path outside the map returns 404 with a JSON body, which the
// PATClient decode helpers surface as *GitHubAPIError.
func newRecordingPATServer(t *testing.T, routes map[string]string) (*PATClient, *[]patRequest) {
	t.Helper()
	return newRecordingPATServerFunc(t, func(r *http.Request) (int, string) {
		body, ok := routes[r.URL.Path]
		if !ok {
			return http.StatusNotFound, `{"message":"Not Found"}`
		}
		return http.StatusOK, body
	})
}

// newRecordingPATServerFunc is the general form: respond decides the status
// and body per request, and every request is still recorded. Assertions run in
// the calling goroutine off the returned slice — a handler goroutine must
// never call t.Fatal.
func newRecordingPATServerFunc(
	t *testing.T, respond func(*http.Request) (int, string),
) (*PATClient, *[]patRequest) {
	t.Helper()
	var recorded []patRequest
	// GetPRFeedback fans its three upstream reads out concurrently, so the
	// recording slice is written from several handler goroutines at once.
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		recorded = append(recorded, patRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Header: r.Header.Clone(),
			Body:   string(body),
		})
		mu.Unlock()
		status, respBody := respond(r)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return newPATClientPointingAt(t, srv.URL), &recorded
}

// newLinkPaginatedPATServer serves `pages` in order from a single endpoint,
// advertising the next page through a GitHub-shaped `Link: <...>; rel="next"`
// header on every response but the last. It exercises getPaginated's
// follow-the-link loop.
func newLinkPaginatedPATServer(
	t *testing.T, path string, pages []string,
) (*PATClient, *[]patRequest) {
	t.Helper()
	var recorded []patRequest
	served := 0
	// getPaginated follows Link headers strictly in order — request N+1 is only
	// sent after N's response is fully read — so these handler goroutines never
	// overlap today. The mutex mirrors newRecordingPATServer and keeps that from
	// becoming a silent race if a future test drives this server concurrently.
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		recorded = append(recorded, patRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Header: r.Header.Clone(),
		})
		index := served
		if r.URL.Path == path && served < len(pages) {
			served++
		}
		mu.Unlock()
		if r.URL.Path != path || index >= len(pages) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
			return
		}
		if index < len(pages)-1 {
			next := githubAPIBase + path + "?per_page=100&page=" + strconv.Itoa(index+2)
			w.Header().Set("Link", "<"+next+`>; rel="next"`)
		}
		_, _ = w.Write([]byte(pages[index]))
	}))
	t.Cleanup(srv.Close)
	return newPATClientPointingAt(t, srv.URL), &recorded
}

// parseQueryValues decodes a raw query string, failing the test on malformed
// input so callers can assert on individual parameters.
func parseQueryValues(t *testing.T, rawQuery string) url.Values {
	t.Helper()
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatalf("parse query %q: %v", rawQuery, err)
	}
	return values
}
