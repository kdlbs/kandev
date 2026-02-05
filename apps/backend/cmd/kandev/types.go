package main

import (
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	analyticsrepository "github.com/kandev/kandev/internal/analytics/repository"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

type Repositories struct {
	Task          repository.Repository
	Analytics     analyticsrepository.Repository
	AgentSettings settingsstore.Repository
	User          userstore.Repository
	Notification  notificationstore.Repository
	Editor        editorstore.Repository
	Prompts       promptstore.Repository
	Workflow      *workflowrepository.Repository
}

type Services struct {
	Task         *taskservice.Service
	User         *userservice.Service
	Editor       *editorservice.Service
	Notification *notificationservice.Service
	Prompts      *promptservice.Service
	Workflow     *workflowservice.Service
}
