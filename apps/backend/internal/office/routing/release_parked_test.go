package routing

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestProvider_ReleaseAuthorExclusionParkedRunsReleasesOnlyThoseSteps(t *testing.T) {
	p, repo := newProviderTest(nil, nil)
	if err := p.ReleaseAuthorExclusionParkedRuns(context.Background(), "ws-1", []string{"step-a"}); err != nil {
		t.Fatalf("release: %v", err)
	}
	if !reflect.DeepEqual(repo.releasedAuthorSteps, [][]string{{"step-a"}}) {
		t.Fatalf("released = %v, want [[step-a]]", repo.releasedAuthorSteps)
	}
	if len(repo.clearedParkedFor) != 0 {
		t.Fatalf("the workspace-wide clear must not run: %v", repo.clearedParkedFor)
	}
}

func TestProvider_ReleaseAuthorExclusionParkedRunsWithoutStepsDoesNothing(t *testing.T) {
	p, repo := newProviderTest(nil, nil)
	if err := p.ReleaseAuthorExclusionParkedRuns(context.Background(), "ws-1", nil); err != nil {
		t.Fatalf("release: %v", err)
	}
	if len(repo.releasedAuthorSteps) != 0 {
		t.Fatalf("released = %v, want no call", repo.releasedAuthorSteps)
	}
}

func TestProvider_ReleaseAuthorExclusionParkedRunsReturnsRepositoryErrors(t *testing.T) {
	p, repo := newProviderTest(nil, nil)
	repo.releasedAuthorErr = errors.New("db down")
	err := p.ReleaseAuthorExclusionParkedRuns(context.Background(), "ws-1", []string{"step-a"})
	if !errors.Is(err, repo.releasedAuthorErr) {
		t.Fatalf("err = %v, want wrapping db down", err)
	}
}
