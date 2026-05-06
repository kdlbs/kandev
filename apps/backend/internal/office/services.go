package office

import (
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/approvals"
	"github.com/kandev/kandev/internal/office/channels"
	"github.com/kandev/kandev/internal/office/config"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/infra"
	"github.com/kandev/kandev/internal/office/labels"
	"github.com/kandev/kandev/internal/office/onboarding"
	"github.com/kandev/kandev/internal/office/projects"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
	"github.com/kandev/kandev/internal/office/scheduler"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/skills"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// Services holds references to all feature services in the office domain.
// It is the central wiring point for HTTP handlers and background jobs.
type Services struct {
	Agents       *agents.AgentService
	Skills       *skills.SkillService
	Projects     *projects.ProjectService
	Costs        *costs.CostService
	Routines     *routines.RoutineService
	Approvals    *approvals.ApprovalService
	Channels     *channels.ChannelService
	Config       *config.ConfigService
	Dashboard    *dashboard.DashboardService
	Labels       *labels.LabelService
	Onboarding   *onboarding.OnboardingService
	Scheduler    *scheduler.SchedulerService
	TreeControls *officeservice.Service
	Workspaces   *officeservice.Service
	Documents    *taskservice.DocumentService
	GC           *infra.GarbageCollector
	Reconciler   *infra.Reconciler
	Repo         *sqlite.Repository
	GitManager   *configloader.GitManager
	// KandevHome is the base storage directory for attachment files.
	KandevHome string
}
