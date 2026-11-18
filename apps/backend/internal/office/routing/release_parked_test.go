package routing

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestProvider_ReleaseParkedRunsClearsTheWorkspaceParks(t *testing.T) {
	p, repo := newProviderTest(nil, nil)
	if err := p.ReleaseParkedRuns(context.Background(), "ws-1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if !reflect.DeepEqual(repo.clearedParkedFor, []string{"ws-1"}) {
		t.Fatalf("cleared = %v, want [ws-1]", repo.clearedParkedFor)
	}
}

func TestProvider_ReleaseParkedRunsReturnsRepositoryErrors(t *testing.T) {
	p, repo := newProviderTest(nil, nil)
	repo.clearedParkedErr = errors.New("db down")
	if err := p.ReleaseParkedRuns(context.Background(), "ws-1"); !errors.Is(err, repo.clearedParkedErr) {
		t.Fatalf("err = %v, want wrapping db down", err)
	}
}
