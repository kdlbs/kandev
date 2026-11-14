package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type delayedMaterializationTransport struct {
	delay   time.Duration
	entered chan struct{}
}

func (t *delayedMaterializationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	close(t.entered)
	timer := time.NewTimer(t.delay)
	defer timer.Stop()

	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case <-timer.C:
		body := `{"destination":"repositories/project"}`
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader(body)),
			ContentLength: int64(len(body)),
			Request:       req,
		}, nil
	}
}

func newDelayedMaterializationClient(transport http.RoundTripper) *Client {
	return newClient("http://agentctl.test", newTestLogger(), func(c *Client) {
		c.httpClient.Transport = transport
		c.longRunningHTTPClient.Transport = transport
	})
}

func materializeRepositoryAsync(c *Client, ctx context.Context) <-chan error {
	result := make(chan error, 1)
	go func() {
		_, err := c.MaterializeRepository(ctx, MaterializeRepositoryRequest{
			RepositoryURL: "https://github.com/kdlbs/project.git",
			Destination:   "repositories/project",
			BaseBranch:    "main",
		})
		result <- err
	}()
	return result
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.6
func TestMaterializeRepository_UsesLongRunningClient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		transport := &delayedMaterializationTransport{
			delay:   61 * time.Second,
			entered: make(chan struct{}),
		}
		c := newDelayedMaterializationClient(transport)
		response, err := c.MaterializeRepository(context.Background(), MaterializeRepositoryRequest{
			RepositoryURL: "https://github.com/kdlbs/project.git",
			Destination:   "repositories/project",
			BaseBranch:    "main",
		})
		if err != nil {
			t.Fatalf("MaterializeRepository: %v", err)
		}
		if response == nil || response.Destination != "repositories/project" {
			t.Fatalf("response = %#v, want materialized destination", response)
		}
	})
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.7
func TestMaterializeRepository_HonorsCallerContext(t *testing.T) {
	t.Run("earlier deadline", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			transport := &delayedMaterializationTransport{
				delay:   time.Minute,
				entered: make(chan struct{}),
			}
			c := newDelayedMaterializationClient(transport)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result := materializeRepositoryAsync(c, ctx)
			<-transport.entered
			err := <-result
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("MaterializeRepository error = %v, want caller deadline", err)
			}
		})
	})

	t.Run("cancellation after request entry", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			transport := &delayedMaterializationTransport{
				delay:   time.Minute,
				entered: make(chan struct{}),
			}
			c := newDelayedMaterializationClient(transport)
			ctx, cancel := context.WithCancel(context.Background())
			result := materializeRepositoryAsync(c, ctx)
			<-transport.entered
			cancel()
			err := <-result
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("MaterializeRepository error = %v, want caller cancellation", err)
			}
		})
	})
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.8
func TestMaterializeRepository_LongRunningTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		transport := &delayedMaterializationTransport{
			delay:   6 * time.Minute,
			entered: make(chan struct{}),
		}
		c := newDelayedMaterializationClient(transport)
		_, err := c.MaterializeRepository(context.Background(), MaterializeRepositoryRequest{
			RepositoryURL: "https://github.com/kdlbs/project.git",
			Destination:   "repositories/project",
			BaseBranch:    "main",
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("MaterializeRepository error = %v, want five-minute timeout", err)
		}
	})
}
