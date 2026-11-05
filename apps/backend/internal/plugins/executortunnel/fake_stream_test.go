package executortunnel

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"
	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
)

// fakeProvider is the provider end of one tunnel. The test drives it through
// toHost and reads what the host sent from fromHost.
type fakeProvider struct {
	ctx      context.Context
	toHost   chan *pluginsdk.ExecutorTunnelFrame
	fromHost chan *pluginsdk.ExecutorTunnelFrame
	mu       sync.Mutex
	canceled bool
}

type fakeStream struct{ provider *fakeProvider }

func (s fakeStream) Send(frame *pluginsdk.ExecutorTunnelFrame) error {
	select {
	case s.provider.fromHost <- frame:
		return nil
	case <-s.provider.ctx.Done():
		return s.provider.ctx.Err()
	}
}

func (s fakeStream) Recv() (*pluginsdk.ExecutorTunnelFrame, error) {
	select {
	case frame, ok := <-s.provider.toHost:
		if !ok {
			return nil, io.EOF
		}
		return frame, nil
	case <-s.provider.ctx.Done():
		return nil, s.provider.ctx.Err()
	}
}

func newFakeOpener(t *testing.T, providers chan<- *fakeProvider) Opener {
	t.Helper()
	return func(ctx context.Context) (Stream, error) {
		provider := &fakeProvider{ctx: ctx, toHost: make(chan *pluginsdk.ExecutorTunnelFrame, 8), fromHost: make(chan *pluginsdk.ExecutorTunnelFrame, 8)}
		go func() {
			<-ctx.Done()
			provider.mu.Lock()
			provider.canceled = true
			provider.mu.Unlock()
		}()
		providers <- provider
		return fakeStream{provider: provider}, nil
	}
}

func (p *fakeProvider) wasCanceled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.canceled
}

func (p *fakeProvider) header(t *testing.T) *pluginsdk.ExecutorTunnelHeader {
	t.Helper()
	select {
	case frame := <-p.fromHost:
		if frame.GetHeader() == nil {
			t.Fatalf("first frame from host = %T, want header", frame.GetKind())
		}
		return frame.GetHeader()
	case <-time.After(5 * time.Second):
		t.Fatal("no header frame reached the provider")
		return nil
	}
}

func (p *fakeProvider) ack(nonce string) {
	p.toHost <- &pluginsdk.ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Ack{Ack: &pluginsdk.ExecutorTunnelAck{BindingNonce: nonce}}}
}

func (p *fakeProvider) data(b []byte) {
	p.toHost <- &pluginsdk.ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Data{Data: b}}
}

func (p *fakeProvider) closeFrame(code string) {
	p.toHost <- &pluginsdk.ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Close{Close: &pluginsdk.ExecutorTunnelClose{ErrorCode: code}}}
}

// newTestRegistry returns a registry whose open tunnels are closed when the
// test ends, so no pump goroutine outlives it.
func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	registry := NewRegistry()
	t.Cleanup(func() {
		registry.mu.Lock()
		var doomed []*tunnel
		for _, env := range registry.envs {
			doomed = append(doomed, collect(env)...)
		}
		registry.mu.Unlock()
		closeAll(doomed, reasonCancel)
	})
	return registry
}
