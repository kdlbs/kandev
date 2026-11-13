package utility

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"go.uber.org/zap"
)

func TestProbeACPSessionPublishesInitializationBeforeModelFailure(t *testing.T) {
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})
	fake := &probeCaptureAgent{}
	acp.NewAgentSideConnection(fake, a2cW, c2aR)
	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	initialized := false
	_, err := e.probeACPSessionWithContext(
		ctx, c2aW, a2cR, t.TempDir(), "provider-without-models", "model", "", nil, nil,
		authSessionArgs{onInitialized: func() { initialized = true }},
	)
	if err == nil {
		t.Fatal("expected the post-initialize model operation to fail")
	}
	if !initialized {
		t.Fatal("protocol initialization was not exposed before the model failure")
	}
}
