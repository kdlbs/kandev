package lsp

import (
	"context"
	"sync"
)

type pendingStartOperation struct {
	generation uint64
	cancel     context.CancelFunc
	active     bool
}

type languageSlot struct {
	opMu           sync.Mutex
	startMu        sync.Mutex
	runtime        *runtime
	lastGeneration uint64
	retired        bool

	// beforeStartRegistration is a test-only scheduling hook. Production
	// managers leave it nil.
	beforeStartRegistration func()

	nextStartToken uint64
	pendingStarts  map[uint64]*pendingStartOperation
	nextStopToken  uint64
	pendingStops   map[uint64]struct{}
}

// lockStartOperation registers before waiting for the operation slot. A Stop
// can therefore cancel both the active start and replacements already queued
// behind it without holding startMu across the potentially long opMu wait.
func (s *languageSlot) lockStartOperation(
	parent context.Context,
	generation uint64,
) (context.Context, func()) {
	if s.beforeStartRegistration != nil {
		s.beforeStartRegistration()
	}
	operationCtx, cancel := context.WithCancel(parent)
	s.startMu.Lock()
	s.nextStartToken++
	token := s.nextStartToken
	if s.pendingStarts == nil {
		s.pendingStarts = make(map[uint64]*pendingStartOperation)
	}
	pending := &pendingStartOperation{generation: generation, cancel: cancel}
	s.pendingStarts[token] = pending
	if len(s.pendingStops) > 0 {
		cancel()
	}
	s.startMu.Unlock()

	s.opMu.Lock()
	s.startMu.Lock()
	pending.active = true
	s.startMu.Unlock()
	return operationCtx, func() {
		s.opMu.Unlock()
		s.startMu.Lock()
		delete(s.pendingStarts, token)
		s.startMu.Unlock()
		cancel()
	}
}

// lockAfterCancelingStart installs a cancellation barrier until Stop owns the
// operation slot. New starts self-cancel behind the barrier; unaccepted starts
// are canceled regardless of generation because they have not won ordering.
func (s *languageSlot) lockAfterCancelingStart(generation uint64) {
	s.startMu.Lock()
	s.nextStopToken++
	stopToken := s.nextStopToken
	if s.pendingStops == nil {
		s.pendingStops = make(map[uint64]struct{})
	}
	s.pendingStops[stopToken] = struct{}{}
	for _, pending := range s.pendingStarts {
		if !pending.active || generation == 0 || generation >= pending.generation {
			pending.cancel()
		}
	}
	s.startMu.Unlock()

	s.opMu.Lock()
	s.startMu.Lock()
	delete(s.pendingStops, stopToken)
	s.startMu.Unlock()
}
