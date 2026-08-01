package backendapp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

type blockingShutdownStopper struct {
	entered chan struct{}
	release chan struct{}
	err     error
}

func (s *blockingShutdownStopper) Stop() error {
	close(s.entered)
	<-s.release
	return s.err
}

func TestRunGracefulShutdownStopsSchedulersBeforeDatabaseCleanup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	stopErr := errors.New("scheduler stop failed")
	stopper := &blockingShutdownStopper{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		err:     stopErr,
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(stopper.release) }) }
	t.Cleanup(release)
	scheduling := &schedulingRuntime{runs: stopper}
	dbClosed := make(chan struct{})
	cleanup := func() {
		close(dbClosed)
	}

	done := make(chan []error, 1)
	go func() {
		done <- runGracefulShutdown(
			server.Config,
			nil,
			scheduling,
			nil,
			nil,
			cleanup,
			logger.Default(),
		)
	}()

	select {
	case <-stopper.entered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("scheduler stop was not called")
	}
	select {
	case <-dbClosed:
		t.Fatal("database cleanup ran before scheduler stop completed")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	var shutdownErrs []error
	select {
	case shutdownErrs = <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("graceful shutdown did not finish after scheduler stop completed")
	}
	select {
	case <-dbClosed:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("database cleanup did not run")
	}

	if !errors.Is(errors.Join(shutdownErrs...), stopErr) {
		t.Fatalf("shutdown errors = %v, want scheduler stop error", shutdownErrs)
	}
}
