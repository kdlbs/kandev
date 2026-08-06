package lsp

// ExecutorSupportsLSP deliberately fails closed. Support means the task host
// can supervise and reap the full process tree with the existing trusted LSP
// installer/cache boundary.
func ExecutorSupportsLSP(executorType string) bool {
	switch executorType {
	case "local", "worktree", "local_pc", "local_docker":
		return true
	default:
		return false
	}
}
