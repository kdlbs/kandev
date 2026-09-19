package alertsource

import (
	"encoding/json"
	"slices"
	"time"
)

// Alert status values. Normalize must set Status to one of these.
const (
	AlertStatusFiring   = "firing"
	AlertStatusResolved = "resolved"
)

// Alert is the normalized model every registered source produces. Its field
// set is taken from the Prometheus Alertmanager webhook payload shape, so a
// source already emitting that shape needs no adapter. Labels carries
// identity, Annotations carries prose, Context carries the body an agent
// needs, and Raw is the original payload retained for replay.
type Alert struct {
	SourceID     string
	SourceType   string
	Status       string
	Labels       map[string]string
	Annotations  map[string]string
	StartsAt     time.Time
	EndsAt       time.Time
	GeneratorURL string
	ExternalID   string
	Severity     string
	Context      string
	Raw          json.RawMessage
}

// Ended reports whether the alert has a resolution time. The zero value of
// EndsAt means "still firing"; no caller should re-derive that by comparing
// against time.Time{} directly.
func (a Alert) Ended() bool {
	return !a.EndsAt.IsZero()
}

// Clone returns a deep copy of a, safe for a caller to mutate without
// affecting the original. Enrich's contract requires the caller to pass a
// scratch copy so a failed enrichment cannot corrupt the alert the task is
// created from when enrichment fails (AC 002.7). A shallow copy would still
// alias Labels, Annotations (both maps) and Raw (a byte slice); Clone copies
// all three. Nil fields stay nil rather than becoming empty, so a source
// checking `a.Raw == nil` sees the same answer before and after cloning.
func (a Alert) Clone() Alert {
	clone := a
	clone.Labels = cloneStringMap(a.Labels)
	clone.Annotations = cloneStringMap(a.Annotations)
	if a.Raw != nil {
		clone.Raw = slices.Clone(a.Raw)
	}
	return clone
}

func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	clone := make(map[string]string, len(m))
	for k, v := range m {
		clone[k] = v
	}
	return clone
}
