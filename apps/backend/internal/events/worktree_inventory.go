package events

// WorktreeInventoryData is one filtered linked-worktree snapshot with the
// execution identity a consumer needs for authorization.
type WorktreeInventoryData struct {
	TaskID            string                       `json:"task_id"`
	SessionID         string                       `json:"session_id"`
	TaskEnvironmentID string                       `json:"task_environment_id,omitempty"`
	ExecutorType      string                       `json:"executor_type,omitempty"`
	RepositoryName    string                       `json:"repository_name,omitempty"`
	CommonDir         string                       `json:"common_dir"`
	Entries           []WorktreeInventoryEntryData `json:"entries"`
	ObservedAt        string                       `json:"observed_at"`
}

// WorktreeInventoryEntryData describes one linked worktree on a named branch.
type WorktreeInventoryEntryData struct {
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	HeadSHA string `json:"head_sha"`
}
