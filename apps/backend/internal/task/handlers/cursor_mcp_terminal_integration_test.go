package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	agentprocess "github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/task/service"
	terminalrepo "github.com/kandev/kandev/internal/terminal/repository"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
	_ "github.com/mattn/go-sqlite3"
)

func TestCursorMCPAuthenticateAfterCompletedTerminalReconnectStartsFreshLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	workspace := t.TempDir()
	marker := filepath.Join(t.TempDir(), "login-count")
	command := "printf x >> " + shellQuoteForTerminalIntegrationTest(marker) + "; exit"

	rawDB, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "terminals.db"))
	if err != nil {
		t.Fatal(err)
	}
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := terminalrepo.NewWithDB(db, db, log)
	if err != nil {
		t.Fatal(err)
	}
	runner := agentprocess.NewInteractiveRunner(nil, log, 2*1024*1024)
	t.Cleanup(func() {
		_, _ = runner.StopUserShellsForScope(context.Background(), "env-auth-integration")
	})
	terminals := terminalservice.New(repo, terminalservice.NewInteractiveRunnerBackend(runner), log)

	executors := lifecycle.NewExecutorRegistry(log)
	standalone := lifecycle.NewStandaloneExecutor(nil, "", 0, log)
	standalone.SetInteractiveRunner(runner)
	executors.Register(standalone)
	manager := lifecycle.NewManager(nil, nil, executors, nil, nil, nil,
		lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	execution := &lifecycle.AgentExecution{
		ID: "exec-auth-integration", TaskID: "task-auth-integration", SessionID: "session-auth-integration",
		TaskEnvironmentID: "env-auth-integration", WorkspacePath: workspace,
	}
	if err := manager.ExecutionStoreForTesting().Add(execution); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetExecutionEnv(context.Background(), execution.ID, map[string]string{"PATH": os.Getenv("PATH")}); err != nil {
		t.Fatal(err)
	}

	recoveryManager := &fakeCursorMCPRecoveryManager{authSpec: runtime.CursorMCPAuthenticationSpec{
		TaskID: execution.TaskID, TaskEnvironmentID: execution.TaskEnvironmentID,
		WorkspacePath: workspace, InitialCommand: command,
		Label: "Sign in to Issue Tracker", ServerID: "server-exact",
	}}
	taskService := service.NewService(service.Repos{}, nil, log, service.RepositoryDiscoveryConfig{})
	recoveryRouter := gin.New()
	recoveryHandlers := RegisterProcessRoutes(recoveryRouter, taskService, manager, log)
	recoveryHandlers.cursorMCPRecovery = recoveryManager
	recoveryHandlers.SetTerminalService(terminals)

	terminalHandler := websocket.NewTerminalHandler(manager, nil, nil, log)
	terminalHandler.SetTerminalService(terminals)
	terminalRouter := gin.New()
	terminalRouter.GET("/terminal/*target", terminalHandler.HandleTerminalWS)
	terminalServer := httptest.NewServer(terminalRouter)
	t.Cleanup(terminalServer.Close)
	wsBase := "ws" + strings.TrimPrefix(terminalServer.URL, "http")

	authenticate := func() cursorMCPAuthenticationResponse {
		t.Helper()
		rec := performMCPRecoveryRequest(recoveryRouter, http.MethodPost,
			"/api/v1/task-sessions/"+execution.SessionID+"/mcp/authenticate", `{"server_id":"server-exact"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("authenticate status=%d body=%s", rec.Code, rec.Body.String())
		}
		var response cursorMCPAuthenticationResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	connectTerminal := func(terminalID string) *gorillaws.Conn {
		t.Helper()
		url := wsBase + "/terminal/environment/" + execution.TaskEnvironmentID +
			"?mode=shell&terminalId=" + terminalID
		conn, _, err := gorillaws.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("connect terminal %s: %v", terminalID, err)
		}
		return conn
	}
	startLogin := func(conn *gorillaws.Conn) {
		t.Helper()
		frame := append([]byte{1}, []byte(`{"cols":80,"rows":24}`)...)
		if err := conn.WriteMessage(gorillaws.BinaryMessage, frame); err != nil {
			t.Fatalf("send terminal resize: %v", err)
		}
	}
	waitForMarker := func(want string) {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			data, readErr := os.ReadFile(marker)
			if readErr == nil && string(data) == want {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		data, _ := os.ReadFile(marker)
		t.Fatalf("login executions = %q, want %q", data, want)
	}

	first := authenticate()
	firstConn := connectTerminal(first.TerminalID)
	startLogin(firstConn)
	waitForMarker("x")
	waitForUserShellExit(t, runner, execution.TaskEnvironmentID, first.TerminalID)
	_ = firstConn.Close()

	// This is the browser's automatic reconnect after `exec cursor-agent mcp
	// login ...` exits. It is closed normally with a typed reason and does not
	// start a blank shell for the completed authentication terminal.
	reconnect := connectTerminal(first.TerminalID)
	_, _, readErr := reconnect.ReadMessage()
	_ = reconnect.Close()
	var closeErr *gorillaws.CloseError
	if !errors.As(readErr, &closeErr) || closeErr.Code != gorillaws.CloseNormalClosure || closeErr.Text != "initial_command_completed" {
		t.Fatalf("completed terminal reconnect close = %#v, error %v", closeErr, readErr)
	}

	// The actual Authenticate HTTP handler sees the stopped terminal, creates
	// a new one-shot terminal, and the next terminal connection runs one new login.
	second := authenticate()
	if second.TerminalID == first.TerminalID || second.Reused {
		t.Fatalf("second authenticate = %+v, want a fresh terminal", second)
	}
	secondConn := connectTerminal(second.TerminalID)
	startLogin(secondConn)
	waitForMarker("xx")
	_ = secondConn.Close()
}

func waitForUserShellExit(t *testing.T, runner *agentprocess.InteractiveRunner, scopeID, terminalID string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if !runner.IsUserShellAlive(scopeID, terminalID) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("user shell %s did not exit", terminalID)
}

func shellQuoteForTerminalIntegrationTest(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
