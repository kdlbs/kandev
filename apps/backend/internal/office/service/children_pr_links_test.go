package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"go.uber.org/zap"
)

// fakeTaskPRLister returns a static map / error for ListTaskPRsByTaskIDs.
type fakeTaskPRLister struct {
	got    []string
	result map[string][]TaskPRLink
	err    error
}

func (f *fakeTaskPRLister) ListTaskPRsByTaskIDs(_ context.Context, taskIDs []string) (map[string][]TaskPRLink, error) {
	f.got = append([]string{}, taskIDs...)
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func newServiceWithPRs(prs TaskPRLister) *Service {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return &Service{
		logger:  log.WithFields(zap.String("component", "test")),
		taskPRs: prs,
	}
}

func TestLookupChildPRLinks_NilLister(t *testing.T) {
	svc := newServiceWithPRs(nil)
	out := svc.lookupChildPRLinks(context.Background(), []sqlite.ChildSummary{
		{TaskID: "child-1"},
	})
	if len(out) != 0 {
		t.Errorf("expected empty map when lister nil, got %#v", out)
	}
}

func TestLookupChildPRLinks_HappyPath(t *testing.T) {
	prs := &fakeTaskPRLister{
		result: map[string][]TaskPRLink{
			"child-1": {
				{URL: "https://github.com/owner/repo/pull/42", Title: "Add /healthz", Number: 42, State: "open"},
			},
			"child-2": {
				{URL: "https://github.com/owner/repo/pull/43", Number: 43, State: "merged"},
				{URL: "https://github.com/owner/back/pull/9", Number: 9, State: "open"},
			},
		},
	}
	svc := newServiceWithPRs(prs)
	out := svc.lookupChildPRLinks(context.Background(), []sqlite.ChildSummary{
		{TaskID: "child-1"}, {TaskID: "child-2"}, {TaskID: ""}, // empty id is skipped
	})
	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %d (%#v)", len(out), out)
	}
	if got := out["child-1"]; len(got) != 1 || got[0] != "https://github.com/owner/repo/pull/42" {
		t.Errorf("child-1 PRs = %#v", got)
	}
	if got := out["child-2"]; len(got) != 2 {
		t.Errorf("child-2 PRs = %#v (want 2)", got)
	}
	// Empty task id is filtered before the lookup call.
	if len(prs.got) != 2 {
		t.Errorf("lister received %d ids, want 2", len(prs.got))
	}
}

func TestLookupChildPRLinks_LookupErrorIsBestEffort(t *testing.T) {
	prs := &fakeTaskPRLister{err: errors.New("db boom")}
	svc := newServiceWithPRs(prs)
	out := svc.lookupChildPRLinks(context.Background(), []sqlite.ChildSummary{
		{TaskID: "child-1"},
	})
	if len(out) != 0 {
		t.Errorf("expected empty map on lookup error, got %#v", out)
	}
}

func TestLookupChildPRLinks_DropsBlankURLs(t *testing.T) {
	prs := &fakeTaskPRLister{
		result: map[string][]TaskPRLink{
			"child-1": {{URL: ""}, {URL: "https://x/y/pull/1"}},
		},
	}
	svc := newServiceWithPRs(prs)
	out := svc.lookupChildPRLinks(context.Background(), []sqlite.ChildSummary{{TaskID: "child-1"}})
	if got := out["child-1"]; len(got) != 1 || got[0] != "https://x/y/pull/1" {
		t.Errorf("expected blank URL filtered, got %#v", got)
	}
}
