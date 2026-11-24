package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	acp "github.com/coder/acp-go-sdk"
	wire "github.com/coder/acp-go-sdk/experimental/v2"
	"github.com/google/uuid"
)

const previewHistoryDirectory = ".kandev/mock-acp-v2"

func inspectMockACPVersion(input io.Reader) (uint16, io.Reader) {
	buffered := bufio.NewReader(input)
	first, _ := buffered.ReadBytes('\n')
	var message struct {
		Method string `json:"method"`
		Params struct {
			ProtocolVersion uint16 `json:"protocolVersion"`
		} `json:"params"`
	}
	version := uint16(1)
	if json.Unmarshal(first, &message) == nil && message.Method == "initialize" {
		version = message.Params.ProtocolVersion
	}
	return version, io.MultiReader(bytes.NewReader(first), buffered)
}

type previewHistoryEntry struct{ ID, Text string }

type previewFixture struct {
	conn    *acp.Connection
	ready   chan struct{}
	mu      sync.Mutex
	session wire.SessionID
	path    string
	history []previewHistoryEntry
	busy    bool
}

func runACPPreviewFixture(stdin io.Reader, stdout io.Writer) {
	peer := &previewFixture{ready: make(chan struct{})}
	peer.conn = acp.NewConnection(peer.handle, stdout, stdin)
	close(peer.ready)
	<-peer.conn.Done()
}

func (p *previewFixture) handle(ctx context.Context, method string, raw json.RawMessage) (any, *acp.RequestError) {
	<-p.ready
	p.mu.Lock()
	defer p.mu.Unlock()
	switch method {
	case "initialize":
		traceACP("initialize_v2", "", nil)
		return wire.InitializeResponse{
			ProtocolVersion: 2, Info: wire.Implementation{Name: "mock-agent", Version: "2.0.0"},
			Capabilities: &wire.AgentCapabilities{Session: &wire.SessionCapabilities{
				MCP: &wire.MCPCapabilities{HTTP: &wire.Capability{}},
			}},
		}, nil
	case "session/new":
		return p.newSession(raw)
	case "session/resume":
		return p.resume(ctx, raw)
	case "session/prompt":
		return p.prompt(ctx, raw)
	case "session/set_config_option":
		return p.setConfigOption(raw)
	case "session/cancel":
		var note wire.CancelSessionNotification
		if json.Unmarshal(raw, &note) != nil || note.SessionID != p.session || !p.busy {
			return nil, nil
		}
		p.busy = false
		return nil, p.state(ctx, wire.StateIdle, wire.StopReasonCancelled)
	case "session/close":
		return wire.CloseSessionResponse{}, nil
	default:
		return nil, acp.NewMethodNotFound(method)
	}
}

func (p *previewFixture) setConfigOption(raw json.RawMessage) (wire.SetSessionConfigOptionResponse, *acp.RequestError) {
	var req wire.SetSessionConfigOptionRequest
	if json.Unmarshal(raw, &req) != nil || req.SessionID != p.session {
		return wire.SetSessionConfigOptionResponse{}, acp.NewInvalidParams(nil)
	}
	options := previewFixtureConfig()
	for _, option := range options {
		if req.ConfigID == option.ConfigID && req.Value == wire.SelectConfigValue(option.Select.CurrentValue) {
			return wire.SetSessionConfigOptionResponse{ConfigOptions: options}, nil
		}
	}
	return wire.SetSessionConfigOptionResponse{}, acp.NewInvalidParams(nil)
}

func (p *previewFixture) newSession(raw json.RawMessage) (wire.NewSessionResponse, *acp.RequestError) {
	var req wire.NewSessionRequest
	if json.Unmarshal(raw, &req) != nil || !filepath.IsAbs(req.Cwd) {
		return wire.NewSessionResponse{}, acp.NewInvalidParams(nil)
	}
	p.session, p.history = wire.SessionID(uuid.NewString()), nil
	p.path = filepath.Join(req.Cwd, previewHistoryDirectory, string(p.session)+".json")
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return wire.NewSessionResponse{}, acp.NewInternalError(nil)
	}
	if err := p.saveHistory(); err != nil {
		return wire.NewSessionResponse{}, err
	}
	traceACP("session_new_v2", string(p.session), nil)
	return wire.NewSessionResponse{SessionID: p.session, ConfigOptions: previewFixtureConfig()}, nil
}

