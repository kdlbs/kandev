package main

import (
	"context"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestSetSessionConfigOptionReturnsAuthoritativeState(t *testing.T) {
	sessionID := acp.SessionId("session-config-test")
	agent := &mockAgent{
		sessionConfig: map[acp.SessionId][]acp.SessionConfigOption{
			sessionID: mockSessionConfigOptions(),
		},
	}

	response, err := agent.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: sessionID,
			ConfigId:  "effort",
			Value:     "low",
		},
	})
	if err != nil {
		t.Fatalf("SetSessionConfigOption() error = %v", err)
	}

	values := make(map[string]string, len(response.ConfigOptions))
	for _, option := range response.ConfigOptions {
		if option.Select != nil {
			values[string(option.Select.Id)] = string(option.Select.CurrentValue)
		}
	}
	if values["model"] != "mock-fast" || values["effort"] != "low" {
		t.Fatalf("config values = %#v, want model=mock-fast and effort=low", values)
	}
}

func TestSessionConfigIsIsolatedAcrossNewSessions(t *testing.T) {
	agent := &mockAgent{
		sessions:        make(map[acp.SessionId]bool),
		sessionConfig:   make(map[acp.SessionId][]acp.SessionConfigOption),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
	first, err := agent.NewSession(context.Background(), acp.NewSessionRequest{})
	if err != nil {
		t.Fatalf("first NewSession: %v", err)
	}
	second, err := agent.NewSession(context.Background(), acp.NewSessionRequest{})
	if err != nil {
		t.Fatalf("second NewSession: %v", err)
	}
	if first.SessionId == second.SessionId {
		t.Fatalf("session IDs must be unique: %q", first.SessionId)
	}

	_, err = agent.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: first.SessionId,
			ConfigId:  "effort",
			Value:     "low",
		},
	})
	if err != nil {
		t.Fatalf("set first effort: %v", err)
	}
	response, err := agent.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: second.SessionId,
			ConfigId:  "model",
			Value:     "mock-fast",
		},
	})
	if err != nil {
		t.Fatalf("read second state: %v", err)
	}
	for _, option := range response.ConfigOptions {
		if option.Select != nil && option.Select.Id == "effort" && option.Select.CurrentValue != "medium" {
			t.Fatalf("second effort = %q, want medium", option.Select.CurrentValue)
		}
	}
}

func TestSetSessionConfigOptionRejectsClosedSession(t *testing.T) {
	agent := &mockAgent{
		sessions:        make(map[acp.SessionId]bool),
		sessionConfig:   make(map[acp.SessionId][]acp.SessionConfigOption),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
	session, err := agent.NewSession(context.Background(), acp.NewSessionRequest{})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err = agent.CloseSession(context.Background(), acp.CloseSessionRequest{SessionId: session.SessionId}); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	_, err = agent.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: session.SessionId,
			ConfigId:  "effort",
			Value:     "low",
		},
	})
	if err == nil {
		t.Fatal("SetSessionConfigOption() error = nil, want unknown session error")
	}
	if _, ok := agent.sessionConfig[session.SessionId]; ok {
		t.Fatal("SetSessionConfigOption() recreated closed session config")
	}
}
