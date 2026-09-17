package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	settings "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const capabilityReady = "ready"
const capabilityUnknown = "unknown"
const capabilityUnavailable = "unavailable"
const capabilityConversation = "conversation"
const capabilityEffectRead = "read"
const capabilityEffectWrite = "write"
const capabilityDisconnected = "disconnected"
const capabilityGitLabName = "gitlab"
const capabilityJiraName = "jira"
const capabilityLinearName = "linear"
const capabilitySentryName = "sentry"
const capabilityAzureName = "azuredevops"
const capabilityTitleKey = "title"
const capabilityMessageKey = "message"
const capabilitySchemaType = "type"

type capabilityProfiles interface {
	ListAgents(context.Context) ([]*settings.Agent, error)
	ListAgentProfiles(context.Context, string) ([]*settings.AgentProfile, error)
	GetAgentProfile(context.Context, string) (*settings.AgentProfile, error)
	GetAgentProfileMcpConfig(context.Context, string) (*settings.AgentProfileMcpConfig, error)
}
type capabilityExecutors interface {
	ListExecutors(context.Context) ([]*taskmodels.Executor, error)
	ListAllExecutorProfiles(context.Context) ([]*taskmodels.ExecutorProfile, error)
}
type capabilityPlugins interface {
	AgentToolCatalog() (plugintools.Snapshot, error)
}

type assistantCapabilityReader struct {
	tasks        *taskservice.Service
	profiles     capabilityProfiles
	executors    capabilityExecutors
	plugins      capabilityPlugins
	integrations []capabilityIntegration
}

func (a *assistantCapabilityReader) ReadCapabilities(ctx context.Context, q shared.CapabilityQuery) (shared.CapabilityPage, error) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID != q.OwnerID || q.WorkspaceID == "" || a.tasks == nil {
		return shared.CapabilityPage{}, fmt.Errorf("capability scope unavailable")
	}
	if err := a.tasks.AuthorizeWorkspaceAccess(ctx, q.WorkspaceID); err != nil {
		return shared.CapabilityPage{}, err
	}
	rows := nativeAssistantCapabilities(q.WorkspaceID)
	readers := []func(context.Context, shared.CapabilityQuery) ([]shared.Capability, error){a.profileCapabilities, a.workflowCapabilities, a.executorCapabilities, a.pluginCapabilities, a.sessionCapabilities, a.integrationCapabilities}
	for _, read := range readers {
		entries, err := read(ctx, q)
		if err != nil {
			return shared.CapabilityPage{}, fmt.Errorf("capability source unavailable")
		}
		rows = append(rows, entries...)
	}
	return capabilityPage(rows, q)
}

func capabilityPage(rows []shared.Capability, q shared.CapabilityQuery) (shared.CapabilityPage, error) {
	slices.SortFunc(rows, func(a, b shared.Capability) int { return strings.Compare(a.ID, b.ID) })
	data, _ := json.Marshal(rows)
	generation := fmt.Sprintf("%x", sha256.Sum256(data))
	if q.Generation != "" && q.Generation != generation {
		return shared.CapabilityPage{}, shared.ErrCapabilityGeneration
	}
	page := shared.CapabilityPage{Entries: []shared.Capability{}, Generation: generation}
	limit := min(max(q.Limit, 1), 100)
	for _, entry := range rows {
		if entry.ID <= q.After || (q.Kind != "" && entry.Kind != q.Kind) {
			continue
		}
		if len(page.Entries) == limit {
			page.After = page.Entries[len(page.Entries)-1].ID
			break
		}
		page.Entries = append(page.Entries, entry)
	}
	return page, nil
}

func catalogCapability(kind, id, name, workspace, revision string) shared.Capability {
	return shared.Capability{ID: kind + "/" + id, Kind: kind, Name: safeCapabilityName(name), ResourceID: id, WorkspaceID: workspace, Revision: revision,
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Effect:      capabilityUnknown, Surfaces: []string{capabilityConversation}, Health: capabilityUnknown, Reason: "configuration_only", Configured: true}
}

