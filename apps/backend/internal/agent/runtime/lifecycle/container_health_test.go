package lifecycle

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/constants"
)

func TestContainerManagerWaitForHealthUsesSetupTimeout(t *testing.T) {
	previousTimeout := constants.SetupScriptTimeout
	constants.SetupScriptTimeout = 20 * time.Millisecond
	t.Cleanup(func() { constants.SetupScriptTimeout = previousTimeout })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	address := strings.TrimPrefix(server.URL, "http://")
	host, portString, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split server address: %v", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	client := agentctl.NewControlClient(host, port, newTestLogger())
	manager := &ContainerManager{logger: newTestLogger()}

	startedAt := time.Now()
	waitCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err = manager.waitForHealth(waitCtx, client)
	if err == nil {
		t.Fatal("waitForHealth() error = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "20ms") {
		t.Fatalf("waitForHealth() error = %v, want setup timeout", err)
	}
	if elapsed := time.Since(startedAt); elapsed >= 500*time.Millisecond {
		t.Fatalf("waitForHealth() took %s, want it to use the configured timeout", elapsed)
	}
}
