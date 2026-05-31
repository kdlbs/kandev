package oslimits

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/health"
)

// stubProbe is a test double for the Probe interface.
type stubProbe struct {
	samples []Sample
	err     error
}

func (s *stubProbe) Name() string                                { return "test" }
func (s *stubProbe) Category() string                            { return categoryID }
func (s *stubProbe) Samples(_ context.Context) ([]Sample, error) { return s.samples, s.err }

func makeInstanceSample(ratio float64) Sample {
	limit := uint64(128)
	used := uint64(float64(limit) * ratio)
	return Sample{
		ID:         sampleIDInotifyInstances,
		Name:       "Inotify instances",
		Unit:       unitInstances,
		Used:       used,
		Limit:      limit,
		UsageRatio: ratio,
		Supported:  true,
	}
}

func makeWatchSample(ratio float64) Sample {
	limit := uint64(8192)
	used := uint64(float64(limit) * ratio)
	return Sample{
		ID:         sampleIDInotifyWatches,
		Name:       "Inotify watches",
		Unit:       unitWatches,
		Used:       used,
		Limit:      limit,
		UsageRatio: ratio,
		Supported:  true,
	}
}

func TestOSLimitsChecker_NoIssuesBelowThreshold(t *testing.T) {
	probe := &stubProbe{samples: []Sample{makeInstanceSample(0.79)}}
	checker := NewOSLimitsChecker(probe)

	issues := checker.Check(context.Background())
	if len(issues) != 0 {
		t.Errorf("expected 0 issues at 79%%, got %d", len(issues))
	}
}

func TestOSLimitsChecker_WarningAt80Pct(t *testing.T) {
	probe := &stubProbe{samples: []Sample{makeInstanceSample(0.80)}}
	checker := NewOSLimitsChecker(probe)

	issues := checker.Check(context.Background())
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue at 80%%, got %d", len(issues))
	}
	if issues[0].Severity != health.SeverityWarning {
		t.Errorf("severity = %q, want warning", issues[0].Severity)
	}
}

func TestOSLimitsChecker_ErrorAt95Pct(t *testing.T) {
	probe := &stubProbe{samples: []Sample{makeInstanceSample(0.95)}}
	checker := NewOSLimitsChecker(probe)

	issues := checker.Check(context.Background())
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue at 95%%, got %d", len(issues))
	}
	if issues[0].Severity != health.SeverityError {
		t.Errorf("severity = %q, want error", issues[0].Severity)
	}
}

func TestOSLimitsChecker_UnsupportedProbe(t *testing.T) {
	probe := &stubProbe{samples: []Sample{
		{ID: sampleIDInotifyInstances, Supported: false, UsageRatio: 0.99},
	}}
	checker := NewOSLimitsChecker(probe)

	issues := checker.Check(context.Background())
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for unsupported probe, got %d", len(issues))
	}
}

func TestOSLimitsChecker_ProbeError(t *testing.T) {
	probe := &stubProbe{err: errors.New("proc unavailable")}
	checker := NewOSLimitsChecker(probe)

	// Must not panic and must return no issues.
	issues := checker.Check(context.Background())
	if len(issues) != 0 {
		t.Errorf("expected 0 issues on probe error, got %d", len(issues))
	}
}

func TestOSLimitsChecker_MultipleIssues(t *testing.T) {
	probe := &stubProbe{samples: []Sample{
		makeInstanceSample(0.85),
		makeWatchSample(0.90),
	}}
	checker := NewOSLimitsChecker(probe)

	issues := checker.Check(context.Background())
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
}

func TestOSLimitsChecker_IssueIDs(t *testing.T) {
	probe := &stubProbe{samples: []Sample{
		makeInstanceSample(0.85),
		makeWatchSample(0.85),
	}}
	checker := NewOSLimitsChecker(probe)

	issues := checker.Check(context.Background())
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	ids := map[string]bool{}
	for _, iss := range issues {
		ids[iss.ID] = true
	}
	if !ids["os_inotify_instances_high"] {
		t.Errorf("expected issue ID %q not found in %v", "os_inotify_instances_high", ids)
	}
	if !ids["os_inotify_watches_high"] {
		t.Errorf("expected issue ID %q not found in %v", "os_inotify_watches_high", ids)
	}
}

func TestOSLimitsChecker_TopConsumersInMessage(t *testing.T) {
	s := makeInstanceSample(0.85)
	s.TopConsumers = []Consumer{
		{PID: 42, Command: "node", FDCount: 5},
	}
	probe := &stubProbe{samples: []Sample{s}}
	checker := NewOSLimitsChecker(probe)

	issues := checker.Check(context.Background())
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "node") {
		t.Errorf("message should contain consumer name, got %q", issues[0].Message)
	}
	if !strings.Contains(issues[0].Message, "pid 42") {
		t.Errorf("message should contain PID, got %q", issues[0].Message)
	}
}

func TestOSLimitsChecker_WatchConsumersShowWatchCount(t *testing.T) {
	s := makeWatchSample(0.85)
	s.TopConsumers = []Consumer{
		{PID: 99, Command: "webpack", FDCount: 2, WatchCount: 8412},
	}
	probe := &stubProbe{samples: []Sample{s}}
	checker := NewOSLimitsChecker(probe)

	issues := checker.Check(context.Background())
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "8412 watches") {
		t.Errorf("watches message should show WatchCount, got %q", issues[0].Message)
	}
	if strings.Contains(issues[0].Message, "2 fds") {
		t.Errorf("watches message must not show FDCount, got %q", issues[0].Message)
	}
}

func TestOSLimitsChecker_NameCategory(t *testing.T) {
	checker := NewOSLimitsChecker()
	if checker.Name() != "OS resource limits" {
		t.Errorf("Name() = %q, want %q", checker.Name(), "OS resource limits")
	}
	if checker.Category() != categoryID {
		t.Errorf("Category() = %q, want %q", checker.Category(), categoryID)
	}
}
