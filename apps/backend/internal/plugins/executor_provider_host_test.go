package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/protobuf/proto"
)

type executorProviderHostHandlerFake struct{ checkpoints int }

func (h *executorProviderHostHandlerFake) CheckpointExecutorResource(context.Context, *pluginsdk.CheckpointExecutorResourceRequest) error {
	h.checkpoints++
	return nil
}

type blockingExecutorProviderHostHandler struct {
	entered chan struct{}
	finish  chan struct{}
}

func (h *blockingExecutorProviderHostHandler) CheckpointExecutorResource(context.Context, *pluginsdk.CheckpointExecutorResourceRequest) error {
	close(h.entered)
	<-h.finish
	return nil
}

func (*blockingExecutorProviderHostHandler) ReportExecutorProgress(context.Context, *pluginsdk.ReportExecutorProgressRequest) error {
	return nil
}

func (*blockingExecutorProviderHostHandler) ReadExecutorRuntimeArtifact(context.Context, *pluginsdk.ReadExecutorRuntimeArtifactRequest, func(*pluginsdk.ExecutorRuntimeArtifactChunk) error) error {
	return nil
}

func (*executorProviderHostHandlerFake) ReportExecutorProgress(context.Context, *pluginsdk.ReportExecutorProgressRequest) error {
	return nil
}

func (*executorProviderHostHandlerFake) ReadExecutorRuntimeArtifact(context.Context, *pluginsdk.ReadExecutorRuntimeArtifactRequest, func(*pluginsdk.ExecutorRuntimeArtifactChunk) error) error {
	return nil
}

func TestExecutorProviderHostCallbackRejectsStaleDispatchGeneration(t *testing.T) {
	service := NewService(nil, nil, nil, nil)
	handler := &executorProviderHostHandlerFake{}
	service.SetExecutorProviderHostHandler(handler)
	requestContext := &pluginsdk.ExecutorProviderRequestContext{
		PluginId: "fixture", InstallationId: "install-1", ProviderKey: "remote", ContractVersion: 1,
		TaskId: "task-1", SessionId: "session-1", EnvironmentId: "environment-1",
		ExecutionId: "execution-1", OperationId: "execution-1", EnvironmentGeneration: 4,
		Deadline: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano), InputDigest: "digest",
	}
	active, release := service.beginExecutorProviderOperation(requestContext)
	host := &executorProviderHost{service: service, pluginID: "fixture"}
	response, err := host.CheckpointExecutorResource(context.Background(), &pluginsdk.CheckpointExecutorResourceRequest{
		Context: active, Resource: &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-1"}, Phase: "provisioned",
	})
	if err != nil || response == nil || !response.GetAccepted() || handler.checkpoints != 1 {
		t.Fatalf("current callback = (%+v, %v), checkpoints=%d", response, err, handler.checkpoints)
	}
	stale := proto.Clone(active).(*pluginsdk.ExecutorProviderRequestContext)
	stale.DispatchGeneration++
	if _, err := host.CheckpointExecutorResource(context.Background(), &pluginsdk.CheckpointExecutorResourceRequest{
		Context: stale, Resource: &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "stale-resource"}, Phase: "provisioned",
	}); err == nil {
		t.Fatal("stale dispatch generation callback was accepted")
	}
	release()
	if _, err := host.CheckpointExecutorResource(context.Background(), &pluginsdk.CheckpointExecutorResourceRequest{
		Context: active, Resource: &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "late-resource"}, Phase: "provisioned",
	}); err == nil {
		t.Fatal("callback after operation lease release was accepted")
	}
}

func TestPluginExecutorCallbackReleaseDrainsAdmittedHostCallback(t *testing.T) {
	service := NewService(nil, nil, nil, nil)
	handler := &blockingExecutorProviderHostHandler{entered: make(chan struct{}), finish: make(chan struct{})}
	service.SetExecutorProviderHostHandler(handler)
	requestContext := &pluginsdk.ExecutorProviderRequestContext{
		PluginId: "fixture", InstallationId: "install-1", ProviderKey: "remote", ContractVersion: 1,
		TaskId: "task-1", SessionId: "session-1", EnvironmentId: "environment-1",
		ExecutionId: "execution-1", OperationId: "execution-1", EnvironmentGeneration: 4,
		Deadline: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano), InputDigest: "digest",
	}
	active, release := service.beginExecutorProviderOperation(requestContext)
	host := &executorProviderHost{service: service, pluginID: "fixture"}
	callbackDone := make(chan error, 1)
	go func() {
		_, err := host.CheckpointExecutorResource(context.Background(), &pluginsdk.CheckpointExecutorResourceRequest{
			Context: active, Resource: &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-1"}, Phase: "provisioned",
		})
		callbackDone <- err
	}()
	select {
	case <-handler.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("host callback did not enter its handler")
	}
	releaseDone := make(chan struct{})
	go func() {
		release()
		close(releaseDone)
	}()
	select {
	case <-releaseDone:
		t.Fatal("operation release returned while its admitted host callback was still writing")
	case <-time.After(20 * time.Millisecond):
	}
	close(handler.finish)
	if err := <-callbackDone; err != nil {
		t.Fatalf("admitted callback error = %v", err)
	}
	select {
	case <-releaseDone:
	case <-time.After(2 * time.Second):
		t.Fatal("operation release did not finish after its host callback drained")
	}
	if _, err := host.CheckpointExecutorResource(context.Background(), &pluginsdk.CheckpointExecutorResourceRequest{
		Context: active, Resource: &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "late-resource"}, Phase: "provisioned",
	}); err == nil {
		t.Fatal("callback after operation release was accepted")
	}
}
