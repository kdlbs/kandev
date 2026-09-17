package backendapp

import (
	orchstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// Ownership is persisted data policy, not a runtime feature toggle. Install
// this before any HTTP, WS, MCP, session or execution access can be served.
func wireAssistantOwnership(tasks *taskservice.Service, repo *orchstore.Repository) {
	tasks.SetTaskAccessChecker(repo.AuthorizeTask)
}
