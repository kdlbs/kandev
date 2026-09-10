package retention

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// CensusState is the tri-state freshness of a thresholded table's retained
// count (AC-OFFICE-RUN-HISTORY-RETENTION-003.11): zero is a real
// measurement, so "nobody has counted yet" cannot be represented as a
// count of zero.
type CensusState int

const (
	// CensusNotComputed is the state before the first evaluation ever
	// succeeds for a table. AC-OFFICE-RUN-HISTORY-RETENTION-003.7 and
	// -004.8 both require the surface to render this honestly rather than
	// as a zero count.
	CensusNotComputed CensusState = iota
	// CensusFresh means RetainedCount reflects the most recent evaluation,
	// which succeeded.
	CensusFresh
	// CensusStale means the most recent evaluation failed, and the fields
	// below are carried over unchanged from the last one that succeeded
	// (AC-OFFICE-RUN-HISTORY-RETENTION-003.11's "keeps the last successful
	// counts, reports them stale" — extended to the unknown-status warning
	// riding the same query, since neither the requirement nor the design
	// says the two field groups should diverge on a failed evaluation).
	CensusStale
)

var censusStateNames = map[CensusState]string{
	CensusNotComputed: "not_computed",
	CensusFresh:       "fresh",
	CensusStale:       "stale",
}

// MarshalJSON renders the state as its stable wire name rather than the
// underlying int, so an HTTP consumer never has to hardcode 0/1/2.
func (s CensusState) MarshalJSON() ([]byte, error) {
	name, ok := censusStateNames[s]
	if !ok {
		return nil, fmt.Errorf("retention: unknown census state %d", s)
	}
	return json.Marshal(name)
}

// UnmarshalJSON accepts only the names MarshalJSON produces.
func (s *CensusState) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}
	for state, candidate := range censusStateNames {
		if candidate == name {
			*s = state
			return nil
		}
	}
	return fmt.Errorf("retention: unknown census state %q", name)
}

// UnknownStatusCount is one status this package does not recognize, with
// the number of retained rows currently holding it
// (AC-OFFICE-RUN-HISTORY-RETENTION-001.10).
type UnknownStatusCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

// TableCensus is one thresholded table's retained-count evaluation.
type TableCensus struct {
	State           CensusState          `json:"state"`
	RetainedCount   int64                `json:"retained_count"`
	AsOf            time.Time            `json:"as_of"`
	UnknownStatuses []UnknownStatusCount `json:"unknown_statuses,omitempty"`  // ascending by status; only populated for status-bearing tables
	TopRoutineID    string               `json:"top_routine_id,omitempty"`    // office_routine_runs only; empty when not applicable
	TopRoutineShare float64              `json:"top_routine_share,omitempty"` // top routine's retained rows / table's retained count
}

// RetainedCounts holds the current census result for every thresholded
// table (AC-OFFICE-RUN-HISTORY-RETENTION-003.11).
type RetainedCounts struct {
	OfficeRoutineRuns TableCensus `json:"office_routine_runs"`
	Runs              TableCensus `json:"runs"`
	RunEvents         TableCensus `json:"run_events"`
}

// summarizeStatusCensus turns a status->count breakdown into a retained
// count (the sum across every status, since "retained" is the table's
// current row count) and the status/count list, sorted ascending by
// status, for statuses belonging to neither the history nor the
// live-state set (AC-OFFICE-RUN-HISTORY-RETENTION-001.10).
func summarizeStatusCensus(counts map[string]int64, history, live []string) (retained int64, unknown []UnknownStatusCount) {
	known := make(map[string]bool, len(history)+len(live))
	for _, s := range history {
		known[s] = true
	}
	for _, s := range live {
		known[s] = true
	}
	for status, count := range counts {
		retained += count
		if !known[status] {
			unknown = append(unknown, UnknownStatusCount{Status: status, Count: count})
		}
	}
	sort.Slice(unknown, func(i, j int) bool { return unknown[i].Status < unknown[j].Status })
	return retained, unknown
}

// CensusTracker holds the latest RetainedCounts in memory, applying the
// tri-state rule per table: a successful evaluation replaces a table's
// entry and marks it fresh; a failed evaluation leaves an already-computed
// entry in place and marks it stale, or leaves a never-computed entry as
// not-yet-computed. One table's failure never touches another table's
// entry. Safe for concurrent use: Record* is called from the scheduler
// goroutine and Snapshot from HTTP handlers.
type CensusTracker struct {
	mu     sync.Mutex
	counts RetainedCounts
}

// NewCensusTracker returns a tracker with every table not yet computed.
func NewCensusTracker() *CensusTracker {
	return &CensusTracker{}
}

// Snapshot returns the current RetainedCounts.
func (t *CensusTracker) Snapshot() RetainedCounts {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counts
}

// RecordRoutineRuns applies an office_routine_runs evaluation outcome.
func (t *CensusTracker) RecordRoutineRuns(fresh TableCensus, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts.OfficeRoutineRuns = applyCensusResult(t.counts.OfficeRoutineRuns, fresh, err)
}

// RecordRuns applies a runs evaluation outcome.
func (t *CensusTracker) RecordRuns(fresh TableCensus, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts.Runs = applyCensusResult(t.counts.Runs, fresh, err)
}

// RecordRunEvents applies a run_events evaluation outcome.
func (t *CensusTracker) RecordRunEvents(fresh TableCensus, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts.RunEvents = applyCensusResult(t.counts.RunEvents, fresh, err)
}

func applyCensusResult(prev, fresh TableCensus, err error) TableCensus {
	if err != nil {
		if prev.State == CensusNotComputed {
			return prev
		}
		prev.State = CensusStale
		return prev
	}
	fresh.State = CensusFresh
	return fresh
}
