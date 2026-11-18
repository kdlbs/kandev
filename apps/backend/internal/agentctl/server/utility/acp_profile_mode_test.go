package utility

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"
)

func TestIssue5364ProbePublishesConfigModeValues(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := executor.probeACPSessionWithContext(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "", "", nil, nil,
	)
	if err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v", err)
	}

	wantModes := []ProbeMode{
		{ID: "session", Name: "Don't Ask"},
		{ID: "workspace", Name: "Ask"},
	}
	if !reflect.DeepEqual(response.Modes, wantModes) {
		t.Fatalf("modes = %#v, want config mode choices %#v", response.Modes, wantModes)
	}
	if response.CurrentModeID != "workspace" {
		t.Fatalf("current mode = %q, want config value workspace", response.CurrentModeID)
	}
	if calls := agent.recordedCalls(); len(calls) != 0 {
		t.Fatalf("mode changes during discovery = %v, want none", calls)
	}
}

func TestIssue5364ProbeDoesNotPublishUnlistedConfigModeDefault(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.currentMode = "unlisted"
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := executor.probeACPSessionWithContext(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "", "", nil, nil,
	)
	if err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v", err)
	}
	if response.CurrentModeID != "" {
		t.Fatalf("current mode = %q, want empty for an unlisted config value", response.CurrentModeID)
	}
}

func TestIssue5364ProbeSendsExactConfigModeValue(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.rejectLegacyMode = true
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := executor.probeACPSessionWithContext(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "fixture-model", "session", nil, nil,
	)
	if err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v", err)
	}

	wantCalls := []string{
		"config:edit_approval_policy:session",
		"config:model:fixture-model",
	}
	if got := agent.recordedCalls(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("ACP calls = %v, want mode config before model config %v", got, wantCalls)
	}
}

func TestIssue5364ProbeSkipsLegacyValueWhenConfigModeSelectIsEmpty(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.emptyModeChoices = true
	agent.rejectLegacyMode = true
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := executor.probeACPSessionWithContext(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "", "dont_ask", nil, nil,
	); err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v", err)
	}
	if calls := agent.recordedCalls(); len(calls) != 0 {
		t.Fatalf("ACP calls = %v, want empty config select to suppress legacy IDs", calls)
	}
}

func TestIssue5364ProbeFallsBackOnlyForMatchingLegacyMode(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.includeConfigModeInLegacy = true
	agent.modeConfigErr = acp.NewMethodNotFound(acp.AgentMethodSessionSetConfigOption)
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := executor.probeACPSessionWithContext(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "", "session", nil, nil,
	); err != nil {
		t.Fatalf("probeACPSessionWithContext(): %v", err)
	}
	wantCalls := []string{"config:edit_approval_policy:session", "legacy:session"}
	if got := agent.recordedCalls(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("ACP calls = %v, want exact-ID fallback %v", got, wantCalls)
	}
}

func TestIssue5364ProbeDoesNotFallBackToDisjointLegacyMode(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.rejectLegacyMode = true
	agent.modeConfigErr = acp.NewMethodNotFound(acp.AgentMethodSessionSetConfigOption)
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := executor.probeACPSessionWithContext(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "", "session", nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "session/set_config_option") {
		t.Fatalf("probe error = %v, want typed method-not-found error", err)
	}
	if calls := agent.recordedCalls(); !reflect.DeepEqual(calls, []string{"config:edit_approval_policy:session"}) {
		t.Fatalf("ACP calls = %v, want disjoint legacy vocabulary rejected", calls)
	}
}

func TestIssue5364ProbeDoesNotFallBackAfterOtherConfigModeError(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.rejectLegacyMode = true
	agent.includeConfigModeInLegacy = true
	agent.modeConfigErr = errors.New("typed mode rejected")
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := executor.probeACPSessionWithContext(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "", "session", nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "session/set_config_option") {
		t.Fatalf("probe error = %v, want typed mode error", err)
	}
	if calls := agent.recordedCalls(); !reflect.DeepEqual(calls, []string{"config:edit_approval_policy:session"}) {
		t.Fatalf("ACP calls = %v, want no legacy fallback", calls)
	}
}

