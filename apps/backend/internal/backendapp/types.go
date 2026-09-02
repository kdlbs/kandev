package backendapp

import (
	"errors"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	analyticsrepository "github.com/kandev/kandev/internal/analytics/repository"
	authservice "github.com/kandev/kandev/internal/auth"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/azuredevops"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	tasklsp "github.com/kandev/kandev/internal/lsp"
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	office "github.com/kandev/kandev/internal/office"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/plugins"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	quickterminalrepository "github.com/kandev/kandev/internal/quickterminal/repository"
	"github.com/kandev/kandev/internal/runtimeflags"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/sentry"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/share"
	terminalrepo "github.com/kandev/kandev/internal/terminal/repository"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
	utilitystore "github.com/kandev/kandev/internal/utility/store"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/workflowsync"
	"github.com/kandev/kandev/internal/worktree"
)

type Repositories struct {
	Task          *sqliterepo.Repository
	Analytics     analyticsrepository.Repository
	AgentSettings settingsstore.Repository
	User          userstore.Repository
	// UserAccounts is the account-management view of the same user store
	// (list/create/role/status), consumed by the auth service.
	UserAccounts  userstore.AccountRepository
	Notification  notificationstore.Repository
	Editor        editorstore.Repository
	Prompts       promptstore.Repository
	Utility       utilitystore.Repository
	Workflow      *workflowrepository.Repository
	Secrets       secrets.SecretStore
	Office        *officesqlite.Repository
	Terminal      *terminalrepo.Repository
	QuickTerminal *quickterminalrepository.Repository
	RuntimeFlags  *runtimeflags.SQLiteStore
	// Auth persists login identities, sessions, PATs, and invites.
	Auth *authstore.Store
}

type Services struct {
	ManagedRuntimeSelections managedruntime.SelectionStore
	DynamicProfileResolver   *agentruntime.ProfileExecutionResolver
	DynamicBindingResolver   *dynamicruntime.CredentialBindingResolver
	Task                     *taskservice.Service
	User                     *userservice.Service
	Editor                   *editorservice.Service
	Notification             *notificationservice.Service
	Prompts                  *promptservice.Service
	Utility                  *utilityservice.Service
	Workflow                 *workflowservice.Service
	GitHub                   *github.Service
	GitLab                   *gitlab.Service
	GitLabCleanup            func() error
	AzureDevOps              *azuredevops.Service
	Jira                     *jira.Service
	Linear                   *linear.Service
	Sentry                   *sentry.Service
	// WorkflowSync keeps workspace workflows in sync with definition files
	// in a configured GitHub repository. Nil when GitHub is unavailable.
	WorkflowSync *workflowsync.Service
	Share        *share.HTTPHandlers
	Office       *officeservice.Service
	OfficeSvcs   *office.Services
	// OrchScheduler is the office SchedulerIntegration constructed by
	// startOfficeSchedulersAndGC. Exposed here so registerRoutes can
	// wire SetTaskContextProvider after the HandoffService is built.
	OrchScheduler *officeservice.SchedulerIntegration
	// WorktreeMgr is the worktree manager. Exposed so the office GC can
	// consult it as the authoritative inventory of live worktrees.
	WorktreeMgr *worktree.Manager
	// Terminal is the first-class user-terminal service (rename, park, etc.).
	// Wired into the gateway once lifecycle.Manager is up so the PTY backend
	// is available.
	Terminal     *terminalservice.Service
	RuntimeFlags *runtimeflags.Service
	// Automation is the trigger-based automation subsystem (cron, GitHub PR
	// events, webhooks). Independent of Office — has its own scheduler and
	// creates tasks via the task service.
	Automation *automation.Components
	// Plugins is the extensible plugin system service (registration
	// registry, event delivery, health monitoring). Always constructed
	// (non-nil) when initialization succeeds.
	Plugins *plugins.Service
	// GitCredentials is the shared provider-neutral lease broker used by the
	// GitHub HTTP endpoint and task executor helper leases.
	GitCredentials *gitcredentials.Broker
	// Mentions owns the provider registry shared by # search and submission authorization.
	Mentions *MentionComponents
	// Auth is the opt-in authentication service (mode state machine, sessions,
	// PATs, invites). Always non-nil; in disabled mode it only answers
	// Mode() == ModeDisabled and the middleware injects the synthetic identity.
	Auth *authservice.Service
	// TaskLSP is the single task/language lifecycle controller shared by
	// browser HTTP controls, WebSocket attachments, and lifecycle cleanup.
	TaskLSP *tasklsp.Controller
}

type schedulerStopper interface {
	Stop() error
}

// schedulingRuntime owns the backend-wide queue and cron loops. Keeping the
// handles together gives startup-failure cleanup and signal-driven shutdown
// the same idempotent stop path.
type schedulingRuntime struct {
	runs schedulerStopper
	cron schedulerStopper
}

func (s *schedulingRuntime) Stop() error {
	if s == nil {
		return nil
	}
	var errs []error
	if s.cron != nil {
		if err := s.cron.Stop(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.runs != nil {
		if err := s.runs.Stop(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
