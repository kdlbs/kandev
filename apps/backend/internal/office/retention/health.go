package retention

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/health"
)

const (
	fixURL   = "/settings/system/data-storage"
	fixLabel = "Review retention settings"
)

// Checker implements health.Checker for office run history retention. Every
// issue is derived fresh from live state on each Check() — LastSweep and
// RetainedCounts are already the durable, tri-state views this design
// specifies (AC-OFFICE-RUN-HISTORY-RETENTION-004.6, -004.7, -003.11) — so
// there is no separate stored issue map to keep in sync with them.
//
// office_retention_count_failed:<table> is a ninth issue id beyond the
// design's closed eight-id catalogue: F33 (a Build-accepted spec gap) found
// no id for a failed census evaluation. It fires only on CensusStale (a
// table that had a successful evaluation and then failed); CensusNotComputed
// is the pre-first-success state AC-003.11 requires rendering as absent
// rather than alarming, so it raises nothing on its own.
//
// office_retention_threshold:<table> and office_retention_disabled:<table>
// both answer AC-003.5/-003.7's "retained count over threshold" condition,
// split by whether retention is enabled: AC-003.7 requires the disabled case
// to additionally state that retention is disabled, and the catalogue gives
// it its own id rather than a variable message under one id.
type Checker struct {
	settingsStore *SettingsStore
	sweeper       *Sweeper
	previewMarker *PreviewMarkerStore
}

// NewChecker wires the health checker to the package's own stores.
func NewChecker(settingsStore *SettingsStore, sweeper *Sweeper, previewMarker *PreviewMarkerStore) *Checker {
	return &Checker{settingsStore: settingsStore, sweeper: sweeper, previewMarker: previewMarker}
}

func (c *Checker) Name() string     { return "Office run retention" }
func (c *Checker) Category() string { return "office" }

func (c *Checker) Check(ctx context.Context) []health.Issue {
	var issues []health.Issue

	settings, err := c.settingsStore.GetSettings(ctx)
	if err != nil {
		issues = append(issues, issue(
			"office_retention_settings_invalid",
			"Retention settings unreadable",
			fmt.Sprintf("Stored retention settings could not be read; using the documented defaults. (%s)", err.Error()),
		))
	}

	if _, readable := c.previewMarker.Get(ctx); !readable {
		issues = append(issues, issue(
			"office_retention_preview_unreadable",
			"Retention preview marker unreadable",
			"The retention preview marker could not be read; office_routine_runs and runs will be previewed again on the next sweep rather than deleting.",
		))
	}

	if lastSweep, ok := c.sweeper.LastSweepSnapshot(); ok {
		issues = append(issues, sweptTableIssues(lastSweep, settings)...)
		issues = append(issues, failedTableIssues(lastSweep)...)
	}

	issues = append(issues, c.censusIssues(settings)...)

	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	return issues
}

// sweptTableIssues covers AC-003.2 (preview pending, one combined issue
// naming every swept table with a nonzero would-delete count) and AC-003.6
// (backlog, per swept table).
func sweptTableIssues(last LastSweep, settings Settings) []health.Issue {
	var issues []health.Issue

	type sweptEntry struct {
		table  TableName
		result SweptTableResult
		window int
	}
	entries := []sweptEntry{
		{TableOfficeRoutineRuns, last.OfficeRoutineRuns, settings.RoutineRuns.WindowDays},
		{TableRuns, last.Runs, settings.Runs.WindowDays},
	}

	var pending []string
	for _, e := range entries {
		if e.result.Previewed && e.result.WouldDelete > 0 {
			pending = append(pending, fmt.Sprintf("%s: %d rows under a %d-day window", e.table, e.result.WouldDelete, e.window))
		}
	}
	if len(pending) > 0 {
		issues = append(issues, issue(
			"office_retention_preview_pending",
			"Retention preview pending deletion",
			"Deletion begins at the next scheduled sweep: "+strings.Join(pending, "; ")+".",
		))
	}

	for _, e := range entries {
		if !e.result.Backlog {
			continue
		}
		issues = append(issues, issue(
			fmt.Sprintf("office_retention_backlog:%s", e.table),
			"Retention is behind",
			fmt.Sprintf("%s has more eligible rows than one sweep's batch limit; %d rows were deleted this sweep and retention remains behind.", e.table, e.result.Deleted),
		))
	}
	return issues
}

