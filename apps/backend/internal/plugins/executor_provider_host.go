package plugins

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/protobuf/proto"
)

// ExecutorProviderHostHandler bridges authenticated provider callbacks to the
// lifecycle owner. Implementations must fence writes by execution and
// environment ownership.
type ExecutorProviderHostHandler interface {
	CheckpointExecutorResource(context.Context, *pluginsdk.CheckpointExecutorResourceRequest) error
	ReportExecutorProgress(context.Context, *pluginsdk.ReportExecutorProgressRequest) error
	ReadExecutorRuntimeArtifact(context.Context, *pluginsdk.ReadExecutorRuntimeArtifactRequest, func(*pluginsdk.ExecutorRuntimeArtifactChunk) error) error
}

type activeExecutorProviderOperation struct {
	context   *pluginsdk.ExecutorProviderRequestContext
	callbacks sync.RWMutex
}

type executorProviderHost struct {
	service  *Service
	pluginID string
}

func (s *Service) SetExecutorProviderHostHandler(handler ExecutorProviderHostHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executorProviderHostHandler = handler
}

func (h *pluginHost) ExecutorProvider() pluginsdk.ExecutorProviderHostAPI {
	return &executorProviderHost{service: h.service, pluginID: h.pluginID}
}

func (h *executorProviderHost) CheckpointExecutorResource(ctx context.Context, req *pluginsdk.CheckpointExecutorResourceRequest) (*pluginsdk.CheckpointExecutorResourceResponse, error) {
	release, err := h.authorize(ctx, req.GetContext())
	if err != nil {
		return nil, err
	}
	defer release()
	handler := h.handler()
	if handler == nil {
		return nil, errors.New("plugins: executor resource checkpoint handler is unavailable")
	}
	if err := handler.CheckpointExecutorResource(ctx, req); err != nil {
		return nil, err
	}
	return &pluginsdk.CheckpointExecutorResourceResponse{Accepted: true}, nil
}

func (h *executorProviderHost) ReportExecutorProgress(ctx context.Context, req *pluginsdk.ReportExecutorProgressRequest) (*pluginsdk.ReportExecutorProgressResponse, error) {
	release, err := h.authorize(ctx, req.GetContext())
	if err != nil {
		return nil, err
	}
	defer release()
	handler := h.handler()
	if handler == nil {
		return nil, errors.New("plugins: executor progress handler is unavailable")
	}
	if err := handler.ReportExecutorProgress(ctx, req); err != nil {
		return nil, err
	}
	return &pluginsdk.ReportExecutorProgressResponse{Accepted: true}, nil
}

func (h *executorProviderHost) ReadExecutorRuntimeArtifact(ctx context.Context, req *pluginsdk.ReadExecutorRuntimeArtifactRequest, send func(*pluginsdk.ExecutorRuntimeArtifactChunk) error) error {
	release, err := h.authorize(ctx, req.GetContext())
	if err != nil {
		return err
	}
	defer release()
	if send == nil {
		return errors.New("plugins: executor artifact receiver is required")
	}
	handler := h.handler()
	if handler == nil {
		return errors.New("plugins: executor artifact handler is unavailable")
	}
	return handler.ReadExecutorRuntimeArtifact(ctx, req, send)
}

func (h *executorProviderHost) authorize(ctx context.Context, request *pluginsdk.ExecutorProviderRequestContext) (func(), error) {
	if h.service == nil || request == nil || request.GetPluginId() != h.pluginID || request.GetOperationId() == "" || request.GetDispatchGeneration() == 0 {
		return nil, errors.New("plugins: executor callback identity is incomplete")
	}
	deadline, err := time.Parse(time.RFC3339Nano, request.GetDeadline())
	if err != nil || !time.Now().Before(deadline) {
		return nil, errors.New("plugins: executor callback lease is expired")
	}
	key := executorProviderOperationKey(h.pluginID, request.GetOperationId(), request.GetDispatchGeneration())
	h.service.executorProviderOpMu.Lock()
	active, ok := h.service.executorProviderOps[key]
	if !ok || !proto.Equal(active.context, request) {
		h.service.executorProviderOpMu.Unlock()
		return nil, errors.New("plugins: executor callback lease is stale")
	}
	active.callbacks.RLock()
	h.service.executorProviderOpMu.Unlock()
	return active.callbacks.RUnlock, nil
}

func (h *executorProviderHost) handler() ExecutorProviderHostHandler {
	if h.service == nil {
		return nil
	}
	h.service.mu.Lock()
	defer h.service.mu.Unlock()
	return h.service.executorProviderHostHandler
}

func executorProviderOperationKey(pluginID, operationID string, generation uint64) string {
	return fmt.Sprintf("%s\x00%s\x00%d", pluginID, operationID, generation)
}

func (s *Service) beginExecutorProviderOperation(request *pluginsdk.ExecutorProviderRequestContext) (*pluginsdk.ExecutorProviderRequestContext, func()) {
	trusted := proto.Clone(request).(*pluginsdk.ExecutorProviderRequestContext)
	if trusted.GetOperationId() == "" {
		return trusted, func() {}
	}
	s.executorProviderOpMu.Lock()
	s.executorProviderDispatchID++
	trusted.DispatchGeneration = s.executorProviderDispatchID
	key := executorProviderOperationKey(trusted.GetPluginId(), trusted.GetOperationId(), trusted.GetDispatchGeneration())
	active := &activeExecutorProviderOperation{context: proto.Clone(trusted).(*pluginsdk.ExecutorProviderRequestContext)}
	s.executorProviderOps[key] = active
	s.executorProviderOpMu.Unlock()
	return trusted, func() {
		s.executorProviderOpMu.Lock()
		if current, exists := s.executorProviderOps[key]; exists && current == active {
			current.callbacks.Lock()
			delete(s.executorProviderOps, key)
			current.callbacks.Unlock()
		}
		s.executorProviderOpMu.Unlock()
	}
}
