package mergestate

import (
	"strconv"
	"strings"
	"time"
)

// Kind is a wake-producing condition of a change.
type Kind string

// Wake-producing condition kinds. Their wake kinds are change_<kind>.
const (
	KindRed      Kind = "red"
	KindConflict Kind = "conflict"
	KindThreads  Kind = "threads"
)

// Row is the stored episode state of one (change key, kind).
type Row struct {
	Active     bool
	Generation int
	HeadSHA    string
	// Baseline marks an episode that was already open at the first successful
	// observation. It never wakes and is replaced by the next real episode.
	Baseline   bool
	ObservedAt time.Time
}

// Observation is one successful provider read, timestamped at its start.
type Observation struct {
	StartedAt time.Time
	HeadSHA   string
	Pipeline  Pipeline
	Conflict  ConflictState
	// UnresolvedThreads is negative when the count could not be read.
	UnresolvedThreads int
}

// Result is the outcome of applying one observation to one condition row.
type Result struct {
	Row   Row
	Write bool
	Wake  bool
}

// EpisodeKey identifies one episode of a change; it keys wake uniqueness.
func EpisodeKey(changeKey string, generation int) string {
	return changeKey + "#" + strconv.Itoa(generation)
}

// ParseEpisodeKey splits an episode key into its change key and generation.
func ParseEpisodeKey(key string) (changeKey string, generation int, ok bool) {
	i := strings.LastIndex(key, "#")
	if i <= 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(key[i+1:])
	if err != nil || n < 1 {
		return "", 0, false
	}
	return key[:i], n, true
}

type reading int

const (
	readUnknown reading = iota
	readActive
	readClear
)

func readingOf(kind Kind, o Observation) reading {
	switch kind {
	case KindRed:
		switch o.Pipeline {
		case PipelineRed:
			return readActive
		case PipelineGreen:
			return readClear
		}
	case KindConflict:
		switch o.Conflict {
		case ConflictPresent:
			return readActive
		case ConflictClear:
			return readClear
		}
	case KindThreads:
		switch {
		case o.UnresolvedThreads > 0:
			return readActive
		case o.UnresolvedThreads == 0:
			return readClear
		}
	}
	return readUnknown
}

// Transition applies an observation to the stored row (nil for the first
// observation). An observation that started no later than the stored one is
// discarded, so a slower older read never overwrites a newer one.
func Transition(prev *Row, kind Kind, o Observation) Result {
	rd := readingOf(kind, o)
	if prev == nil {
		if rd == readUnknown {
			return Result{}
		}
		row := Row{Active: rd == readActive, Generation: 1, HeadSHA: o.HeadSHA, Baseline: true, ObservedAt: o.StartedAt}
		return Result{Row: row, Write: true}
	}
	if !o.StartedAt.After(prev.ObservedAt) {
		return Result{Row: *prev}
	}
	headMoved := kind != KindThreads && o.HeadSHA != "" && o.HeadSHA != prev.HeadSHA
	return transitionExisting(*prev, o, rd, headMoved)
}

func transitionExisting(prev Row, o Observation, rd reading, headMoved bool) Result {
	next := prev
	next.ObservedAt = o.StartedAt
	if headMoved {
		next.HeadSHA = o.HeadSHA
		next.Active = false
		next.Baseline = false
	}
	switch {
	case rd == readActive && !next.Active:
		next.Active, next.Baseline = true, false
		next.Generation = prev.Generation + 1
		return Result{Row: next, Write: true, Wake: true}
	case rd == readClear && next.Active:
		next.Active = false
		return Result{Row: next, Write: true}
	case headMoved:
		return Result{Row: next, Write: true}
	}
	return Result{Row: prev}
}