func (a *assistantCapabilityReader) profileCapabilities(ctx context.Context, q shared.CapabilityQuery) ([]shared.Capability, error) {
	rows := []shared.Capability{}
	if a.profiles == nil {
		return rows, nil
	}
	agents, err := a.profiles.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	for _, agent := range agents {
		if agent.WorkspaceID != nil && *agent.WorkspaceID != "" && *agent.WorkspaceID != q.WorkspaceID {
			continue
		}
		profiles, err := a.profiles.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return nil, err
		}
		for _, profile := range profiles {
			if profile.DeletedAt != nil || profile.Role != "" || (profile.WorkspaceID != "" && profile.WorkspaceID != q.WorkspaceID) {
				continue
			}
			entry := catalogCapability("profile", profile.ID, profile.Name, q.WorkspaceID, profile.UpdatedAt.Format(time.RFC3339Nano))
			entry.ProfileID = profile.ID
			if !profile.Enabled {
				entry.Health, entry.Reason = capabilityUnavailable, "profile_disabled"
			}
			rows = append(rows, entry)
			mcp, err := a.configuredMCPCapabilities(ctx, q, profile)
			if err != nil {
				return nil, err
			}
			rows = append(rows, mcp...)
		}
	}
	return rows, nil
}

func (a *assistantCapabilityReader) workflowCapabilities(ctx context.Context, q shared.CapabilityQuery) ([]shared.Capability, error) {
	workflows, err := a.tasks.ListWorkflows(ctx, q.WorkspaceID, false)
	if err != nil {
		return nil, err
	}
	rows := []shared.Capability{}
	for _, workflow := range workflows {
		if workflow.Hidden || workflow.WorkspaceID != q.WorkspaceID {
			continue
		}
		entry := catalogCapability("workflow", workflow.ID, workflow.Name, q.WorkspaceID, workflow.UpdatedAt.Format(time.RFC3339Nano))
		entry.Health, entry.Reason = capabilityReady, "native_catalog"
		rows = append(rows, entry)
	}
	return rows, nil
}

func (a *assistantCapabilityReader) executorCapabilities(ctx context.Context, q shared.CapabilityQuery) ([]shared.Capability, error) {
	rows := []shared.Capability{}
	if a.executors == nil {
		return rows, nil
	}
	executors, err := a.executors.ListExecutors(ctx)
	if err != nil {
		return nil, err
	}
	parents := map[string]*taskmodels.Executor{}
	for _, executor := range executors {
		if executor.DeletedAt == nil {
			parents[executor.ID] = executor
		}
	}
	profiles, err := a.executors.ListAllExecutorProfiles(ctx)
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		parent := parents[profile.ExecutorID]
		if parent == nil {
			continue
		}
		revision := profile.UpdatedAt.Format(time.RFC3339Nano) + ":" + parent.UpdatedAt.Format(time.RFC3339Nano)
		entry := catalogCapability("executor", profile.ID, profile.Name, q.WorkspaceID, revision)
		entry.Effect = capabilityEffectWrite
		if parent.Status != taskmodels.ExecutorStatusActive {
			entry.Health, entry.Reason = capabilityUnavailable, "executor_inactive"
		}
		rows = append(rows, entry)
	}
	return rows, nil
}

func newAssistantCapabilityReader(repos *Repositories, services *Services) *assistantCapabilityReader {
	reader := &assistantCapabilityReader{tasks: services.Task, profiles: repos.AgentSettings, executors: repos.Task, integrations: assistantIntegrationSources(services)}
	if services.Plugins != nil {
		reader.plugins = services.Plugins
	}
	return reader
}

func safeCapabilityName(name string) string {
	value := []rune(redaction.NewRedactor().String(name))
	return string(value[:min(len(value), 256)])
}
