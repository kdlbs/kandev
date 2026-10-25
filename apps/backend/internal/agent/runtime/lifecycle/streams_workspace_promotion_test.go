package lifecycle

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

type workspacePromotionEventBus struct {
	*MockEventBusWithTracking
	gitEvents chan *GitEventPayload
}

func (b *workspacePromotionEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if err := b.MockEventBusWithTracking.Publish(ctx, subject, event); err != nil {
		return err
	}
	if payload, ok := event.Data.(*GitEventPayload); ok && payload != nil {
		b.gitEvents <- payload
	}
	return nil
}

// AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2, .20, .43: the original agentctl
// workspace WebSocket reaches the manager publisher after repeated ACP starts.
func TestWorkspaceStreamPromotionReachesManagerPublisher(t *testing.T) {
	serverConnection := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConnection <- conn
		defer func() { _ = conn.Close() }()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	host, portString, err := net.SplitHostPort(parsed.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portString)
	require.NoError(t, err)
	streamClient := agentctl.NewClient(host, port, logger.Default(),
		agentctl.WithExecutionID("exec-promotion"), agentctl.WithSessionID("session-promotion"))

	eventBus := &workspacePromotionEventBus{
		MockEventBusWithTracking: &MockEventBusWithTracking{},
		gitEvents:                make(chan *GitEventPayload, 1),
	}
	manager := NewManager(newTestRegistry(), eventBus, nil, &MockCredentialsManager{},
		&MockProfileResolver{}, nil, ExecutorFallbackWarn, "", newTestLogger())
	cleanupManagerStopCh(t, manager)

	execution := &AgentExecution{
		ID:                "exec-promotion",
		TaskID:            "task-promotion",
		SessionID:         "session-promotion",
		TaskEnvironmentID: "environment-promotion",
		WorkspacePath:     "/workspace/task-promotion",
		agentctl:          streamClient,
	}
	require.NoError(t, manager.executionStore.Add(execution))
	ready := make(chan struct{})
	manager.streamManager.ConnectWorkspaceStream(execution, ready)
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("workspace stream did not become ready")
	}
	var conn *websocket.Conn
	select {
	case conn = <-serverConnection:
	case <-time.After(5 * time.Second):
		t.Fatal("agentctl WebSocket server did not accept the workspace stream")
	}
	require.Same(t, streamClient, execution.currentAgentCtlClient())
	require.NotNil(t, execution.GetWorkspaceStream())

	execution.beginStartupAttemptWithID("promotion-start")
	execution.beginStartupAttemptWithID("promotion-restart")
	reusedReady := make(chan struct{})
	manager.streamManager.ConnectWorkspaceStream(execution, reusedReady)
	select {
	case <-reusedReady:
	case <-time.After(5 * time.Second):
		t.Fatal("promotion did not settle on the already attached workspace stream")
	}
	require.Same(t, streamClient, execution.currentAgentCtlClient())

	status := &agentctl.GitStatusUpdate{
		Timestamp:        time.Now().UTC(),
		StatusState:      "ready",
		FilesComplete:    true,
		DetailState:      "ready",
		TrackerID:        "agentctl/promotion",
		TrackerEpoch:     3,
		SnapshotRevision: 2,
		RepositoryName:   "backend",
		Branch:           "feature/promotion",
		Modified:         []string{"src/promoted.go"},
		Files: map[string]agentctl.FileInfo{
			"src/promoted.go": {
				Path:      "src/promoted.go",
				Status:    "modified",
				Additions: 2,
				Deletions: 1,
				DiffState: "ready",
				Diff:      "@@ -1 +1,2 @@\n-before\n+after\n+promoted",
			},
		},
	}
	require.NoError(t, conn.WriteJSON(types.WorkspaceStreamMessage{
		Type:      types.WorkspaceMessageTypeGitStatus,
		GitStatus: status,
	}))

	var payload *GitEventPayload
	select {
	case payload = <-eventBus.gitEvents:
	case <-time.After(5 * time.Second):
		t.Fatal("promoted workspace status did not reach the manager Git publisher")
	}
	require.Equal(t, GitEventTypeStatusUpdate, payload.Type)
	require.Equal(t, "task-promotion", payload.TaskID)
	require.Equal(t, "session-promotion", payload.SessionID)
	require.Equal(t, "environment-promotion", payload.TaskEnvironmentID)
	require.Equal(t, "exec-promotion", payload.AgentID)
	require.NotNil(t, payload.Status)
	require.Equal(t, "backend", payload.Status.RepositoryName)
	require.Equal(t, "ready", payload.Status.StatusState)
	require.True(t, payload.Status.FilesComplete)
	require.Equal(t, "ready", payload.Status.DetailState)
	require.Equal(t, []string{"src/promoted.go"}, payload.Status.Modified)
	files, ok := payload.Status.Files.(map[string]agentctl.FileInfo)
	require.True(t, ok, "published file detail type is %T", payload.Status.Files)
	require.Equal(t, "@@ -1 +1,2 @@\n-before\n+after\n+promoted", files["src/promoted.go"].Diff)
}
