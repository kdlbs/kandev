package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
)

type grpcUsageReader struct {
	client pluginv1.HostClient
}

func (r grpcUsageReader) UpsertBatch(ctx context.Context, workspaceID string, measurements []SessionUsageMeasurement) ([]SessionUsageWriteResult, error) {
	items := make([]*pluginv1.SessionUsageMeasurement, len(measurements))
	for i := range measurements {
		items[i] = measurements[i].toProto()
	}
	resp, err := r.client.UpsertSessionUsage(ctx, &pluginv1.UpsertSessionUsageRequest{
		WorkspaceId:  workspaceID,
		Measurements: items,
	})
	if err != nil {
		return nil, err
	}
	results := make([]SessionUsageWriteResult, len(resp.GetResults()))
	for i, result := range resp.GetResults() {
		results[i] = SessionUsageWriteResult{
			SourceRecordID: result.GetSourceRecordId(), UsageIdentity: result.GetUsageIdentity(),
			Model: result.GetModel(), Provider: result.GetProvider(), Status: result.GetStatus(),
			Error: result.GetError(),
		}
		if result.GetMeasurement() != nil {
			measurement := sessionUsageMeasurementFromProto(result.GetMeasurement())
			results[i].Measurement = &measurement
		}
	}
	return results, nil
}

func (r grpcUsageReader) List(ctx context.Context, filter SessionUsageFilter, page Page) ([]SessionUsageMeasurement, *PageInfo, error) {
	return r.list(ctx, filter, page, false)
}

func (r grpcUsageReader) ListCanonical(ctx context.Context, filter SessionUsageFilter, page Page) ([]SessionUsageMeasurement, *PageInfo, error) {
	return r.list(ctx, filter, page, true)
}

func (r grpcUsageReader) list(ctx context.Context, filter SessionUsageFilter, page Page, canonical bool) ([]SessionUsageMeasurement, *PageInfo, error) {
	filter.Canonical = canonical
	resp, err := r.client.ListSessionUsage(ctx, &pluginv1.ListSessionUsageRequest{
		Filter: filter.toProto(),
		Page:   page.toProto(),
	})
	if err != nil {
		return nil, nil, err
	}
	return sessionUsageMeasurementsFromProto(resp.GetMeasurements()), pageInfoFromProto(resp.GetPageInfo()), nil
}

func (s *grpcHostServer) UpsertSessionUsage(ctx context.Context, req *pluginv1.UpsertSessionUsageRequest) (*pluginv1.UpsertSessionUsageResponse, error) {
	measurements := make([]SessionUsageMeasurement, len(req.GetMeasurements()))
	for i, item := range req.GetMeasurements() {
		measurements[i] = sessionUsageMeasurementFromProto(item)
	}
	results, err := s.impl.Usage().UpsertBatch(ctx, req.GetWorkspaceId(), measurements)
	if err != nil {
		return nil, err
	}
	protoResults := make([]*pluginv1.SessionUsageWriteResult, len(results))
	for i, result := range results {
		protoResults[i] = &pluginv1.SessionUsageWriteResult{
			SourceRecordId: result.SourceRecordID,
			UsageIdentity:  result.UsageIdentity,
			Model:          result.Model,
			Provider:       result.Provider,
			Status:         result.Status,
			Error:          result.Error,
		}
		if result.Measurement != nil {
			protoResults[i].Measurement = result.Measurement.toProto()
		}
	}
	return &pluginv1.UpsertSessionUsageResponse{Results: protoResults}, nil
}

func (s *grpcHostServer) ListSessionUsage(ctx context.Context, req *pluginv1.ListSessionUsageRequest) (*pluginv1.ListSessionUsageResponse, error) {
	filter := sessionUsageFilterFromProto(req.GetFilter())
	page := pageFromProto(req.GetPage())
	var items []SessionUsageMeasurement
	var pageInfo *PageInfo
	var err error
	if filter.Canonical {
		items, pageInfo, err = s.impl.Usage().ListCanonical(ctx, filter, page)
	} else {
		items, pageInfo, err = s.impl.Usage().List(ctx, filter, page)
	}
	if err != nil {
		return nil, err
	}
	protoItems := make([]*pluginv1.SessionUsageMeasurement, len(items))
	for i := range items {
		protoItems[i] = items[i].toProto()
	}
	return &pluginv1.ListSessionUsageResponse{Measurements: protoItems, PageInfo: pageInfo.toProto()}, nil
}
