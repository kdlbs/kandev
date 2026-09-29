package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/profiles"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// Containment condition names, in the order Check returns them.
const (
	ConditionExecutorIsolated   = "executor_isolated"
	ConditionAuthEnabled        = "auth_enabled"
	ConditionNoKandevCredential = "no_kandev_credential"
	ConditionNoExtraTools       = "no_extra_tools"
)

// Condition details that are not an executor type or a count.
const (
	DetailUnreadable         = "unreadable"
	DetailChangedSinceLaunch = "changed_since_launch"
	DetailUnverifiedSource   = "unverified_source"
	DetailKandevAPIKey       = "KANDEV_API_KEY"
	DetailKandevRunToken     = "KANDEV_RUN_TOKEN"
	DetailPATValue           = "pat_value"
	detailAuthDisabled       = "disabled"
	detailAuthSetup          = "setup"
	kandevPATPrefix          = "kandev_pat_"
	launchStateCreated       = string(taskmodels.TaskSessionStateCreated)
)

// ContainmentCondition is one of the four conditions of a Result.
type ContainmentCondition struct {
	Name   string
	Met    bool
	Detail string
}

// ContainmentResult is the outcome of Check. Contained is true only when all
// four conditions are met; Conditions always has four entries in fixed order.
type ContainmentResult struct {
	Contained  bool
	Conditions []ContainmentCondition
}

// FirstUnmet returns the first unmet condition and true, or false when all
// conditions are met.
func (r ContainmentResult) FirstUnmet() (ContainmentCondition, bool) {
	for _, c := range r.Conditions {
		if !c.Met {
			return c, true
		}
	}
	return ContainmentCondition{}, false
}

// ContainmentExecutorReader resolves the coordinator's executor profile and
// its executor. Satisfied by the task service.
type ContainmentExecutorReader interface {
	GetExecutorProfile(ctx context.Context, id string) (*taskmodels.ExecutorProfile, error)
	GetExecutor(ctx context.Context, id string) (*taskmodels.Executor, error)
}

// ContainmentAuthReader reports the effective auth mode. Satisfied by
// auth.Service.
type ContainmentAuthReader interface {
	Mode() auth.Mode
}

// ContainmentMCPReader reads an agent profile's MCP configuration and returns
// sql.ErrNoRows when the profile has none. Satisfied by the agent settings
// store.
type ContainmentMCPReader interface {
	GetAgentProfileMcpConfig(ctx context.Context, profileID string) (*settingsmodels.AgentProfileMcpConfig, error)
}

// ContainmentSecretReader reveals a global secret and reports when it was last
// updated.
type ContainmentSecretReader interface {
	RevealSecret(ctx context.Context, secretID string) (value string, updatedAt time.Time, err error)
}

// CredentialValue is one environment variable the credentials manager would
// supply to an agent's launch.
type CredentialValue struct {
	Key   string
	Value string
}

// ContainmentCredentialReader returns the credentials-manager values the launch
// adds for an agent: one entry per required key that resolves to a non-empty
// value. A failed agent or registry lookup is an error.
type ContainmentCredentialReader interface {
	CredentialValues(ctx context.Context, agentID string) ([]CredentialValue, error)
}

// ContainmentRepositoryReader reports whether a task carries a repository
// binding environment variable.
type ContainmentRepositoryReader interface {
	HasRepositoryBinding(ctx context.Context, taskID string) (bool, error)
}

// ContainmentSession is the state and start time of a conversation session.
type ContainmentSession struct {
	State     string
	StartedAt time.Time
}

// ContainmentSessionReader returns the conversation task's session, or nil
// when it has none.
type ContainmentSessionReader interface {
	ConversationSession(ctx context.Context, taskID string) (*ContainmentSession, error)
}

// ContainmentReaders bundles the narrow readers Check needs. A nil Executors,
// Auth, Agents, MCP, Secrets or Sessions reader makes the conditions it feeds
// unreadable; a nil Credentials or Repositories reader makes condition 3
// unverified_source.
type ContainmentReaders struct {
	Executors    ContainmentExecutorReader
	Auth         ContainmentAuthReader
	Agents       AgentProfileReader
	MCP          ContainmentMCPReader
	Secrets      ContainmentSecretReader
	Credentials  ContainmentCredentialReader
	Repositories ContainmentRepositoryReader
	Sessions     ContainmentSessionReader
}

// ContainmentChecker evaluates the containment conditions. It caches nothing:
// every Check reads current state.
type ContainmentChecker struct {
	readers     ContainmentReaders
	log         *logger.Logger
	environment func() profiles.Environment
	state       admissionState
}

