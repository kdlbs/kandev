package backendapp

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/github"
	shared "github.com/kandev/kandev/internal/orchestration/models"
)

type storedIntegrationHealth struct {
	id, status, revision string
	configured           bool
}

type capabilityIntegration struct {
	name string
	read func(context.Context, string) ([]storedIntegrationHealth, error)
}

func (a *assistantCapabilityReader) integrationCapabilities(ctx context.Context, q shared.CapabilityQuery) ([]shared.Capability, error) {
	rows := []shared.Capability{}
	for _, source := range a.integrations {
		observations, err := source.read(ctx, q.WorkspaceID)
		if err != nil {
			observations = []storedIntegrationHealth{{status: capabilityUnavailable}}
		}
		if len(observations) == 0 {
			observations = []storedIntegrationHealth{{status: "missing"}}
		}
		for _, health := range observations {
			id := source.name
			if health.id != "" {
				id += "/" + health.id
			}
			entry := catalogCapability("integration", id, source.name, q.WorkspaceID, health.revision)
			entry.Configured = health.configured
			entry.Health, entry.Reason = health.status, "stored_health_only"
			rows = append(rows, entry)
		}
	}
	return rows, nil
}

func storedHealth(id string, ok bool, checked *time.Time, updated time.Time) []storedIntegrationHealth {
	status := capabilityUnknown
	if checked != nil {
		status = capabilityDisconnected
		if ok {
			status = capabilityReady
		}
	}
	return []storedIntegrationHealth{{id: id, status: status, configured: true, revision: updated.Format(time.RFC3339Nano)}}
}

type integrationCapabilitySources struct{ services *Services }

func assistantIntegrationSources(s *Services) []capabilityIntegration {
	r := integrationCapabilitySources{services: s}
	return []capabilityIntegration{{"github", r.github}, {capabilityGitLabName, r.gitlab}, {capabilityJiraName, r.jira}, {capabilityLinearName, r.linear}, {capabilitySentryName, r.sentry}, {capabilityAzureName, r.azure}}
}

func (r integrationCapabilitySources) jira(ctx context.Context, workspace string) ([]storedIntegrationHealth, error) {
	if r.services.Jira == nil || r.services.Jira.Store() == nil {
		return nil, nil
	}
	c, err := r.services.Jira.Store().GetConfigForWorkspace(ctx, workspace)
	if err != nil || c == nil {
		return nil, err
	}
	return storedHealth("", c.LastOk, c.LastCheckedAt, c.UpdatedAt), nil
}

func (r integrationCapabilitySources) linear(ctx context.Context, workspace string) ([]storedIntegrationHealth, error) {
	if r.services.Linear == nil || r.services.Linear.Store() == nil {
		return nil, nil
	}
	c, err := r.services.Linear.Store().GetConfigForWorkspace(ctx, workspace)
	if err != nil || c == nil {
		return nil, err
	}
	return storedHealth("", c.LastOk, c.LastCheckedAt, c.UpdatedAt), nil
}

func (r integrationCapabilitySources) sentry(ctx context.Context, workspace string) ([]storedIntegrationHealth, error) {
	if r.services.Sentry == nil || r.services.Sentry.Store() == nil {
		return nil, nil
	}
	configs, err := r.services.Sentry.Store().ListInstances(ctx, workspace)
	if err != nil {
		return nil, err
	}
	rows := []storedIntegrationHealth{}
	for _, c := range configs {
		rows = append(rows, storedHealth(c.ID, c.LastOk, c.LastCheckedAt, c.UpdatedAt)...)
	}
	return rows, nil
}

func (r integrationCapabilitySources) azure(ctx context.Context, workspace string) ([]storedIntegrationHealth, error) {
	if r.services.AzureDevOps == nil || r.services.AzureDevOps.Store() == nil {
		return nil, nil
	}
	c, err := r.services.AzureDevOps.Store().GetConfig(ctx, workspace)
	if err != nil || c == nil {
		return nil, err
	}
	return storedHealth("", c.LastOK, c.LastCheckedAt, c.UpdatedAt), nil
}

func (r integrationCapabilitySources) gitlab(ctx context.Context, workspace string) ([]storedIntegrationHealth, error) {
	if r.services.GitLab == nil {
		return nil, nil
	}
	c, err := r.services.GitLab.GetConfigForWorkspace(ctx, workspace)
	if err != nil || c == nil {
		return nil, err
	}
	return storedHealth("", c.LastOK, c.LastCheckedAt, c.UpdatedAt), nil
}

func (r integrationCapabilitySources) github(ctx context.Context, workspace string) ([]storedIntegrationHealth, error) {
	if r.services.GitHub == nil {
		return nil, nil
	}
	c, err := r.services.GitHub.WorkspaceConnectionMetadata(ctx, workspace)
	if err != nil || c == nil {
		return nil, err
	}
	status := capabilityDisconnected
	if c.Status == github.ConnectionStatusActive {
		status = capabilityReady
	}
	return []storedIntegrationHealth{{configured: true, status: status, revision: fmt.Sprintf("%d:%s", c.CredentialGeneration, c.UpdatedAt.Format(time.RFC3339Nano))}}, nil
}
