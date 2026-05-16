package main

import (
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	analyticsrepository "github.com/kandev/kandev/internal/analytics/repository"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	office "github.com/kandev/kandev/internal/office"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeservice "github.com/kandev/kandev/internal/office/service"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/slack"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
	utilitystore "github.com/kandev/kandev/internal/utility/store"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/worktree"
)

type Repositories struct {
	Task          *sqliterepo.Repository
	Analytics     analyticsrepository.Repository
	AgentSettings settingsstore.Repository
	User          userstore.Repository
	Notification  notificationstore.Repository
	Editor        editorstore.Repository
	Prompts       promptstore.Repository
	Utility       utilitystore.Repository
	Workflow      *workflowrepository.Repository
	Secrets       secrets.SecretStore
	Office        *officesqlite.Repository
}

type Services struct {
	Task         *taskservice.Service
	User         *userservice.Service
	Editor       *editorservice.Service
	Notification *notificationservice.Service
	Prompts      *promptservice.Service
	Utility      *utilityservice.Service
	Workflow     *workflowservice.Service
	GitHub       *github.Service
	Jira         *jira.Service
	Linear       *linear.Service
	Slack        *slack.Service
	Office       *officeservice.Service
	OfficeSvcs   *office.Services
	// OrchScheduler is the office SchedulerIntegration constructed by
	// startOfficeSchedulersAndGC. Exposed here so registerRoutes can
	// wire SetTaskContextProvider after the HandoffService is built.
	OrchScheduler *officeservice.SchedulerIntegration
	// WorktreeMgr is the worktree manager. Exposed so the office GC can
	// consult it as the authoritative inventory of live worktrees.
	WorktreeMgr *worktree.Manager
}