// NewContainmentChecker builds a checker over the given readers.
func NewContainmentChecker(readers ContainmentReaders, log *logger.Logger) *ContainmentChecker {
	return &ContainmentChecker{
		readers:     readers,
		log:         log,
		environment: profiles.DetectEnvironment,
		state:       newAdmissionState(),
	}
}

// Check evaluates all four conditions for the coordinator. A condition whose
// read fails is not met.
func (c *ContainmentChecker) Check(ctx context.Context, co *Coordinator) ContainmentResult {
	launch := c.launchOf(ctx, co)
	conditions := []ContainmentCondition{
		c.executorIsolated(ctx, co),
		c.authEnabled(),
		c.noKandevCredential(ctx, co, launch),
		c.noExtraTools(ctx, co, launch),
	}
	contained := true
	for _, cond := range conditions {
		contained = contained && cond.Met
	}
	return ContainmentResult{Contained: contained, Conditions: conditions}
}

func unmet(name, detail string) ContainmentCondition {
	return ContainmentCondition{Name: name, Detail: detail}
}

func isolatedExecutor(t taskmodels.ExecutorType, env profiles.Environment) bool {
	switch t {
	case taskmodels.ExecutorTypeLocalDocker, taskmodels.ExecutorTypeRemoteDocker,
		taskmodels.ExecutorTypeSprites, taskmodels.ExecutorTypeKubernetes:
		return true
	case taskmodels.ExecutorTypeMockRemote:
		return env == profiles.EnvE2E
	}
	return false
}

func (c *ContainmentChecker) executorIsolated(ctx context.Context, co *Coordinator) ContainmentCondition {
	if co == nil || c.readers.Executors == nil || ctx.Err() != nil {
		return unmet(ConditionExecutorIsolated, DetailUnreadable)
	}
	profile, err := c.readers.Executors.GetExecutorProfile(ctx, co.ExecutorProfileID)
	if err != nil || profile == nil {
		return unmet(ConditionExecutorIsolated, DetailUnreadable)
	}
	executor, err := c.readers.Executors.GetExecutor(ctx, profile.ExecutorID)
	if err != nil || executor == nil {
		return unmet(ConditionExecutorIsolated, DetailUnreadable)
	}
	return ContainmentCondition{
		Name:   ConditionExecutorIsolated,
		Met:    isolatedExecutor(executor.Type, c.environment()),
		Detail: string(executor.Type),
	}
}

func (c *ContainmentChecker) authEnabled() ContainmentCondition {
	if c.readers.Auth == nil {
		return unmet(ConditionAuthEnabled, DetailUnreadable)
	}
	switch c.readers.Auth.Mode() {
	case auth.ModeEnabled:
		return ContainmentCondition{Name: ConditionAuthEnabled, Met: true}
	case auth.ModeSetup:
		return unmet(ConditionAuthEnabled, detailAuthSetup)
	default:
		return unmet(ConditionAuthEnabled, detailAuthDisabled)
	}
}

func (c *ContainmentChecker) noExtraTools(ctx context.Context, co *Coordinator, launch launchInfo) ContainmentCondition {
	if co == nil || c.readers.Agents == nil || c.readers.MCP == nil || ctx.Err() != nil {
		return unmet(ConditionNoExtraTools, DetailUnreadable)
	}
	source, servers, ok := c.mcpSource(ctx, co.AgentProfileID)
	if !ok {
		return unmet(ConditionNoExtraTools, DetailUnreadable)
	}
	if detail := launch.currency(source); detail != "" {
		return unmet(ConditionNoExtraTools, detail)
	}
	if servers > 0 {
		return unmet(ConditionNoExtraTools, strconv.Itoa(servers))
	}
	return ContainmentCondition{Name: ConditionNoExtraTools, Met: true}
}

// mcpSource returns the updated_at that feeds launch currency and the number
// of configured servers. A disabled configuration counts no servers.
func (c *ContainmentChecker) mcpSource(ctx context.Context, profileID string) (time.Time, int, bool) {
	cfg, err := c.readers.MCP.GetAgentProfileMcpConfig(ctx, profileID)
	if err == nil && cfg != nil {
		if !cfg.Enabled {
			return cfg.UpdatedAt, 0, true
		}
		return cfg.UpdatedAt, len(cfg.Servers), true
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, 0, false
	}
	profile, perr := c.readers.Agents.GetAgentProfile(ctx, profileID)
	if perr != nil || profile == nil {
		return time.Time{}, 0, false
	}
	return profile.UpdatedAt, 0, true
}
