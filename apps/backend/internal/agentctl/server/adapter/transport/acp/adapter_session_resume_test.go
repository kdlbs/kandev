package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type sessionResumeAgent struct {
	sessionRequestCaptureAgent
	resumeRequest acpsdk.ResumeSessionRequest
	resumeError   error
	resumeCalls   int
}

func (a *sessionResumeAgent) ResumeSession(_ context.Context, req acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	a.resumeCalls++
	a.resumeRequest = req
	var response acpsdk.ResumeSessionResponse
	if err := json.Unmarshal([]byte(`{
		"modes":{"currentModeId":"code","availableModes":[{"id":"code","name":"Code"}]},
		"configOptions":[{"type":"select","id":"model","name":"Model","category":"model",
			"currentValue":"saved-model","options":[{"value":"saved-model","name":"Saved model"}]}],
		"_meta":{"restored":true}
	}`), &response); err != nil {
		return response, err
	}
	return response, a.resumeError
}

func newSessionResumeAdapter(t *testing.T, load, resume bool) (*Adapter, *sessionResumeAgent) {
	t.Helper()
	toAgent, fromClient := io.Pipe()
	t.Cleanup(func() { _ = toAgent.Close(); _ = fromClient.Close() })
	toClient, fromAgent := io.Pipe()
	t.Cleanup(func() { _ = toClient.Close(); _ = fromAgent.Close() })
	fake := &sessionResumeAgent{}
	conn := acpsdk.NewClientSideConnection(acpclient.NewClient(), fromClient, toClient)
	_ = acpsdk.NewAgentSideConnection(fake, fromAgent, toAgent)
	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })
	a.acpConn = conn
	a.cfg.WorkDir = t.TempDir()
	a.capabilities.LoadSession = load
	a.capabilities.McpCapabilities = acpsdk.McpCapabilities{Http: true, Sse: true}
	if resume {
		a.capabilities.SessionCapabilities.Resume = &acpsdk.SessionResumeCapabilities{}
	}
	return a, fake
}

func TestLoadSessionPrefersAdvertisedResume(t *testing.T) {
	for _, load := range []bool{true, false} {
		t.Run(fmt.Sprintf("load_supported_%t", load), func(t *testing.T) {
			a, fake := newSessionResumeAdapter(t, load, true)
			servers := []types.McpServer{
				{Name: "kandev", Type: "http", URL: "http://localhost:10005/mcp"},
				{Name: "kandev", Type: "sse", URL: "http://localhost:10005/sse"},
			}
			if err := a.LoadSession(t.Context(), "saved-session", servers); err != nil {
				t.Fatalf("LoadSession: %v", err)
			}
			if fake.resumeCalls != 1 || fake.loadRequest.SessionId != "" || fake.sessionCounter != 0 {
				t.Fatalf("want one resume without replay or replacement, got resume=%d load=%q new=%d",
					fake.resumeCalls, fake.loadRequest.SessionId, fake.sessionCounter)
			}
			if fake.resumeRequest.SessionId != "saved-session" || fake.resumeRequest.Cwd != a.cfg.WorkDir {
				t.Fatalf("resume lost session identity or cwd: %+v", fake.resumeRequest)
			}
			assertCapturedKandevTransport(t, fake.resumeRequest.McpServers, "http")
			if a.GetSessionID() != "saved-session" || a.isLoadingSession {
				t.Fatal("resume did not settle the saved session")
			}
			events := drainEvents(a)
			models := findSessionModelsEvent(t, events)
			if models.CurrentModelID != "saved-model" || len(models.ConfigOptions) != 1 {
				t.Fatalf("resume lost model/config state: %+v", models)
			}
			if state := a.GetSessionModelState(); state == nil || state.CurrentModelID != "saved-model" {
				t.Fatalf("cached model state = %+v", state)
			}
			var resumed, mode bool
			for _, event := range events {
				resumed = resumed || event.SessionStatus == streams.SessionStatusResumed
				mode = mode || event.CurrentModeID == "code"
			}
			if !resumed || !mode {
				t.Fatalf("resume events missing: resumed=%t mode=%t", resumed, mode)
			}
		})
	}
}

func TestLoadSessionFallsBackToReplayOnlyWhenResumeUnsupported(t *testing.T) {
	for _, advertised := range []bool{true, false} {
		t.Run(fmt.Sprintf("resume_advertised_%t", advertised), func(t *testing.T) {
			a, fake := newSessionResumeAdapter(t, true, advertised)
			fake.resumeError = acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionResume)
			if err := a.LoadSession(t.Context(), "saved-session", nil); err != nil {
				t.Fatalf("LoadSession: %v", err)
			}
			if fake.loadRequest.SessionId != "saved-session" || fake.sessionCounter != 0 {
				t.Fatalf("want replay of saved session, got load=%q new=%d", fake.loadRequest.SessionId, fake.sessionCounter)
			}
			if advertised != (fake.resumeCalls == 1) {
				t.Fatalf("resume calls=%d advertised=%t", fake.resumeCalls, advertised)
			}
		})
	}
}

func TestLoadSessionResumeFailurePreservesIdentity(t *testing.T) {
	for _, cause := range []error{
		acpsdk.NewInternalError(map[string]any{"error": "context deadline exceeded"}),
		acpsdk.NewAuthRequired(nil),
		acpsdk.NewRequestCancelled(nil),
	} {
		t.Run(cause.Error(), func(t *testing.T) {
			a, fake := newSessionResumeAdapter(t, true, true)
			fake.resumeError = cause
			a.sessionID = "prior-session"
			if err := a.LoadSession(t.Context(), "saved-session", nil); err == nil {
				t.Fatal("expected resume error")
			}
			if fake.resumeCalls != 1 || fake.loadRequest.SessionId != "" || fake.sessionCounter != 0 {
				t.Fatalf("inconclusive error retried: resume=%d load=%q new=%d",
					fake.resumeCalls, fake.loadRequest.SessionId, fake.sessionCounter)
			}
			if a.GetSessionID() != "prior-session" || a.isLoadingSession {
				t.Fatal("failed resume changed identity or retained replay suppression")
			}
		})
	}
}
