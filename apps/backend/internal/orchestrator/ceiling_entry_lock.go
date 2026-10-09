package orchestrator

import (
	"context"
	"errors"
	"sync"

	"github.com/kandev/kandev/internal/task/models"
)

type ceilingEntryAdmissionLock struct {
	mu   chan struct{}
	refs int
}

type ceilingEntryAdmissionContextKey struct{}
type ceilingEntryDispatchCommitContextKey struct{}

// ErrCeilingEntryDispatchCommitted reports that a workflow route is already
// owned by an admitted provider dispatch. A competing transition must leave
// that route unchanged and retry after the dispatch settles.
var ErrCeilingEntryDispatchCommitted = errors.New("workflow route dispatch is already committed")

// ceilingEntryDispatchCommit is a process-local ownership fence. The durable
// deferred-launch claim survives a restart, while this marker closes the
// smaller gap between the final route read and provider I/O in this process.
type ceilingEntryDispatchCommit struct {
	binding models.CeilingWorkflowEntryBinding
}

// acquireCeilingEntryAdmissionLock serializes operations that read or write a
// task's committed workflow route and its ceiling deferred-launch record. A
// channel is used instead of a mutex so the lock can be acquired with the same
// simple release shape as the other orchestrator guards.
func (s *Service) acquireCeilingEntryAdmissionLock(taskID string) func() {
	if s == nil || taskID == "" {
		return func() {}
	}
	s.ceilingEntryAdmissionLocksMu.Lock()
	if s.ceilingEntryAdmissionLocks == nil {
		s.ceilingEntryAdmissionLocks = make(map[string]*ceilingEntryAdmissionLock)
	}
	entry := s.ceilingEntryAdmissionLocks[taskID]
	if entry == nil {
		entry = &ceilingEntryAdmissionLock{mu: make(chan struct{}, 1)}
		s.ceilingEntryAdmissionLocks[taskID] = entry
	}
	entry.refs++
	s.ceilingEntryAdmissionLocksMu.Unlock()

	entry.mu <- struct{}{}
	return func() {
		<-entry.mu
		s.ceilingEntryAdmissionLocksMu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(s.ceilingEntryAdmissionLocks, taskID)
		}
		s.ceilingEntryAdmissionLocksMu.Unlock()
	}
}

func ceilingEntryAdmissionLockHeld(ctx context.Context, taskID string) bool {
	if ctx == nil || taskID == "" {
		return false
	}
	owned, _ := ctx.Value(ceilingEntryAdmissionContextKey{}).(string)
	return owned == taskID
}

// lockCeilingEntryAdmission makes nested helpers re-entrant while keeping the
// ownership marker scoped to the context used inside the critical section.
func (s *Service) lockCeilingEntryAdmission(ctx context.Context, taskID string) (context.Context, func()) {
	if taskID == "" || ceilingEntryAdmissionLockHeld(ctx, taskID) {
		return ctx, func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	release := s.acquireCeilingEntryAdmissionLock(taskID)
	return context.WithValue(ctx, ceilingEntryAdmissionContextKey{}, taskID), release
}

func ceilingEntryDispatchCommitFromContext(ctx context.Context) *ceilingEntryDispatchCommit {
	if ctx == nil {
		return nil
	}
	commit, _ := ctx.Value(ceilingEntryDispatchCommitContextKey{}).(*ceilingEntryDispatchCommit)
	return commit
}

// commitCeilingEntryDispatch validates and records immutable workflow-entry
// ownership while the task admission lock is held. Provider and readiness I/O
// run after the lock is released, carrying the process-local commit marker in
// context so route writers can reject a successor during that I/O.
func (s *Service) commitCeilingEntryDispatch(
	ctx context.Context,
	taskID string,
	binding *models.CeilingWorkflowEntryBinding,
) (context.Context, func(), error) {
	if s == nil || taskID == "" || binding == nil {
		return ctx, func() {}, nil
	}
	if !binding.Valid() {
		return ctx, func() {}, errors.New("workflow entry dispatch binding is incomplete")
	}
	if existing := ceilingEntryDispatchCommitFromContext(ctx); existing != nil {
		return ctx, func() {}, nil
	}

	admissionCtx, releaseAdmission := s.lockCeilingEntryAdmission(ctx, taskID)
	validationCtx := withCeilingEntryBinding(admissionCtx, binding)
	if err := s.validateClaimedCeilingBinding(validationCtx, taskID, binding); err != nil {
		releaseAdmission()
		return ctx, func() {}, err
	}

	commit := &ceilingEntryDispatchCommit{binding: *binding}
	s.ceilingEntryDispatchCommitsMu.Lock()
	if s.ceilingEntryDispatchCommits == nil {
		s.ceilingEntryDispatchCommits = make(map[string]*ceilingEntryDispatchCommit)
	}
	if existing := s.ceilingEntryDispatchCommits[taskID]; existing != nil {
		s.ceilingEntryDispatchCommitsMu.Unlock()
		releaseAdmission()
		return ctx, func() {}, ErrCeilingEntryDispatchCommitted
	}
	s.ceilingEntryDispatchCommits[taskID] = commit
	s.ceilingEntryDispatchCommitsMu.Unlock()
	// The marker, not the admission lock, spans provider I/O. Releasing here
	// lets lifecycle and route readers acquire the task lock without allowing a
	// successor route to replace this committed binding.
	releaseAdmission()

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			s.ceilingEntryDispatchCommitsMu.Lock()
			if s.ceilingEntryDispatchCommits[taskID] == commit {
				delete(s.ceilingEntryDispatchCommits, taskID)
			}
			s.ceilingEntryDispatchCommitsMu.Unlock()
		})
	}
	// Do not return validationCtx itself: it carries the admission-lock marker,
	// but that lock was released before provider I/O. Rebuild the dispatch
	// context from the caller's context so nested route writers cannot mistake
	// the released lock for a held one.
	dispatchCtx := withCeilingEntryBinding(ctx, binding)
	return context.WithValue(dispatchCtx, ceilingEntryDispatchCommitContextKey{}, commit), release, nil
}

// workflowRouteMutationAllowed lets the route owner proceed while rejecting
// unrelated transitions after an admitted dispatch has committed its binding.
func (s *Service) workflowRouteMutationAllowed(ctx context.Context, taskID string) error {
	if s == nil || taskID == "" {
		return nil
	}
	s.ceilingEntryDispatchCommitsMu.Lock()
	commit := s.ceilingEntryDispatchCommits[taskID]
	s.ceilingEntryDispatchCommitsMu.Unlock()
	if commit == nil || ceilingEntryDispatchCommitFromContext(ctx) == commit {
		return nil
	}
	return ErrCeilingEntryDispatchCommitted
}
