package engine_adapters

import (
	"context"
	"errors"
	"testing"
)

type fakeAgentProfileExistenceRepo struct {
	exists bool
	err    error

	gotID string
}

func (f *fakeAgentProfileExistenceRepo) AgentProfileExists(_ context.Context, id string) (bool, error) {
	f.gotID = id
	return f.exists, f.err
}

func TestAgentProfileResolverAdapter_ExistsTrue(t *testing.T) {
	repo := &fakeAgentProfileExistenceRepo{exists: true}
	a := NewAgentProfileResolverAdapter(repo)

	got, err := a.AgentProfileExists(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Errorf("exists = false, want true")
	}
	if repo.gotID != "agent-1" {
		t.Errorf("repo called with id %q, want agent-1", repo.gotID)
	}
}

func TestAgentProfileResolverAdapter_ExistsFalse(t *testing.T) {
	repo := &fakeAgentProfileExistenceRepo{exists: false}
	a := NewAgentProfileResolverAdapter(repo)

	got, err := a.AgentProfileExists(context.Background(), "agent-deleted")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Errorf("exists = true, want false")
	}
}

func TestAgentProfileResolverAdapter_EmptyIDIsFalseWithoutCallingRepo(t *testing.T) {
	repo := &fakeAgentProfileExistenceRepo{exists: true}
	a := NewAgentProfileResolverAdapter(repo)

	got, err := a.AgentProfileExists(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Errorf("exists = true, want false for empty id")
	}
	if repo.gotID != "" {
		t.Errorf("repo should not have been called for empty id, got id %q", repo.gotID)
	}
}

func TestAgentProfileResolverAdapter_PropagatesRepoError(t *testing.T) {
	boom := errors.New("boom")
	repo := &fakeAgentProfileExistenceRepo{err: boom}
	a := NewAgentProfileResolverAdapter(repo)

	_, err := a.AgentProfileExists(context.Background(), "agent-1")
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want wrapping %v", err, boom)
	}
}
