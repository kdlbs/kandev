package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type fakePassthroughLifecycle struct {
	accessErr  error
	execution  *lifecycle.AgentExecution
	writeErr   error
	resizeErr  error
	writeCalls int
	resizeCall int
	data       string
	cols       uint16
	rows       uint16
}

func (f *fakePassthroughLifecycle) CheckSessionAccess(context.Context, string) error {
	return f.accessErr
}

func (f *fakePassthroughLifecycle) GetExecutionBySessionID(string) (*lifecycle.AgentExecution, bool) {
	return f.execution, f.execution != nil
}

func (f *fakePassthroughLifecycle) WritePassthroughStdin(_ context.Context, _ string, data string) error {
	f.writeCalls++
	f.data = data
	return f.writeErr
}

func (f *fakePassthroughLifecycle) ResizePassthroughPTY(_ context.Context, _ string, cols, rows uint16) error {
	f.resizeCall++
	f.cols, f.rows = cols, rows
	return f.resizeErr
}

func TestPassthroughHandlersRegisterActions(t *testing.T) {
	h := NewPassthroughHandlers(&fakePassthroughLifecycle{}, newTestLogger())
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	if !dispatcher.HasHandler(ws.ActionAgentStdin) || !dispatcher.HasHandler(ws.ActionAgentResize) {
		t.Fatal("passthrough handlers were not registered")
	}
}

func TestPassthroughStdinSuccessAndFailure(t *testing.T) {
	for _, tt := range []struct {
		name    string
		fake    *fakePassthroughLifecycle
		wantErr string
	}{
		{name: "success", fake: &fakePassthroughLifecycle{execution: &lifecycle.AgentExecution{PassthroughProcessID: "pty"}}},
		{name: "no execution", fake: &fakePassthroughLifecycle{}, wantErr: "no agent running"},
		{name: "not passthrough", fake: &fakePassthroughLifecycle{execution: &lifecycle.AgentExecution{}}, wantErr: "not in passthrough mode"},
		{name: "write failure", fake: &fakePassthroughLifecycle{execution: &lifecycle.AgentExecution{PassthroughProcessID: "pty"}, writeErr: errors.New("closed")}, wantErr: "failed to send agent input"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPassthroughHandlers(tt.fake, newTestLogger())
			msg, _ := ws.NewRequest("id", ws.ActionAgentStdin, AgentStdinRequest{SessionID: "s", Data: "hello\n"})
			response, err := h.wsAgentStdin(context.Background(), msg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || response == nil || tt.fake.writeCalls != 1 || tt.fake.data != "hello\n" {
				t.Fatalf("result = (%#v, %v), fake=%+v", response, err, tt.fake)
			}
			var payload map[string]any
			_ = json.Unmarshal(response.Payload, &payload)
			if payload["success"] != true {
				t.Fatalf("payload = %v", payload)
			}
		})
	}
}

func TestPassthroughResizeValidationSuccessAndFailure(t *testing.T) {
	for _, size := range []AgentResizeRequest{{SessionID: "s", Cols: 0, Rows: 24}, {SessionID: "s", Cols: 80, Rows: 0}} {
		fake := &fakePassthroughLifecycle{execution: &lifecycle.AgentExecution{PassthroughProcessID: "pty"}}
		h := NewPassthroughHandlers(fake, newTestLogger())
		msg, _ := ws.NewRequest("id", ws.ActionAgentResize, size)
		if _, err := h.wsAgentResize(context.Background(), msg); err == nil || fake.resizeCall != 0 {
			t.Fatalf("validation result = %v, calls=%d", err, fake.resizeCall)
		}
	}

	for _, tt := range []struct {
		name    string
		fake    *fakePassthroughLifecycle
		wantErr string
	}{
		{name: "success", fake: &fakePassthroughLifecycle{execution: &lifecycle.AgentExecution{PassthroughProcessID: "pty"}}},
		{name: "no execution", fake: &fakePassthroughLifecycle{}, wantErr: "no agent running"},
		{name: "not passthrough", fake: &fakePassthroughLifecycle{execution: &lifecycle.AgentExecution{}}, wantErr: "not in passthrough mode"},
		{name: "resize failure", fake: &fakePassthroughLifecycle{execution: &lifecycle.AgentExecution{PassthroughProcessID: "pty"}, resizeErr: errors.New("closed")}, wantErr: "failed to resize terminal"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPassthroughHandlers(tt.fake, newTestLogger())
			msg, _ := ws.NewRequest("id", ws.ActionAgentResize, AgentResizeRequest{SessionID: "s", Cols: 80, Rows: 24})
			response, err := h.wsAgentResize(context.Background(), msg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || response == nil || tt.fake.resizeCall != 1 || tt.fake.cols != 80 || tt.fake.rows != 24 {
				t.Fatalf("result = (%#v, %v), fake=%+v", response, err, tt.fake)
			}
		})
	}
}
