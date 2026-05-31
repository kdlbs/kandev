package oslimits

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/health"
)

const (
	warnThreshold  = 0.80
	errorThreshold = 0.95
	categoryID     = "system_resources"
	fixURL         = "/settings/system/status"
	fixLabel       = "View system status"
	unitInstances  = "instances"
	unitWatches    = "watches"

	sampleIDInotifyInstances = "inotify_instances"
	sampleIDInotifyWatches   = "inotify_watches"
)

// OSLimitsChecker implements health.Checker over a list of OS resource probes.
type OSLimitsChecker struct {
	probes []Probe
}

// NewOSLimitsChecker creates a checker backed by the given probes.
func NewOSLimitsChecker(probes ...Probe) *OSLimitsChecker {
	return &OSLimitsChecker{probes: probes}
}

// Name returns the user-facing label for this check.
func (c *OSLimitsChecker) Name() string { return "OS resource limits" }

// Category returns the issue category this checker emits issues under.
func (c *OSLimitsChecker) Category() string { return categoryID }

// Check runs all probes and returns issues for samples over threshold.
func (c *OSLimitsChecker) Check(ctx context.Context) []health.Issue {
	var issues []health.Issue
	for _, probe := range c.probes {
		samples, err := probe.Samples(ctx)
		if err != nil {
			// probe errors are non-fatal; the check simply produces no issue
			continue
		}
		for _, s := range samples {
			issues = append(issues, sampleIssues(s)...)
		}
	}
	return issues
}

// sampleIssues converts a Sample into 0 or 1 health issues based on thresholds.
func sampleIssues(s Sample) []health.Issue {
	if !s.Supported || s.UsageRatio < warnThreshold {
		return nil
	}
	severity := health.SeverityWarning
	if s.UsageRatio >= errorThreshold {
		severity = health.SeverityError
	}
	pct := int(s.UsageRatio * 100)
	title := fmt.Sprintf("%s limit nearly exhausted", s.Name)
	msg := buildMessage(s, pct)
	return []health.Issue{{
		ID:       issueID(s.ID),
		Category: categoryID,
		Title:    title,
		Message:  msg,
		Severity: severity,
		FixURL:   fixURL,
		FixLabel: fixLabel,
	}}
}

// issueID maps a sample ID to a health issue ID.
func issueID(sampleID string) string {
	return "os_" + sampleID + "_high"
}

func buildMessage(s Sample, pct int) string {
	base := fmt.Sprintf("%d/%d %s in use (%d%%)", s.Used, s.Limit, s.Unit, pct)
	base += unitAdvice(s.Unit)
	if len(s.TopConsumers) > 0 {
		base += " Top consumers: " + formatConsumers(s.TopConsumers)
	}
	return base
}

func unitAdvice(unit string) string {
	switch unit {
	case unitInstances:
		return ". Exhaustion causes new terminals, dev servers, and agent CLIs to fail or hang." +
			" To increase: sudo sysctl -w fs.inotify.max_user_instances=1024"
	case unitWatches:
		return ". Exhaustion prevents file watchers and dev servers from monitoring directories." +
			" To increase: sudo sysctl -w fs.inotify.max_user_watches=1048576"
	default:
		return ""
	}
}

func formatConsumers(consumers []Consumer) string {
	parts := make([]string, 0, len(consumers))
	for _, c := range consumers {
		parts = append(parts, formatConsumer(c))
	}
	return strings.Join(parts, ", ")
}

func formatConsumer(c Consumer) string {
	if c.Command != "" {
		return fmt.Sprintf("%s (pid %d, %d fds)", c.Command, c.PID, c.FDCount)
	}
	return fmt.Sprintf("pid %d (%d fds)", c.PID, c.FDCount)
}
