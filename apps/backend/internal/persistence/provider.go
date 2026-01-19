package persistence

import (
	agentsettingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agent/worktree"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/task/repository"
	userstore "github.com/kandev/kandev/internal/user/store"
)

type Provider interface {
	TaskRepo() (repository.Repository, error)
	AgentSettingsRepo() (agentsettingsstore.Repository, error)
	UserRepo() (userstore.Repository, error)
	NotificationRepo() (notificationstore.Repository, error)
	EditorRepo() (editorstore.Repository, error)
	WorktreeStore() (worktree.Store, error)
	Close() error
}