func TestIssue5364UtilityUsesTypedProfileMode(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.rejectLegacyMode = true
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := executor.executeACPSession(
		ctx,
		clientWriter,
		clientReader,
		t.TempDir(),
		"fixture-acp",
		"first prompt",
		"",
		nil,
		"session",
		nil,
		nil,
		nil,
	); err != nil {
		t.Fatalf("executeACPSession(): %v", err)
	}

	wantCalls := []string{"config:edit_approval_policy:session", "prompt"}
	if got := agent.recordedCalls(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("ACP calls = %v, want confirmed mode before prompt %v", got, wantCalls)
	}
	if got := agent.promptMode(); got != "session" {
		t.Fatalf("mode at first prompt = %q, want session", got)
	}
}

func TestIssue5364UtilityAcceptsModeResponseWithoutCategory(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.omitModeCategoryInResponse = true
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := executor.executeACPSession(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "first prompt", "", nil,
		"session", nil, nil, nil,
	); err != nil {
		t.Fatalf("executeACPSession(): %v", err)
	}
	if calls := agent.recordedCalls(); !reflect.DeepEqual(calls, []string{"config:edit_approval_policy:session", "prompt"}) {
		t.Fatalf("ACP calls = %v, want prompt after matching mode value without a category", calls)
	}
	if got := agent.promptMode(); got != "session" {
		t.Fatalf("mode at first prompt = %q, want session", got)
	}
}

func TestIssue5364UtilityRequiresExactModeConfirmationBeforePrompt(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.modeResponseValue = "workspace"
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := executor.executeACPSession(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "first prompt", "", nil,
		"session", nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "confirmed") {
		t.Fatalf("executeACPSession() error = %v, want mode confirmation error", err)
	}
	if calls := agent.recordedCalls(); !reflect.DeepEqual(calls, []string{"config:edit_approval_policy:session"}) {
		t.Fatalf("ACP calls = %v, want prompt blocked after mismatched snapshot", calls)
	}
}

func TestIssue5364UtilityRequiresSnapshotWhenTypedModeResponseIsEmpty(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.omitModeSnapshot = true
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := executor.executeACPSession(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "first prompt", "", nil,
		"session", nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "confirmed") {
		t.Fatalf("executeACPSession() error = %v, want missing mode confirmation", err)
	}
	if calls := agent.recordedCalls(); !reflect.DeepEqual(calls, []string{"config:edit_approval_policy:session"}) {
		t.Fatalf("ACP calls = %v, want prompt blocked without a snapshot", calls)
	}
}

func TestIssue5364UtilityAcceptsModeConfirmedByConfigUpdate(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.omitModeSnapshot = true
	agent.notifyModeSnapshot = true
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := executor.executeACPSession(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "first prompt", "", nil,
		"session", nil, nil, nil,
	); err != nil {
		t.Fatalf("executeACPSession(): %v", err)
	}
	if calls := agent.recordedCalls(); !reflect.DeepEqual(calls, []string{"config:edit_approval_policy:session", "prompt"}) {
		t.Fatalf("ACP calls = %v, want prompt after config update confirmation", calls)
	}
	if got := agent.promptMode(); got != "session" {
		t.Fatalf("mode at first prompt = %q, want session", got)
	}
}

func TestIssue5364UtilityRejectsModeUpdateFromForeignSession(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.omitModeSnapshot = true
	agent.notifyModeSnapshot = true
	agent.foreignModeSnapshot = true
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := executor.executeACPSession(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "first prompt", "", nil,
		"session", nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "confirmed") {
		t.Fatalf("executeACPSession() error = %v, want foreign-session mode update rejected", err)
	}
	if calls := agent.recordedCalls(); !reflect.DeepEqual(calls, []string{"config:edit_approval_policy:session"}) {
		t.Fatalf("ACP calls = %v, want prompt blocked after foreign-session update", calls)
	}
}

