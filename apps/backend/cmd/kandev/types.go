package main

import (
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
)

type Repositories struct {
	Task          repository.Repository
	TaskImpl      *repository.SQLiteRepository
	AgentSettings *settingsstore.SQLiteRepository
	User          *userstore.SQLiteRepository
	Notification  *notificationstore.SQLiteRepository
	Editor        *editorstore.SQLiteRepository
}

type Services struct {
	Task         *taskservice.Service
	User         *userservice.Service
	Editor       *editorservice.Service
	Notification *notificationservice.Service
}
