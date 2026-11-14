package pluginsdk

import (
	"context"

	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
)

type grpcUsageReader struct {
	client pluginv1.HostClient
}

func (r grpcUsageReader) ListCanonical(ctx context.Context, filter SessionUsageFilter, page Page) ([]SessionUsageMeasurement, *PageInfo, error) {
	filter.Canonical = true
	resp, err := r.client.ListSessionUsage(ctx, &pluginv1.ListSessionUsageRequest{
		Filter: filter.toProto(),
		Page:   page.toProto(),
	})
	if err != nil {
		return nil, nil, err
	}
	return sessionUsageMeasurementsFromProto(resp.GetMeasurements()), pageInfoFromProto(resp.GetPageInfo()), nil
}

func (s *grpcHostServer) ListSessionUsage(ctx context.Context, req *pluginv1.ListSessionUsageRequest) (*pluginv1.ListSessionUsageResponse, error) {
	filter := sessionUsageFilterFromProto(req.GetFilter())
	page := pageFromProto(req.GetPage())
	var items []SessionUsageMeasurement
	var pageInfo *PageInfo
	var err error
	items, pageInfo, err = s.impl.Usage().ListCanonical(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	protoItems := make([]*pluginv1.SessionUsageMeasurement, len(items))
	for i := range items {
		protoItems[i] = items[i].toProto()
	}
	return &pluginv1.ListSessionUsageResponse{Measurements: protoItems, PageInfo: pageInfo.toProto()}, nil
}