func (p *previewFixture) resume(ctx context.Context, raw json.RawMessage) (wire.ResumeSessionResponse, *acp.RequestError) {
	var req wire.ResumeSessionRequest
	if json.Unmarshal(raw, &req) != nil || !filepath.IsAbs(req.Cwd) || uuid.Validate(string(req.SessionID)) != nil ||
		req.ReplayFrom == nil || req.ReplayFrom.Type != "start" {
		return wire.ResumeSessionResponse{}, acp.NewInvalidParams(nil)
	}
	path := filepath.Join(req.Cwd, previewHistoryDirectory, string(req.SessionID)+".json")
	content, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(content, &p.history) != nil {
		return wire.ResumeSessionResponse{}, acp.NewInvalidParams(nil)
	}
	p.session, p.path = req.SessionID, path
	traceACP("session_resume_v2", string(p.session), nil)
	for _, entry := range p.history {
		if err := p.answer(ctx, entry); err != nil {
			return wire.ResumeSessionResponse{}, err
		}
	}
	if err := p.update(ctx, wire.ReplayComplete{}); err != nil {
		return wire.ResumeSessionResponse{}, err
	}
	return wire.ResumeSessionResponse{ConfigOptions: previewFixtureConfig()}, nil
}

func (p *previewFixture) prompt(ctx context.Context, raw json.RawMessage) (wire.PromptResponse, *acp.RequestError) {
	var req wire.PromptRequest
	if json.Unmarshal(raw, &req) != nil || req.SessionID != p.session || p.busy {
		return wire.PromptResponse{}, acp.NewInvalidParams(nil)
	}
	entry := previewHistoryEntry{ID: uuid.NewString(), Text: "ACP v2 completed response"}
	for _, block := range req.Prompt {
		if block.Text != nil && strings.Contains(*block.Text, "/e2e:acp-v2-hold") {
			p.busy = true
			entry.Text = "ACP v2 response before cancellation"
			break
		}
	}
	p.history = append(p.history, entry)
	if err := p.saveHistory(); err != nil {
		return wire.PromptResponse{}, err
	}
	if err := p.state(ctx, wire.StateRunning, ""); err != nil {
		return wire.PromptResponse{}, err
	}
	if err := p.answer(ctx, entry); err != nil {
		return wire.PromptResponse{}, err
	}
	if !p.busy {
		if err := p.state(ctx, wire.StateIdle, wire.StopReasonEndTurn); err != nil {
			return wire.PromptResponse{}, err
		}
	}
	return wire.PromptResponse{MessageID: wire.MessageID(entry.ID + "-prompt")}, nil
}

func (p *previewFixture) saveHistory() *acp.RequestError {
	content, err := json.Marshal(p.history)
	if err != nil || os.WriteFile(p.path, content, 0o600) != nil {
		return acp.NewInternalError(nil)
	}
	return nil
}

func (p *previewFixture) answer(ctx context.Context, entry previewHistoryEntry) *acp.RequestError {
	return p.update(ctx, wire.MessageChunk{Kind: wire.UpdateAgentMessageChunk, MessageID: wire.MessageID(entry.ID), Content: wire.TextBlock(entry.Text)})
}

func (p *previewFixture) state(ctx context.Context, state wire.SessionState, reason wire.StopReason) *acp.RequestError {
	update := wire.StateUpdate{State: state}
	if reason != "" {
		update.StopReason = acp.OptionalValue(reason)
	}
	return p.update(ctx, update)
}

func (p *previewFixture) update(ctx context.Context, update wire.SessionUpdateValue) *acp.RequestError {
	err := p.conn.SendNotification(ctx, "session/update", wire.UpdateSessionNotification{
		SessionID: p.session, Update: wire.SessionUpdate{Value: update},
	})
	if err != nil {
		return acp.NewInternalError(nil)
	}
	return nil
}

func previewFixtureConfig() []wire.SessionConfigOption {
	return []wire.SessionConfigOption{
		previewFixtureOption(modelConfigID, "Model", modelFast, "Mock Fast"),
		previewFixtureOption("mode", "Mode", mockDefaultMode, "Default"),
	}
}

func previewFixtureOption(category, name, value, label string) wire.SessionConfigOption {
	return wire.SessionConfigOption{
		Type: "select", ConfigID: wire.ConfigID(category), Name: name, Category: &category,
		Select: &wire.SessionConfigSelect{CurrentValue: value, Options: wire.ConfigSelectOptions{
			Choices: []wire.ConfigSelectOption{{Value: value, Name: label}},
		}},
	}
}