func TestIssue5364UtilityUsesModeCatalogFromModelConfigUpdate(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.notifyModelCatalog = true
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := executor.executeACPSession(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "first prompt", "next-model", nil,
		"session", nil, nil, nil,
	); err != nil {
		t.Fatalf("executeACPSession(): %v", err)
	}
	wantCalls := []string{
		"config:model:next-model",
		"config:approval_mode_v2:session",
		"prompt",
	}
	if got := agent.recordedCalls(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("ACP calls = %v, want model-updated mode option before prompt %v", got, wantCalls)
	}
	if got := agent.promptMode(); got != "session" {
		t.Fatalf("mode at first prompt = %q, want session", got)
	}
}

func TestIssue5364UtilityRejectsModelConfigValueMissingFromUpdatedCatalog(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.notifyModelCatalog = true
	agent.includeReasoningOption = true
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := executor.executeACPSession(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "first prompt", "next-model",
		map[string]string{"reasoning_effort": "high"}, "", nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "not advertised") {
		t.Fatalf("executeACPSession() error = %v, want unsupported explicit model setting", err)
	}
	if calls := agent.recordedCalls(); !reflect.DeepEqual(calls, []string{"config:model:next-model"}) {
		t.Fatalf("ACP calls = %v, want prompt and unsupported setting blocked after model update", calls)
	}
}

func TestIssue5364NotificationStateRetainsEmptyConfigSnapshot(t *testing.T) {
	state := newACPProbeNotificationState("fixture-acp")
	state.setSessionID("active-session")
	previousVersion := state.currentConfigUpdateVersion()
	state.handle(acp.SessionNotification{
		SessionId: "active-session",
		Update: acp.SessionUpdate{ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
			ConfigOptions: []acp.SessionConfigOption{},
		}},
	})

	options, received, err := state.waitForConfigUpdate(context.Background(), previousVersion)
	if err != nil {
		t.Fatalf("waitForConfigUpdate(): %v", err)
	}
	if !received || options == nil || len(options) != 0 {
		t.Fatalf("config update = (%#v, %t), want a received, explicit empty snapshot", options, received)
	}
}

func TestIssue5364UtilityContinuesAfterLegacyModeAckWithoutConfigUpdate(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	agent.includeConfigModeInLegacy = true
	agent.legacyOnlyMode = true
	agent.legacyNoConfigUpdate = true
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	if _, err := executor.executeACPSession(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "first prompt", "", nil,
		"session", nil, nil, nil,
	); err != nil {
		t.Fatalf("executeACPSession() after legacy ACK: %v", err)
	}
	wantCalls := []string{"legacy:session", "prompt"}
	if got := agent.recordedCalls(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("ACP calls = %v, want prompt immediately after legacy ACK %v", got, wantCalls)
	}
	if got := agent.promptMode(); got != "session" {
		t.Fatalf("mode at first prompt = %q, want session", got)
	}
}

func TestIssue5364UtilityRejectsSavedLegacyIDOutsideTypedCatalog(t *testing.T) {
	agent, clientWriter, clientReader := newIssue5364ACPAgent(t)
	executor := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := executor.executeACPSession(
		ctx, clientWriter, clientReader, t.TempDir(), "fixture-acp", "first prompt", "", nil,
		"dont_ask", nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "not advertised") {
		t.Fatalf("executeACPSession() error = %v, want saved mode rejection", err)
	}
	if calls := agent.recordedCalls(); len(calls) != 0 {
		t.Fatalf("ACP calls = %v, want no mutation or prompt for an unadvertised ID", calls)
	}
}

type issue5364ACPAgent struct {
	*probeCaptureAgent
	conn                       *acp.AgentSideConnection
	mu                         sync.Mutex
	calls                      []string
	currentMode                string
	currentModel               string
	promptModeID               string
	rejectLegacyMode           bool
	emptyModeChoices           bool
	includeConfigModeInLegacy  bool
	foreignModeSnapshot        bool
	legacyOnlyMode             bool
	legacyNoConfigUpdate       bool
	notifyModelCatalog         bool
	modeResponseValue          string
	modeConfigErr              error
	omitModeSnapshot           bool
	omitModeCategoryInResponse bool
	includeReasoningOption     bool
	notifyModeSnapshot         bool
}

