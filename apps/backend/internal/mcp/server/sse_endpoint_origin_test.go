package mcp

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

func TestSSEEndpointUsesConnectionOrigin(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	port := server.Listener.Addr().(*net.TCPAddr).Port
	backend := NewChannelBackendClient(logger.Default())
	t.Cleanup(backend.Close)

	s := NewWithProfile(
		backend,
		"session-1",
		"task-1",
		port,
		logger.Default(),
		"",
		false,
		mcpprofile.Legacy(ModeTask, false, nil),
		WithSSEBaseURL("http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(port))),
	)
	server.Config.Handler = s.sseServer.SSEHandler()
	server.Start()
	t.Cleanup(server.Close)

	response, err := http.Get(server.URL + "/sse")
	if err != nil {
		t.Fatalf("connect to SSE endpoint: %v", err)
	}
	defer func() { _ = response.Body.Close() }()

	reader := bufio.NewReader(response.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read SSE event type: %v", err)
	}
	dataLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read SSE endpoint event: %v", err)
	}
	endpointText := strings.TrimSpace(strings.TrimPrefix(dataLine, "data: "))
	endpoint, err := url.Parse(endpointText)
	if err != nil {
		t.Fatalf("parse SSE message endpoint %q: %v", endpointText, err)
	}
	connection, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse SSE connection URL %q: %v", server.URL, err)
	}
	if endpoint.Host != connection.Host {
		t.Fatalf("SSE message endpoint host = %q, connection host = %q", endpoint.Host, connection.Host)
	}
}
