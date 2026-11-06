package pluginsdk

import (
	"context"

	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcUsageHistoryReader struct {
	client pluginv1.HostClient
}

func (h *grpcHostClient) UsageHistory() UsageHistoryReader {
	return grpcUsageHistoryReader{client: h.client}
}

func (r grpcUsageHistoryReader) ListTargets(ctx context.Context, query UsageCollectionTargetQueryV1) (UsageCollectionTargetPageV1, error) {
	if err := validateUsageCurrentTargetQuery(query); err != nil {
		return UsageCollectionTargetPageV1{}, err
	}
	response, err := r.client.ListUsageCollectionTargetsV1(ctx, query.toProto())
	if err != nil {
		return UsageCollectionTargetPageV1{}, err
	}
	page := usageCollectionTargetPageV1FromProto(response)
	return page, validateUsageCurrentTargetPage(query, page)
}

func (r grpcUsageHistoryReader) Report(ctx context.Context, batch UsageObservationBatchV1) (UsageObservationBatchResultV1, error) {
	response, err := r.client.ReportUsageObservationsV1(ctx, batch.toProto())
	if err != nil {
		return UsageObservationBatchResultV1{}, err
	}
	return usageObservationBatchResultV1FromProto(response), nil
}

func (s *grpcHostServer) ListUsageCollectionTargetsV1(ctx context.Context, request *pluginv1.ListUsageCollectionTargetsV1Request) (*pluginv1.ListUsageCollectionTargetsV1Response, error) {
	reader, ok := HostUsageHistory(s.impl)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "retained usage collection is not supported")
	}
	page, err := reader.ListTargets(ctx, usageCollectionTargetQueryV1FromProto(request))
	if err != nil {
		return nil, err
	}
	return page.toProto(), nil
}

func (s *grpcHostServer) ReportUsageObservationsV1(ctx context.Context, request *pluginv1.ReportUsageObservationsV1Request) (*pluginv1.ReportUsageObservationsV1Response, error) {
	reader, ok := HostUsageHistory(s.impl)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "retained usage collection is not supported")
	}
	result, err := reader.Report(ctx, usageObservationBatchV1FromProto(request))
	if err != nil {
		return nil, err
	}
	return result.toProto(), nil
}

var _ UsageHistoryReader = grpcUsageHistoryReader{}
var _ UsageHistoryHost = (*grpcHostClient)(nil)
var _ pluginv1.HostServer = (*grpcHostServer)(nil)
