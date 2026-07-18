package docker

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/docker/docker/api/types/build"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

func TestBuildImageHoldsActivityUntilResponseBodyCloses(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	coordinator := activity.NewCoordinator(activity.Options{})
	client := &Client{
		builder: &fakeImageBuilder{response: build.ImageBuildResponse{
			Body: io.NopCloser(bytes.NewBufferString("build output")),
		}},
		logger: log,
	}
	client.SetActivityCoordinator(coordinator)

	body, err := client.BuildImage(context.Background(), "FROM scratch", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0); err != activity.ErrBusy {
		t.Fatalf("maintenance acquire error = %v, want ErrBusy", err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	lease, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("maintenance remained busy after close: %v", err)
	}
	lease.Release()
}

type fakeImageBuilder struct {
	response build.ImageBuildResponse
	err      error
}

func (f *fakeImageBuilder) ImageBuild(context.Context, io.Reader, build.ImageBuildOptions) (build.ImageBuildResponse, error) {
	return f.response, f.err
}