func newIssue5364ACPAgent(t *testing.T) (*issue5364ACPAgent, io.Writer, io.Reader) {
	t.Helper()
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	t.Cleanup(func() {
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
	})

	agent := &issue5364ACPAgent{
		probeCaptureAgent: &probeCaptureAgent{},
		currentMode:       "workspace",
		currentModel:      "fixture-model",
	}
	agent.conn = acp.NewAgentSideConnection(agent, agentToClientWriter, clientToAgentReader)
	return agent, clientToAgentWriter, agentToClientReader
}

func (a *issue5364ACPAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	a.mu.Lock()
	includeConfigModeInLegacy := a.includeConfigModeInLegacy
	a.mu.Unlock()
	legacyModes := []acp.SessionMode{
		{Id: "dont_ask", Name: "Don't Ask"},
		{Id: "ask", Name: "Ask"},
	}
	if includeConfigModeInLegacy {
		legacyModes = append(legacyModes, acp.SessionMode{Id: "session", Name: "Don't Ask"})
	}
	return acp.NewSessionResponse{
		SessionId: acp.SessionId("issue-5364-session"),
		Modes: &acp.SessionModeState{
			CurrentModeId:  "dont_ask",
			AvailableModes: legacyModes,
		},
		ConfigOptions: a.configOptions(),
	}, nil
}

func (a *issue5364ACPAgent) SetSessionConfigOption(
	ctx context.Context,
	request acp.SetSessionConfigOptionRequest,
) (acp.SetSessionConfigOptionResponse, error) {
	if request.ValueId == nil {
		return acp.SetSessionConfigOptionResponse{}, errors.New("missing config value")
	}
	configID := string(request.ValueId.ConfigId)
	value := string(request.ValueId.Value)
	a.recordCall(fmt.Sprintf("config:%s:%s", configID, value))
	a.mu.Lock()
	switch configID {
	case "edit_approval_policy":
		if a.currentModel == "next-model" {
			a.mu.Unlock()
			return acp.SetSessionConfigOptionResponse{}, errors.New("stale mode config option")
		}
		fallthrough
	case "approval_mode_v2":
		if a.modeConfigErr != nil {
			a.mu.Unlock()
			return acp.SetSessionConfigOptionResponse{}, a.modeConfigErr
		}
		a.currentMode = value
		if a.omitModeSnapshot {
			options := a.configOptionsLocked()
			notify := a.notifyModeSnapshot
			sessionID := request.ValueId.SessionId
			if a.foreignModeSnapshot {
				sessionID = "foreign-session"
			}
			a.mu.Unlock()
			if notify {
				err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
					SessionId: sessionID,
					Update: acp.SessionUpdate{ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
						ConfigOptions: options,
					}},
				})
				if err != nil {
					return acp.SetSessionConfigOptionResponse{}, err
				}
			}
			return acp.SetSessionConfigOptionResponse{}, nil
		}
		if a.modeResponseValue != "" {
			a.currentMode = a.modeResponseValue
		}
	case "model":
		a.currentModel = value
		if a.notifyModelCatalog {
			options := a.configOptionsLocked()
			a.mu.Unlock()
			if err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
				SessionId: request.ValueId.SessionId,
				Update: acp.SessionUpdate{ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
					ConfigOptions: options,
				}},
			}); err != nil {
				return acp.SetSessionConfigOptionResponse{}, err
			}
			return acp.SetSessionConfigOptionResponse{}, nil
		}
	default:
		a.mu.Unlock()
		return acp.SetSessionConfigOptionResponse{}, fmt.Errorf("unexpected config option %q", configID)
	}
	options := a.configOptionsLocked()
	if configID == "edit_approval_policy" || configID == "approval_mode_v2" {
		if a.omitModeCategoryInResponse {
			for _, option := range options {
				if option.Select != nil && option.Select.Id == request.ValueId.ConfigId {
					option.Select.Category = nil
					break
				}
			}
		}
	}
	a.mu.Unlock()
	return acp.SetSessionConfigOptionResponse{ConfigOptions: options}, nil
}

