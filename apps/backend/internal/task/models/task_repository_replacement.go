package models

// TaskRepositoryReplacementSnapshot is the canonical state observed after task serialization.
type TaskRepositoryReplacementSnapshot struct {
	Repositories      []*TaskRepository
	EnvironmentExists bool
}
