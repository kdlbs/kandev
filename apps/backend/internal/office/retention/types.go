// Package retention bounds Office run history: office_routine_runs, runs,
// and the run satellite tables (run_events, office_run_route_attempts,
// office_run_skills). See docs/specs/office/requirements/run-history-retention.md
// and its operations counterpart for the frozen contract this package
// implements.
package retention

import (
	"errors"
	"fmt"
)

// ErrValidation is returned by NormalizeSettings when a field is outside its
// permitted range. The error message names the field.
var ErrValidation = errors.New("retention settings validation")

// ErrInvalidPersistedSettings wraps a stored settings document that could
// not be read or parsed. Callers fall back to DefaultSettings and surface a
// health issue rather than failing.
var ErrInvalidPersistedSettings = errors.New("invalid persisted retention settings")

// TableName identifies one of the tables retention reasons about.
type TableName string

const (
	TableOfficeRoutineRuns TableName = "office_routine_runs"
	TableRuns              TableName = "runs"
	TableRunEvents         TableName = "run_events"
)

// SweptTables are the two tables retention selects rows from by policy, in
// the fixed sweep order.
var SweptTables = []TableName{TableOfficeRoutineRuns, TableRuns}

// ReportedTables are the swept tables plus the three run satellites that a
// sweep can delete rows from.
var ReportedTables = []TableName{
	TableOfficeRoutineRuns, TableRuns,
	"run_events", "office_run_route_attempts", "office_run_skills",
}

// ThresholdedTables carry a warning threshold on retained row count.
var ThresholdedTables = []TableName{TableOfficeRoutineRuns, TableRuns, TableRunEvents}

// TableSettings is the per-table retention policy for a swept table.
type TableSettings struct {
	WindowDays    int `json:"window_days"`
	FloorPerOwner int `json:"floor_per_owner"`
	WarnRows      int `json:"warn_rows"`
}

// RunEventsSettings is the threshold-only policy for run_events, which has
// no window or floor of its own: its lifetime is its run's.
type RunEventsSettings struct {
	WarnRows int `json:"warn_rows"`
}

// Settings is the full retention policy document, persisted under one
// settings-store key as JSON.
type Settings struct {
	Enabled            bool              `json:"enabled"`
	SweepIntervalHours int               `json:"sweep_interval_hours"`
	BatchLimit         int               `json:"batch_limit"`
	RoutineRuns        TableSettings     `json:"routine_runs"`
	Runs               TableSettings     `json:"runs"`
	RunEvents          RunEventsSettings `json:"run_events"`
}

// DefaultSettings returns the documented AC-OFFICE-RUN-HISTORY-RETENTION-004.2
// defaults.
func DefaultSettings() Settings {
	return Settings{
		Enabled:            true,
		SweepIntervalHours: 6,
		BatchLimit:         5000,
		RoutineRuns:        TableSettings{WindowDays: 30, FloorPerOwner: 50, WarnRows: 25000},
		Runs:               TableSettings{WindowDays: 30, FloorPerOwner: 50, WarnRows: 25000},
		RunEvents:          RunEventsSettings{WarnRows: 250000},
	}
}

// Permitted ranges, AC-OFFICE-RUN-HISTORY-RETENTION-004.3.
const (
	minWindowDays = 1
	maxWindowDays = 3650

	minSweepIntervalHours = 1
	maxSweepIntervalHours = 168

	minFloorPerOwner = 0
	maxFloorPerOwner = 10000

	minBatchLimit = 100
	maxBatchLimit = 100000

	minWarnRows = 0
)

// NormalizeSettings validates every field against its permitted range and
// returns a field-named error on the first violation, changing nothing.
// Ranges are inclusive on both ends.
func NormalizeSettings(in Settings) (Settings, error) {
	if err := validateRange("sweep_interval_hours", in.SweepIntervalHours, minSweepIntervalHours, maxSweepIntervalHours); err != nil {
		return Settings{}, err
	}
	if err := validateRange("batch_limit", in.BatchLimit, minBatchLimit, maxBatchLimit); err != nil {
		return Settings{}, err
	}
	if err := validateTableSettings("routine_runs", in.RoutineRuns); err != nil {
		return Settings{}, err
	}
	if err := validateTableSettings("runs", in.Runs); err != nil {
		return Settings{}, err
	}
	if err := validateMin("run_events.warn_rows", in.RunEvents.WarnRows, minWarnRows); err != nil {
		return Settings{}, err
	}
	return in, nil
}

func validateTableSettings(prefix string, s TableSettings) error {
	if err := validateRange(prefix+".window_days", s.WindowDays, minWindowDays, maxWindowDays); err != nil {
		return err
	}
	if err := validateRange(prefix+".floor_per_owner", s.FloorPerOwner, minFloorPerOwner, maxFloorPerOwner); err != nil {
		return err
	}
	if err := validateMin(prefix+".warn_rows", s.WarnRows, minWarnRows); err != nil {
		return err
	}
	return nil
}

func validateRange(field string, value, minValue, maxValue int) error {
	if value < minValue || value > maxValue {
		return fmt.Errorf("%w: %s must be between %d and %d", ErrValidation, field, minValue, maxValue)
	}
	return nil
}

func validateMin(field string, value, minValue int) error {
	if value < minValue {
		return fmt.Errorf("%w: %s must be %d or greater", ErrValidation, field, minValue)
	}
	return nil
}
