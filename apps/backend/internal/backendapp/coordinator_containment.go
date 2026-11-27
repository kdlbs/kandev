package backendapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/secrets"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

var errContainmentReaderUnwired = errors.New("containment reader is not wired")

// containmentDeps are the production sources containment reads.
type containmentDeps struct {
	tasks       containmentTaskReader
	auth        coordinator.ContainmentAuthReader
	agents      coordinator.AgentProfileReader
	settings    settingsstore.Repository
	registry    containmentAgentRegistry
	credentials containmentCredentialValues
	secrets     secrets.SecretStore
}

type containmentTaskReader interface {
	coordinator.ContainmentExecutorReader
	ListTaskRepositories(ctx context.Context, taskID string) ([]*taskmodels.TaskRepository, error)
	GetRepository(ctx context.Context, id string) (*taskmodels.Repository, error)
	GetPrimarySession(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
}

type containmentAgentRegistry interface {
	Get(id string) (agents.Agent, bool)
}

type containmentCredentialValues interface {
	GetCredentialValue(ctx context.Context, key string) (string, error)
}

// containmentReaders builds the reader bundle. Credentials and Repositories are
// always set, so their absence never depends on optional wiring.
func containmentReaders(d containmentDeps) coordinator.ContainmentReaders {
	r := coordinator.ContainmentReaders{
		Auth:         d.auth,
		Agents:       d.agents,
		MCP:          settingsMCPReader{repo: d.settings},
		Secrets:      containmentSecrets{store: d.secrets},
		Credentials:  containmentCredentials{settings: d.settings, registry: d.registry, values: d.credentials},
		Repositories: containmentRepositories{tasks: d.tasks},
		Sessions:     containmentSessions{tasks: d.tasks},
	}
	if d.tasks != nil {
		r.Executors = d.tasks
	}
	return r
}

type settingsMCPReader struct{ repo settingsstore.Repository }

func (r settingsMCPReader) GetAgentProfileMcpConfig(ctx context.Context, profileID string) (*settingsmodels.AgentProfileMcpConfig, error) {
	if r.repo == nil {
		return nil, errContainmentReaderUnwired
	}
	return r.repo.GetAgentProfileMcpConfig(ctx, profileID)
}

type containmentSecrets struct{ store secrets.SecretStore }

func (s containmentSecrets) RevealSecret(ctx context.Context, id string) (string, time.Time, error) {
	if s.store == nil {
		return "", time.Time{}, errContainmentReaderUnwired
	}
	meta, err := s.store.Get(ctx, id)
	if err != nil {
		return "", time.Time{}, err
	}
	var value string
	if scoped, ok := s.store.(secrets.ScopedSecretStore); ok {
		value, err = scoped.RevealGlobal(ctx, id)
	} else {
		value, err = s.store.Reveal(ctx, id)
	}
	if err != nil {
		return "", time.Time{}, err
	}
	return value, meta.UpdatedAt, nil
}

type containmentCredentials struct {
	settings settingsstore.Repository
	registry containmentAgentRegistry
	values   containmentCredentialValues
}

// CredentialValues mirrors the launch: for each key in the agent runtime's
// RequiredEnv, a value that resolves is a definition and one that does not is
// skipped. A failed agent or registry lookup is an error.
func (c containmentCredentials) CredentialValues(ctx context.Context, agentID string) ([]coordinator.CredentialValue, error) {
	if c.settings == nil || c.registry == nil || c.values == nil {
		return nil, errContainmentReaderUnwired
	}
	agent, err := c.settings.GetAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("read agent: %w", err)
	}
	def, ok := c.registry.Get(agent.Name)
	if !ok {
		return nil, fmt.Errorf("agent %q is not registered", agent.Name)
	}
	rt := def.Runtime()
	if rt == nil {
		return nil, nil
	}
	var out []coordinator.CredentialValue
	for _, key := range rt.RequiredEnv {
		value, err := c.values.GetCredentialValue(ctx, key)
		if err != nil || value == "" {
			continue
		}
		out = append(out, coordinator.CredentialValue{Key: key, Value: value})
	}
	return out, nil
}

type containmentRepositories struct{ tasks containmentTaskReader }

func (r containmentRepositories) HasRepositoryBinding(ctx context.Context, taskID string) (bool, error) {
	if r.tasks == nil {
		return false, errContainmentReaderUnwired
	}
	links, err := r.tasks.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, tr := range links {
		repo, err := r.tasks.GetRepository(ctx, tr.RepositoryID)
		if err != nil {
			return false, err
		}
		if len(repo.SecretBindings) > 0 {
			return true, nil
		}
	}
	return false, nil
}

type containmentSessions struct{ tasks containmentTaskReader }

func (s containmentSessions) ConversationSession(ctx context.Context, taskID string) (*coordinator.ContainmentSession, error) {
	if s.tasks == nil {
		return nil, errContainmentReaderUnwired
	}
	session, err := s.tasks.GetPrimarySession(ctx, taskID)
	if errors.Is(err, repoerrors.ErrNoPrimarySession) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &coordinator.ContainmentSession{State: string(session.State), StartedAt: session.StartedAt}, nil
}

// wireCoordinatorContainment injects the containment checker and the
// unattended-permission seams on the service. It runs only while phase 3 is
// effective. Registered orchestrator handlers and resolver are set once.
func wireCoordinatorContainment(svc *coordinator.Service, d containmentDeps, orch *orchestrator.Service, log *logger.Logger) {
	svc.SetContainment(coordinator.NewContainmentChecker(containmentReaders(d), log))
	if orch == nil {
		return
	}
	svc.SetUnattendedPermissionResolver(orch)
	orch.SetUnattendedPermissionHandler(svc.HandleUnattendedPermission)
}

// containmentDepsFrom collects the production sources from the route
// parameters. A nil source stays a nil interface so the readers fail closed.
func containmentDepsFrom(p routeParams) containmentDeps {
	var d containmentDeps
	if p.taskSvc != nil {
		d.tasks = p.taskSvc
	}
	if p.authSvc != nil {
		d.auth = p.authSvc
	}
	if p.agentSettingsRepo != nil {
		d.agents = p.agentSettingsRepo
		d.settings = p.agentSettingsRepo
	}
	if p.agentRegistry != nil {
		d.registry = p.agentRegistry
	}
	if p.lifecycleMgr != nil {
		if creds := p.lifecycleMgr.CredentialsManager(); creds != nil {
			d.credentials = creds
		}
	}
	d.secrets = p.secretStore
	return d
}