// failedTableIssues covers AC-002.7/AC-004.6's per-table sweep failure. Only
// office_routine_runs and runs ever carry a nonempty Err in the current
// sweep implementation — a satellite's own delete is never independently
// batched or retried — but every reported table is checked generically so a
// future failure mode on a satellite surfaces without a code change here.
func failedTableIssues(last LastSweep) []health.Issue {
	entries := []struct {
		table  TableName
		result TableSweepResult
	}{
		{TableOfficeRoutineRuns, last.OfficeRoutineRuns.TableSweepResult},
		{TableRuns, last.Runs.TableSweepResult},
		{TableRunEvents, last.RunEvents},
		{"office_run_route_attempts", last.RouteAttempts},
		{"office_run_skills", last.RunSkills},
	}
	var issues []health.Issue
	for _, e := range entries {
		if e.result.Err == "" {
			continue
		}
		issues = append(issues, issue(
			fmt.Sprintf("office_retention_failed:%s", e.table),
			"Retention sweep failed",
			fmt.Sprintf("The last sweep failed for %s: %s", e.table, e.result.Err),
		))
	}
	return issues
}

// censusIssues covers AC-001.10 (unknown status), AC-003.5/-003.7 (threshold,
// split on enabled/disabled), and F33's office_retention_count_failed.
func (c *Checker) censusIssues(settings Settings) []health.Issue {
	counts := c.sweeper.CensusSnapshot()

	entries := []struct {
		table    TableName
		census   TableCensus
		warnRows int
	}{
		{TableOfficeRoutineRuns, counts.OfficeRoutineRuns, settings.RoutineRuns.WarnRows},
		{TableRuns, counts.Runs, settings.Runs.WarnRows},
		{TableRunEvents, counts.RunEvents, settings.RunEvents.WarnRows},
	}

	var issues []health.Issue
	for _, e := range entries {
		if e.census.State == CensusStale {
			issues = append(issues, issue(
				fmt.Sprintf("office_retention_count_failed:%s", e.table),
				"Retained-row count evaluation failing",
				fmt.Sprintf("%s's retained-row count could not be re-evaluated; showing the last successful count from %s.", e.table, e.census.AsOf.Format(time.RFC3339)),
			))
		}
		if e.census.State == CensusNotComputed {
			continue
		}

		if len(e.census.UnknownStatuses) > 0 {
			issues = append(issues, issue(
				fmt.Sprintf("office_retention_unknown_status:%s", e.table),
				"Unrecognized status in retained rows",
				fmt.Sprintf("%s has rows with unrecognized status values, treated as live state and never pruned: %s.", e.table, strings.Join(e.census.UnknownStatuses, ", ")),
			))
		}

		if e.warnRows <= 0 || e.census.RetainedCount <= int64(e.warnRows) {
			continue
		}
		message := thresholdMessage(e.table, e.census, e.warnRows)
		if !settings.Enabled {
			issues = append(issues, issue(
				fmt.Sprintf("office_retention_disabled:%s", e.table),
				"Retention disabled with rows over threshold",
				message+" Retention is disabled, so this table is not being pruned.",
			))
			continue
		}
		issues = append(issues, issue(
			fmt.Sprintf("office_retention_threshold:%s", e.table),
			"Retained rows over threshold",
			message,
		))
	}
	return issues
}

func thresholdMessage(table TableName, census TableCensus, warnRows int) string {
	message := fmt.Sprintf("%s has %d retained rows, over its threshold of %d.", table, census.RetainedCount, warnRows)
	if census.TopRoutineID != "" {
		message += fmt.Sprintf(" Routine %s holds the largest share of retained rows, %.1f%%.", census.TopRoutineID, census.TopRoutineShare*100)
	}
	return message
}

func issue(id, title, message string) health.Issue {
	return health.Issue{
		ID:       id,
		Category: "office",
		Title:    title,
		Message:  message,
		Severity: health.SeverityWarning,
		FixURL:   fixURL,
		FixLabel: fixLabel,
	}
}
