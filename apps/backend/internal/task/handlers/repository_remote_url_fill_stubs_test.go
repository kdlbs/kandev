package handlers

import "context"

// Test doubles ignore the field-only clone URL fill.

func (r *cancellationListRepo) FillRepositoryRemoteURL(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *commandPanelHTTPRepo) FillRepositoryRemoteURL(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *executorRepo) FillRepositoryRemoteURL(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *foreignSessionRepo) FillRepositoryRemoteURL(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *messageAddSwitchRepo) FillRepositoryRemoteURL(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *mockRepository) FillRepositoryRemoteURL(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *moveTaskConflictRepo) FillRepositoryRemoteURL(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *restartHTTPRepo) FillRepositoryRemoteURL(context.Context, string, string) (bool, error) {
	return false, nil
}
