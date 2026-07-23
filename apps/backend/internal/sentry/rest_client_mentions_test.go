package sentry

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestRESTClientListOrganizationsLimitedBoundsDiscovery(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("per_page"); got != "3" {
			t.Errorf("per_page = %q, want limit+1 probe 3", got)
		}
		_, _ = w.Write([]byte(`[
			{"id":"1","slug":"acme","name":"Acme"},
			{"id":"2","slug":"globex","name":"Globex"},
			{"id":"3","slug":"initech","name":"Initech"}
		]`))
	})
	client := pointTo(NewRESTClient(&SentryConfig{}, "tok"), ts.URL)

	organizations, truncated, err := client.ListOrganizationsLimited(context.Background(), 2)
	if err != nil {
		t.Fatalf("list organizations: %v", err)
	}
	if !truncated || len(organizations) != 2 || organizations[0].Slug != "acme" || organizations[1].Slug != "globex" {
		t.Fatalf("organizations = %#v, truncated = %v; want first two and truncation signal", organizations, truncated)
	}
}

func TestRESTClientSearchIssuesLimitedBoundsResponse(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("per_page"); got != "2" {
			t.Errorf("per_page = %q, want 2", got)
		}
		_, _ = fmt.Fprint(w, `[
			{"id":"1","shortId":"APP-1","title":"One","permalink":"https://sentry.example/issues/1"},
			{"id":"2","shortId":"APP-2","title":"Two","permalink":"https://sentry.example/issues/2"},
			{"id":"3","shortId":"APP-3","title":"Three","permalink":"https://sentry.example/issues/3"}
		]`)
	})
	client := pointTo(NewRESTClient(&SentryConfig{}, "tok"), ts.URL)

	result, err := client.SearchIssuesLimited(
		context.Background(),
		SearchFilter{OrgSlug: "acme", Query: `"panic"`},
		"",
		2,
	)
	if err != nil {
		t.Fatalf("search issues: %v", err)
	}
	if result == nil || len(result.Issues) != 2 || result.Issues[1].ID != "2" {
		t.Fatalf("result = %#v, want bounded first two issues", result)
	}
}
