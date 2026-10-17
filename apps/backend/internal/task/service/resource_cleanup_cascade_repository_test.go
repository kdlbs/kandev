package service

import (
	"context"
	"errors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type failingCascadeMutationRepository struct {
	*sqliterepo.Repository
	failTaskID  string
	failArchive bool
	failDelete  bool
}

func (r *failingCascadeMutationRepository) ArchiveTaskIfActive(
	ctx context.Context,
	taskID, cascadeID string,
) (bool, error) {
	if r.failArchive && taskID == r.failTaskID {
		return false, errors.New("injected archive failure")
	}
	return r.Repository.ArchiveTaskIfActive(ctx, taskID, cascadeID)
}

func (r *failingCascadeMutationRepository) DeleteTask(ctx context.Context, taskID string) error {
	if r.failDelete && taskID == r.failTaskID {
		return errors.New("injected delete failure")
	}
	return r.Repository.DeleteTask(ctx, taskID)
}

func (r *failingCascadeMutationRepository) ArchiveTaskIfActiveWithVacatedStep(ctx context.Context, taskID, cascadeID string) (string, bool, error) {
	if r.failArchive && taskID == r.failTaskID {
		return "", false, errors.New("injected archive failure")
	}
	return r.Repository.ArchiveTaskIfActiveWithVacatedStep(ctx, taskID, cascadeID)
}

type commitThenErrorCascadeRepository struct {
	*sqliterepo.Repository
	err error
}

func (r *commitThenErrorCascadeRepository) ArchiveTaskIfActive(
	ctx context.Context,
	taskID, cascadeID string,
) (bool, error) {
	ok, err := r.Repository.ArchiveTaskIfActive(ctx, taskID, cascadeID)
	if err != nil {
		return false, err
	}
	return ok, r.err
}

func (r *commitThenErrorCascadeRepository) DeleteTask(ctx context.Context, taskID string) error {
	if err := r.Repository.DeleteTask(ctx, taskID); err != nil {
		return err
	}
	return r.err
}

func (r *commitThenErrorCascadeRepository) ArchiveTaskIfActiveWithVacatedStep(ctx context.Context, taskID, cascadeID string) (string, bool, error) {
	step, changed, err := r.Repository.ArchiveTaskIfActiveWithVacatedStep(ctx, taskID, cascadeID)
	if err != nil {
		return step, changed, err
	}
	return step, changed, r.err
}

func (r *commitThenErrorCascadeRepository) DeleteTaskWithVacatedStep(ctx context.Context, taskID string) (string, error) {
	step, err := r.Repository.DeleteTaskWithVacatedStep(ctx, taskID)
	if err != nil {
		return step, err
	}
	return step, r.err
}
func (r *failingCascadeMutationRepository) DeleteTaskWithVacatedStep(ctx context.Context, taskID string) (string, error) {
	if r.failDelete && taskID == r.failTaskID {
		return "", errors.New("injected delete failure")
	}
	return r.Repository.DeleteTaskWithVacatedStep(ctx, taskID)
}