func (a *issue5364ACPAgent) SetSessionMode(
	ctx context.Context,
	request acp.SetSessionModeRequest,
) (acp.SetSessionModeResponse, error) {
	mode := string(request.ModeId)
	a.recordCall("legacy:" + mode)
	if a.rejectLegacyMode {
		return acp.SetSessionModeResponse{}, errors.New("legacy mode vocabulary rejected")
	}
	a.mu.Lock()
	a.currentMode = mode
	noConfigUpdate := a.legacyNoConfigUpdate
	options := a.configOptionsLocked()
	a.mu.Unlock()
	if noConfigUpdate {
		return acp.SetSessionModeResponse{}, nil
	}
	sessionID := request.SessionId
	if a.foreignModeSnapshot {
		sessionID = "foreign-session"
	}
	return acp.SetSessionModeResponse{}, a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: sessionID,
		Update: acp.SessionUpdate{ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
			ConfigOptions: options,
		}},
	})
}

func (a *issue5364ACPAgent) Prompt(context.Context, acp.PromptRequest) (acp.PromptResponse, error) {
	a.recordCall("prompt")
	a.mu.Lock()
	a.promptModeID = a.currentMode
	a.mu.Unlock()
	return acp.PromptResponse{}, nil
}

func (a *issue5364ACPAgent) configOptions() []acp.SessionConfigOption {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.configOptionsLocked()
}

func (a *issue5364ACPAgent) configOptionsLocked() []acp.SessionConfigOption {
	modeCategory := acp.SessionConfigOptionCategoryMode
	modelCategory := acp.SessionConfigOptionCategoryModel
	modeOptionID := acp.SessionConfigId("edit_approval_policy")
	if a.currentModel == "next-model" {
		modeOptionID = "approval_mode_v2"
	}
	modeChoices := acp.SessionConfigSelectOptionsUngrouped{}
	if !a.emptyModeChoices {
		modeChoices = append(modeChoices,
			acp.SessionConfigSelectOption{Name: "Don't Ask", Value: "session"},
			acp.SessionConfigSelectOption{Name: "Ask", Value: "workspace"},
		)
	}
	options := []acp.SessionConfigOption{}
	if !a.legacyOnlyMode {
		options = append(options, acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
			Id:           modeOptionID,
			Name:         "Approval policy",
			Category:     &modeCategory,
			Type:         "select",
			CurrentValue: acp.SessionConfigValueId(a.currentMode),
			Options:      acp.SessionConfigSelectOptions{Ungrouped: &modeChoices},
		}})
	}
	options = append(options, acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
		Id:           "model",
		Name:         "Model",
		Category:     &modelCategory,
		Type:         "select",
		CurrentValue: acp.SessionConfigValueId(a.currentModel),
		Options: acp.SessionConfigSelectOptions{Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
			{Name: "Fixture Model", Value: "fixture-model"},
		}},
	}})
	if a.includeReasoningOption {
		effortCategory := acp.SessionConfigOptionCategoryThoughtLevel
		effortValue := acp.SessionConfigValueId("high")
		if a.currentModel == "next-model" {
			effortValue = "low"
		}
		options = append(options, acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
			Id:           "reasoning_effort",
			Name:         "Reasoning effort",
			Category:     &effortCategory,
			Type:         "select",
			CurrentValue: effortValue,
			Options: acp.SessionConfigSelectOptions{Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
				{Name: "Selected effort", Value: effortValue},
			}},
		}})
	}
	return options
}

func (a *issue5364ACPAgent) recordCall(call string) {
	a.mu.Lock()
	a.calls = append(a.calls, call)
	a.mu.Unlock()
}

func (a *issue5364ACPAgent) recordedCalls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.calls...)
}

func (a *issue5364ACPAgent) promptMode() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.promptModeID
}
