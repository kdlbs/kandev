package plugins

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/analytics"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type retainedUsageSource interface {
	ListUsageCollectionTargets(context.Context, string, pluginsdk.UsageCollectionTargetQueryV1) (pluginsdk.UsageCollectionTargetPageV1, error)
	ReportUsageObservations(context.Context, string, pluginsdk.UsageObservationBatchV1) (pluginsdk.UsageObservationBatchResultV1, error)
}

type canonicalUsagePageSource interface {
	ListCanonicalSessionUsagePage(context.Context, models.SessionUsageFilter) ([]models.SessionUsageMeasurement, models.UsageProjectionState, error)
}

type retainedUsageReader struct{ host *pluginHost }

func (h *pluginHost) UsageHistory() pluginsdk.UsageHistoryReader { return retainedUsageReader{host: h} }

func (h *pluginHost) usageAccessContext(ctx context.Context, workspace string) (context.Context, error) {
	if h.taskData == nil {
		return ctx, status.Error(codes.Unavailable, "workspace authorization is unavailable")
	}
	workspaces, err := h.taskData.ListWorkspaces(ctx)
	if err != nil {
		return ctx, status.Error(codes.Unavailable, "workspace authorization is unavailable")
	}
	ids := make([]string, 0, len(workspaces))
	visible := workspace == ""
	for _, row := range workspaces {
		if row != nil {
			ids = append(ids, row.ID)
			if row.ID == workspace {
				visible = true
			}
		}
	}
	if !visible {
		return ctx, status.Error(codes.NotFound, "usage target not found")
	}
	return usagehistory.WithWorkspaceAccess(ctx, ids), nil
}

func (r retainedUsageReader) source() (retainedUsageSource, error) {
	if r.host.usageDep == nil {
		return nil, status.Error(codes.Unimplemented, "retained usage collection is unsupported")
	}
	source, ok := r.host.usageDep().(retainedUsageSource)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "retained usage collection is unsupported")
	}
	return source, nil
}

func (r retainedUsageReader) ListTargets(ctx context.Context, query pluginsdk.UsageCollectionTargetQueryV1) (pluginsdk.UsageCollectionTargetPageV1, error) {
	if !r.host.capabilities.CanRead(resourceSessionUsage) {
		return pluginsdk.UsageCollectionTargetPageV1{}, permissionDenied(apiReadCapability(resourceSessionUsage))
	}
	ctx, err := r.host.usageAccessContext(ctx, query.WorkspaceID)
	if err != nil {
		return pluginsdk.UsageCollectionTargetPageV1{}, err
	}
	source, err := r.source()
	if err != nil {
		return pluginsdk.UsageCollectionTargetPageV1{}, err
	}
	page, err := source.ListUsageCollectionTargets(ctx, r.host.pluginID, query)
	return page, usagePublicError(err)
}

func (r retainedUsageReader) Report(ctx context.Context, batch pluginsdk.UsageObservationBatchV1) (pluginsdk.UsageObservationBatchResultV1, error) {
	if !r.host.capabilities.CanWrite(resourceSessionUsage) {
		return pluginsdk.UsageObservationBatchResultV1{}, permissionDenied(apiWriteCapability(resourceSessionUsage))
	}
	if len(batch.Observations)+len(batch.Tombstones) > 200 {
		return pluginsdk.UsageObservationBatchResultV1{}, status.Error(codes.InvalidArgument, "maximum 200 usage mutations")
	}
	ctx, err := r.host.usageAccessContext(ctx, "")
	if err != nil {
		return pluginsdk.UsageObservationBatchResultV1{}, err
	}
	source, err := r.source()
	if err != nil {
		return pluginsdk.UsageObservationBatchResultV1{}, err
	}
	result, err := source.ReportUsageObservations(ctx, r.host.pluginID, batch)
	return result, usagePublicError(err)
}

func usagePublicError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, analytics.ErrUsageProjectionChanged) {
		return status.Error(codes.FailedPrecondition, "usage publication changed; restart the saved query")
	}
	if errors.Is(err, analytics.ErrUsageAmountOverflow) {
		return status.Error(codes.OutOfRange, "usage totals exceed the supported integer range")
	}
	if errors.Is(err, repository.ErrUsageNotVisible) {
		return status.Error(codes.NotFound, "usage target not found")
	}
	if errors.Is(err, repository.ErrInvalidUsageBatch) {
		return status.Error(codes.InvalidArgument, "invalid usage batch")
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "usage request canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "usage request timed out")
	}
	return status.Error(codes.Unavailable, "retained usage is unavailable")
}

var _ pluginsdk.UsageHistoryHost = (*pluginHost)(nil)
