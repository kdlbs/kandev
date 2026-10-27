package sandboxruntime

import (
	"context"
	"net/url"
	"runtime"
	"strconv"
	"time"

	"github.com/google/uuid"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// CompileLearning uses frozen attempt facts. It neither opens a new sandbox nor grants access.
func (m *Manager) CompileLearning(ctx context.Context, attempt *policy.Attempt, records []policy.DenialRecord) (contract.Grants, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	captured := attempt.Snapshot.Provider
	binding, err := m.selectProvider(ctx, captured.InstallationID, captured.Key)
	if err != nil {
		return contract.Grants{}, err
	}
	if binding.PluginID != captured.PluginID || binding.Version != captured.Version || binding.PackageDigest != captured.PackageDigest {
		return contract.Grants{}, policy.ErrConflict
	}
	request, err := learningRequest(attempt, records)
	if err != nil {
		return contract.Grants{}, err
	}
	description, err := m.providers.DescribeSandboxProvider(ctx, binding, &pluginsdk.DescribeSandboxProviderRequest{Context: request.Context, Runtime: request.Runtime})
	if err != nil {
		return contract.Grants{}, err
	}
	binary, err := m.providers.ReadSandboxRuntime(ctx, binding, description.Description.RuntimeArtifact)
	if err != nil {
		return contract.Grants{}, err
	}
	if binary.Digest != captured.RuntimeDigest || description.Description.RuntimeVersion != captured.RuntimeVersion {
		return contract.Grants{}, policy.ErrConflict
	}
	response, err := m.providers.ProposeSandboxRevision(ctx, binding, request)
	if err != nil {
		return contract.Grants{}, err
	}
	if len(response.SourceRecordIds) != len(records) {
		return contract.Grants{}, policy.ErrConflict
	}
	return learningGrants(response.AddedGrants)
}

func learningRequest(attempt *policy.Attempt, records []policy.DenialRecord) (*pluginsdk.ProposeSandboxRevisionRequest, error) {
	snapshot := attempt.Snapshot
	provider := snapshot.Provider
	request := &pluginsdk.ProposeSandboxRevisionRequest{
		Context: &pluginsdk.SandboxRequestContext{ProtocolMajor: 1, InstallationId: provider.InstallationID, ProviderKey: provider.Key,
			OperationId: uuid.NewString(), AttemptId: attempt.ID, ExecutionId: snapshot.ExecutionID, Generation: snapshot.Launch.Generation},
		Policy:     wirePolicy(snapshot.Resolved, snapshot.Grants),
		Runtime:    &pluginsdk.SandboxRuntimeFacts{Os: runtime.GOOS, Architecture: runtime.GOARCH, Transport: "local", RuntimeVersion: provider.RuntimeVersion, RuntimeDigest: provider.RuntimeDigest},
		References: childDirectoryReferences(snapshot.AgentState.Directories()),
	}
	for _, record := range records {
		request.SelectedRecords = append(request.SelectedRecords, &pluginsdk.SandboxDenialRecord{RecordId: record.ID, Operation: record.Operation,
			Target: record.Target, OccurredAtUnixMs: record.OccurredAt.UnixMilli(), PolicyDigest: snapshot.Resolved.Revision.Digest, AttemptId: attempt.ID})
	}
	if snapshot.Grants.KandevMCP {
		endpoint, err := capturedMCPEndpoint(snapshot.Launch.MCPURL)
		if err != nil {
			return nil, err
		}
		request.Endpoints = []*pluginsdk.SandboxEndpoint{endpoint}
	}
	return request, nil
}

func capturedMCPEndpoint(raw string) (*pluginsdk.SandboxEndpoint, error) {
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" {
		return nil, policy.ErrConflict
	}
	port, err := strconv.ParseUint(endpoint.Port(), 10, 16)
	if err != nil || port == 0 {
		return nil, policy.ErrConflict
	}
	// The private relay path is a capability. Compilation receives only endpoint intent.
	return &pluginsdk.SandboxEndpoint{Key: "kandev.mcp", Host: endpoint.Hostname(), Port: uint32(port), Scheme: endpoint.Scheme, Path: "/mcp"}, nil
}

func learningGrants(value *pluginsdk.SandboxGrants) (contract.Grants, error) {
	if value == nil {
		return contract.Grants{}, policy.ErrConflict
	}
	result := contract.Grants{KandevMCP: value.KandevMcp, EnvironmentKeys: value.EnvironmentKeys}
	for _, grant := range value.Filesystem {
		if grant == nil {
			return contract.Grants{}, policy.ErrConflict
		}
		result.Filesystem = append(result.Filesystem, contract.FilesystemGrant{Path: grant.Path, Writable: grant.Writable})
	}
	for _, grant := range value.Network {
		if grant == nil || grant.Port == 0 || grant.Port > 65535 {
			return contract.Grants{}, policy.ErrConflict
		}
		result.Network = append(result.Network, contract.NetworkGrant{Host: grant.Host, Port: uint16(grant.Port), Protocol: grant.Protocol})
	}
	return result, nil
}
