package canvas

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/webapp"
)

type blockingDownloadBody struct {
	closed chan struct{}
}

func (b *blockingDownloadBody) Read([]byte) (int, error) {
	<-b.closed
	return 0, io.EOF
}

func (b *blockingDownloadBody) Close() error {
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	return nil
}

func TestDownloadInstallBundleClosesOversizedResponseWithoutDrainingIt(t *testing.T) {
	body := &blockingDownloadBody{closed: make(chan struct{})}
	service := NewDistributionService(nil, nil, nil, func(context.Context, string) error { return nil }, nil)
	service.SetInstallHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, ContentLength: webapp.MaxCompressedBytes + 1, Body: body}, nil
	})})

	result := make(chan error, 1)
	go func() {
		_, err := service.downloadInstallBundle(context.Background(), "https://example.test/too-large.tar.gz")
		result <- err
	}()

	select {
	case err := <-result:
		if !errors.Is(err, webapp.ErrCompressedTooLarge) {
			t.Fatalf("download error = %v, want ErrCompressedTooLarge", err)
		}
	case <-time.After(2 * time.Second):
		_ = body.Close()
		t.Fatal("oversized response was drained instead of closed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
