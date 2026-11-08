package coordinator

import (
	"context"
	"sync"
	"time"
)

// CitationState is the outcome of checking one citation.
type CitationState int

// Citation outcomes. Absent is confirmed absence or change; Unavailable means
// the check could not be made.
const (
	CitationPresent CitationState = iota
	CitationAbsent
	CitationUnavailable
)

const (
	citationDeadline    = 5 * time.Second
	citationParallelism = 4
)

// CitationChecker checks citations against the board and repositories without
// fetching. One checker serves one validation run, so it may capture each
// repository's HEAD once.
type CitationChecker interface {
	Check(ctx context.Context, c NoteCitation) CitationState
}

// NoteResolver opens a checker for a coordinator's workspace.
type NoteResolver interface {
	Open(ctx context.Context, workspaceID, coordinatorID string) CitationChecker
}

// SetNoteResolver wires the citation resolver; without one every citation is
// unavailable.
func (s *Service) SetNoteResolver(r NoteResolver) { s.noteResolver = r }

func (s *Service) openChecker(ctx context.Context, c *Coordinator) CitationChecker {
	if s.noteResolver == nil {
		return unavailableChecker{}
	}
	return s.noteResolver.Open(ctx, c.WorkspaceID, c.ID)
}

type unavailableChecker struct{}

func (unavailableChecker) Check(context.Context, NoteCitation) CitationState {
	return CitationUnavailable
}

// runCitationJobs checks every citation, at most citationParallelism at a time,
// starting them in order. When the deadline passes every unfinished citation
// counts as unavailable.
func runCitationJobs(ctx context.Context, checker CitationChecker, cites []NoteCitation, deadline time.Duration) []CitationState {
	out := make([]CitationState, len(cites))
	for i := range out {
		out[i] = CitationUnavailable
	}
	if len(cites) == 0 {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < citationParallelism && w < len(cites); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				if ctx.Err() != nil {
					continue
				}
				if st := checker.Check(ctx, cites[i]); ctx.Err() == nil {
					out[i] = st
				}
			}
		}()
	}
	for i := range cites {
		next <- i
	}
	close(next)
	wg.Wait()
	return out
}

// foldCitationStates reduces a note's citation states: any absence makes it
// stale, else any unavailable makes it unavailable, else valid. The lowest
// absent index is returned for write-time reporting (-1 when none).
func foldCitationStates(states []CitationState) (result string, firstAbsent int) {
	firstAbsent = -1
	unavailable := false
	for i, st := range states {
		switch st {
		case CitationAbsent:
			if firstAbsent < 0 {
				firstAbsent = i
			}
		case CitationUnavailable:
			unavailable = true
		}
	}
	switch {
	case firstAbsent >= 0:
		return validationStale, firstAbsent
	case unavailable:
		return validationUnavailable, -1
	}
	return validationValid, -1
}
