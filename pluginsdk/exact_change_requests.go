package pluginsdk

import (
	"context"

	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
)

// ExactTaskChangeRequestReport records one plugin-reported task change
// request: what the plugin observes on its own code host. The host owns the
// row; the provider id must be one of the calling plugin's manifest-declared
// repository_providers.
type ExactTaskChangeRequestReport struct {
	RequestID        string
	WorkspaceID      string
	TaskID           string
	ProviderID       string
	ProviderHost     string
	RepositoryID     string
	Number           int64
	URL              string
	Title            string
	State            string // open | merged | closed
	HeadBranch       string
	BaseBranch       string
	CreatedAt        string
	MergedAt         string
	ClosedAt         string
	IdempotencyKey   string
	ApprovalRevision uint64
	ManifestDigest   string
}

// ExactTaskChangeRequestRemove deletes one plugin-reported task change
// request owned by the calling installation.
type ExactTaskChangeRequestRemove struct {
	RequestID        string
	WorkspaceID      string
	ProviderID       string
	RepositoryID     string
	Number           int64
	IdempotencyKey   string
	ApprovalRevision uint64
	ManifestDigest   string
}

// TaskChangeRequest is one Host-owned, plugin-reported task change request.
type TaskChangeRequest struct {
	ID              string
	WorkspaceID     string
	TaskID          string
	ProviderID      string
	ProviderHost    string
	RepositoryID    string
	Number          int64
	URL             string
	Title           string
	State           string
	HeadBranch      string
	BaseBranch      string
	CreatedAt       string
	MergedAt        *string
	ClosedAt        *string
	ResourceVersion string
}

// ExactTaskChangeRequestCommandManager exposes the Host-owned task
// change-request write surface for code-host plugins.
type ExactTaskChangeRequestCommandManager interface {
	Report(ctx context.Context, input ExactTaskChangeRequestReport) (*CommandResult, TaskChangeRequest, error)
	Remove(ctx context.Context, input ExactTaskChangeRequestRemove) (*CommandResult, error)
}

// ExactTaskChangeRequestCommandHost is an additive Host extension for exact
// task change-request commands.
type ExactTaskChangeRequestCommandHost interface {
	TaskChangeRequests() ExactTaskChangeRequestCommandManager
}

// HostTaskChangeRequests returns the exact task change-request command
// extension when available.
func HostTaskChangeRequests(host Host) (ExactTaskChangeRequestCommandManager, bool) {
	extended, ok := host.(ExactTaskChangeRequestCommandHost)
	if !ok || extended.TaskChangeRequests() == nil {
		return nil, false
	}
	return extended.TaskChangeRequests(), true
}

type grpcExactTaskChangeRequestCommandManager struct {
	client pluginv1.HostClient
}

func (m grpcExactTaskChangeRequestCommandManager) Report(ctx context.Context, input ExactTaskChangeRequestReport) (*CommandResult, TaskChangeRequest, error) {
	response, err := m.client.ReportTaskChangeRequestExact(ctx, &pluginv1.ReportTaskChangeRequestExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID, TaskId: input.TaskID,
		ProviderId: input.ProviderID, ProviderHost: input.ProviderHost, RepositoryId: input.RepositoryID,
		Number: input.Number, Url: input.URL, Title: input.Title, State: input.State,
		HeadBranch: input.HeadBranch, BaseBranch: input.BaseBranch,
		CreatedAt: input.CreatedAt, MergedAt: input.MergedAt, ClosedAt: input.ClosedAt,
		IdempotencyKey:   input.IdempotencyKey,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, TaskChangeRequest{}, err
	}
	return commandResultFromProto(response.GetResult()), taskChangeRequestFromProto(response.GetChangeRequest()), nil
}

func (m grpcExactTaskChangeRequestCommandManager) Remove(ctx context.Context, input ExactTaskChangeRequestRemove) (*CommandResult, error) {
	response, err := m.client.RemoveTaskChangeRequestExact(ctx, &pluginv1.RemoveTaskChangeRequestExactRequest{
		RequestId: input.RequestID, WorkspaceId: input.WorkspaceID,
		ProviderId: input.ProviderID, RepositoryId: input.RepositoryID, Number: input.Number,
		IdempotencyKey:   input.IdempotencyKey,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, err
	}
	return commandResultFromProto(response.GetResult()), nil
}

func taskChangeRequestToProto(item TaskChangeRequest) *pluginv1.TaskChangeRequest {
	out := &pluginv1.TaskChangeRequest{Id: item.ID, WorkspaceId: item.WorkspaceID, TaskId: item.TaskID,
		ProviderId: item.ProviderID, ProviderHost: item.ProviderHost, RepositoryId: item.RepositoryID,
		Number: item.Number, Url: item.URL, Title: item.Title, State: item.State,
		HeadBranch: item.HeadBranch, BaseBranch: item.BaseBranch, CreatedAt: item.CreatedAt,
		ResourceVersion: item.ResourceVersion}
	if item.MergedAt != nil {
		out.MergedAt = item.MergedAt
	}
	if item.ClosedAt != nil {
		out.ClosedAt = item.ClosedAt
	}
	return out
}

func taskChangeRequestFromProto(item *pluginv1.TaskChangeRequest) TaskChangeRequest {
	if item == nil {
		return TaskChangeRequest{}
	}
	return TaskChangeRequest{ID: item.GetId(), WorkspaceID: item.GetWorkspaceId(), TaskID: item.GetTaskId(),
		ProviderID: item.GetProviderId(), ProviderHost: item.GetProviderHost(), RepositoryID: item.GetRepositoryId(),
		Number: item.GetNumber(), URL: item.GetUrl(), Title: item.GetTitle(), State: item.GetState(),
		HeadBranch: item.GetHeadBranch(), BaseBranch: item.GetBaseBranch(), CreatedAt: item.GetCreatedAt(),
		MergedAt: item.MergedAt, ClosedAt: item.ClosedAt, ResourceVersion: item.GetResourceVersion()}
}
