package executor

import "context"

// Test doubles ignore the field-only clone URL fill.

func (r *mockRepository) FillRepositoryRemoteURL(context.Context, string, string) (bool, error) {
	return false, nil
}
