package retention

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// retentionSettingsEnvelope captures each top-level field as raw JSON
// rather than a typed value, so a request body can be told apart into
// three cases per field: absent (nil RawMessage, take the documented
// default), present as the literal JSON null (rejected —
// AC-OFFICE-RUN-HISTORY-RETENTION-004.9), or present with a value (decoded
// and validated). A plain typed struct cannot distinguish the first two: an
// ordinary `*int` field is nil either way.
type retentionSettingsEnvelope struct {
	Enabled            json.RawMessage `json:"enabled"`
	SweepIntervalHours json.RawMessage `json:"sweep_interval_hours"`
	BatchLimit         json.RawMessage `json:"batch_limit"`
	RoutineRuns        json.RawMessage `json:"routine_runs"`
	Runs               json.RawMessage `json:"runs"`
	RunEvents          json.RawMessage `json:"run_events"`
}

type tableSettingsEnvelope struct {
	WindowDays    json.RawMessage `json:"window_days"`
	FloorPerOwner json.RawMessage `json:"floor_per_owner"`
	WarnRows      json.RawMessage `json:"warn_rows"`
}

type runEventsSettingsEnvelope struct {
	WarnRows json.RawMessage `json:"warn_rows"`
}

// decodeRetentionSettings implements AC-OFFICE-RUN-HISTORY-RETENTION-004.9's
// PUT semantics: a full replace where an omitted field takes its documented
// default, an unrecognized field or an explicit null is rejected naming the
// field, and nothing is written on any rejection (the caller is expected to
// not persist the zero-value Settings returned alongside a non-nil error).
// Range validation (AC-004.3) is NormalizeSettings's job, called by
// SettingsStore.SaveSettings after this decode succeeds.
func decodeRetentionSettings(body []byte) (Settings, error) {
	defaults := DefaultSettings()

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var env retentionSettingsEnvelope
	if err := dec.Decode(&env); err != nil {
		return Settings{}, err
	}
	if dec.More() {
		return Settings{}, fmt.Errorf("request body: unexpected data after the JSON object")
	}

	enabled, err := decodeBoolField(env.Enabled, "enabled", defaults.Enabled)
	if err != nil {
		return Settings{}, err
	}
	sweepIntervalHours, err := decodeIntField(env.SweepIntervalHours, "sweep_interval_hours", defaults.SweepIntervalHours)
	if err != nil {
		return Settings{}, err
	}
	batchLimit, err := decodeIntField(env.BatchLimit, "batch_limit", defaults.BatchLimit)
	if err != nil {
		return Settings{}, err
	}
	routineRuns, err := decodeTableSettings(env.RoutineRuns, "routine_runs", defaults.RoutineRuns)
	if err != nil {
		return Settings{}, err
	}
	runs, err := decodeTableSettings(env.Runs, "runs", defaults.Runs)
	if err != nil {
		return Settings{}, err
	}
	runEvents, err := decodeRunEventsSettings(env.RunEvents, "run_events", defaults.RunEvents)
	if err != nil {
		return Settings{}, err
	}

	return Settings{
		Enabled:            enabled,
		SweepIntervalHours: sweepIntervalHours,
		BatchLimit:         batchLimit,
		RoutineRuns:        routineRuns,
		Runs:               runs,
		RunEvents:          runEvents,
	}, nil
}

func decodeTableSettings(raw json.RawMessage, name string, defaults TableSettings) (TableSettings, error) {
	if raw == nil {
		return defaults, nil
	}
	if isJSONNull(raw) {
		return TableSettings{}, rejectNull(name)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var env tableSettingsEnvelope
	if err := dec.Decode(&env); err != nil {
		return TableSettings{}, fmt.Errorf("%s: %w", name, err)
	}
	windowDays, err := decodeIntField(env.WindowDays, name+".window_days", defaults.WindowDays)
	if err != nil {
		return TableSettings{}, err
	}
	floorPerOwner, err := decodeIntField(env.FloorPerOwner, name+".floor_per_owner", defaults.FloorPerOwner)
	if err != nil {
		return TableSettings{}, err
	}
	warnRows, err := decodeIntField(env.WarnRows, name+".warn_rows", defaults.WarnRows)
	if err != nil {
		return TableSettings{}, err
	}
	return TableSettings{WindowDays: windowDays, FloorPerOwner: floorPerOwner, WarnRows: warnRows}, nil
}

func decodeRunEventsSettings(raw json.RawMessage, name string, defaults RunEventsSettings) (RunEventsSettings, error) {
	if raw == nil {
		return defaults, nil
	}
	if isJSONNull(raw) {
		return RunEventsSettings{}, rejectNull(name)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var env runEventsSettingsEnvelope
	if err := dec.Decode(&env); err != nil {
		return RunEventsSettings{}, fmt.Errorf("%s: %w", name, err)
	}
	warnRows, err := decodeIntField(env.WarnRows, name+".warn_rows", defaults.WarnRows)
	if err != nil {
		return RunEventsSettings{}, err
	}
	return RunEventsSettings{WarnRows: warnRows}, nil
}

func decodeBoolField(raw json.RawMessage, name string, defaultVal bool) (bool, error) {
	if raw == nil {
		return defaultVal, nil
	}
	if isJSONNull(raw) {
		return false, rejectNull(name)
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, fmt.Errorf("%s: %w", name, err)
	}
	return v, nil
}

func decodeIntField(raw json.RawMessage, name string, defaultVal int) (int, error) {
	if raw == nil {
		return defaultVal, nil
	}
	if isJSONNull(raw) {
		return 0, rejectNull(name)
	}
	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return v, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func rejectNull(name string) error {
	return fmt.Errorf("%s: must not be null; omit the field to use its default", name)
}
