package orchestrator

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type repoWithoutAdmittedLister struct{}

// TestNewSessionCeilingForRepoBindsARepositoryThatCanCount pins the production
// wiring: the controller the service holds must read the persisted half of the
// population, not just its own reservations.
func TestNewSessionCeilingForRepoBindsARepositoryThatCanCount(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1", "s2")

	controller := newSessionCeilingForRepo(lister, 4, zap.NewNop())
	if controller == nil {
		t.Fatal("newSessionCeilingForRepo returned nil")
	}
	if controller.ceiling != 4 {
		t.Fatalf("ceiling = %d, want 4", controller.ceiling)
	}
	got, err := controller.population(context.Background())
	if err != nil {
		t.Fatalf("population: %v", err)
	}
	if got != 2 {
		t.Fatalf("population = %d, want 2; the repository is not bound", got)
	}
}

// TestNewSessionCeilingForRepoWarnsWhenTheRepositoryCannotCount covers the
// degrade path. A repository that cannot enumerate admitted sessions leaves the
// controller counting only its own reservations, which under-counts every
// persisted session. That is the one failure here that is invisible at runtime,
// so it is announced rather than accepted silently.
func TestNewSessionCeilingForRepoWarnsWhenTheRepositoryCannotCount(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)

	controller := newSessionCeilingForRepo(repoWithoutAdmittedLister{}, 4, zap.New(core))
	if controller == nil {
		t.Fatal("newSessionCeilingForRepo returned nil")
	}
	if controller.lister != nil {
		t.Fatal("a repository without ListAdmittedSessionIDs must not be bound as a lister")
	}
	if n := logs.FilterMessageSnippet("cannot enumerate admitted sessions").Len(); n != 1 {
		t.Fatalf("expected exactly one WARN about the unbound repository, got %d: %v", n, logs.All())
	}
}
