package sqlite

import (
	"context"
	"errors"
	"testing"
)

func TestDeleteTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	err := repo.DeleteTaskEnvironment(context.Background(), "missing-environment")

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("DeleteTaskEnvironment error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}
