package metrics

import "time"

type Snapshot struct {
	Timestamp       time.Time        `json:"timestamp"`
	IntervalSeconds int              `json:"interval_seconds"`
	Sources         []SourceSnapshot `json:"sources"`
}

type SourceSnapshot struct {
	ID           string         `json:"id"`
	Label        string         `json:"label"`
	Kind         string         `json:"kind"`
	ExecutorType string         `json:"executor_type,omitempty"`
	SessionID    string         `json:"session_id,omitempty"`
	TaskID       string         `json:"task_id,omitempty"`
	Metrics      []MetricSample `json:"metrics"`
}

type MetricSample struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Unit      string   `json:"unit,omitempty"`
	Value     *float64 `json:"value,omitempty"`
	Available bool     `json:"available"`
	Error     string   `json:"error,omitempty"`
}
