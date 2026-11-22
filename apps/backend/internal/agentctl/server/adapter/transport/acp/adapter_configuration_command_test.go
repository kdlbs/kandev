package acp

import (
	"context"
	"io"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type configurationCommandPromptAgent struct {
	*sessionRequestCaptureAgent
	connection *sdk.AgentSideConnection
	onPrompt   func(context.Context, sdk.PromptRequest) error
}

func (a *configurationCommandPromptAgent) Prompt(ctx context.Context, request sdk.PromptRequest) (sdk.PromptResponse, error) {
	if a.onPrompt != nil {
		if err := a.onPrompt(ctx, request); err != nil {
			return sdk.PromptResponse{}, err
		}
	}
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

func newConfigurationCommandPromptFixture(
	t *testing.T,
	agentID string,
	onPrompt func(*Adapter, *configurationCommandPromptAgent, context.Context, sdk.PromptRequest) error,
) *Adapter {
	t.Helper()
	const sessionID = "configuration-command-session"
	a := newTestAdapterForAgent(agentID)
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	if err := a.Connect(clientToAgentWriter, agentToClientReader); err != nil {
		t.Fatalf("connect adapter: %v", err)
	}
	agent := &configurationCommandPromptAgent{
		sessionRequestCaptureAgent: &sessionRequestCaptureAgent{
			newResponse: &sdk.NewSessionResponse{
				SessionId:     sdk.SessionId(sessionID),
				ConfigOptions: configurationCommandModelOptions("mock-fast", "mock-fast", "mock-smart", "auto"),
			},
		},
	}
	agent.onPrompt = func(ctx context.Context, request sdk.PromptRequest) error {
		return onPrompt(a, agent, ctx, request)
	}
	agent.connection = sdk.NewAgentSideConnection(agent, agentToClientWriter, clientToAgentReader)
	t.Cleanup(func() {
		_ = a.Close()
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
	})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize adapter: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return a
}

func configurationCommandModelOptions(current string, values ...string) []sdk.SessionConfigOption {
	category := sdk.SessionConfigOptionCategoryModel
	choices := make(sdk.SessionConfigSelectOptionsUngrouped, 0, len(values))
	for _, value := range values {
		name := value
		switch value {
		case "mock-fast":
			name = "Mock Fast"
		case "mock-smart":
			name = "Mock Smart"
		case "auto":
			name = "Auto"
		}
		choices = append(choices, sdk.SessionConfigSelectOption{Value: sdk.SessionConfigValueId(value), Name: name})
	}
	return []sdk.SessionConfigOption{{Select: &sdk.SessionConfigOptionSelect{
		Type: "select", Id: "model", Name: "Model", Category: &category,
		CurrentValue: sdk.SessionConfigValueId(current),
		Options:      sdk.SessionConfigSelectOptions{Ungrouped: &choices},
	}}}
}

func reportConfigurationCommandModel(ctx context.Context, agent *configurationCommandPromptAgent, model string) error {
	return agent.connection.SessionUpdate(ctx, sdk.SessionNotification{
		SessionId: "configuration-command-session",
		Update: sdk.SessionUpdate{
			ConfigOptionUpdate: &sdk.SessionConfigOptionUpdate{
				ConfigOptions: configurationCommandModelOptions(model, "mock-fast", "mock-smart", "auto"),
			},
		},
	})
}

func TestConfigurationCommandFeedbackPrompt(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		reported   string
		competing  bool
		wantResult *streams.ConfigurationCommandResult
	}{
		{
			name:     "advertised model change is confirmed",
			message:  "/model mock-smart",
			reported: "mock-smart",
			wantResult: &streams.ConfigurationCommandResult{
				Status: "applied", ConfigID: "model", Requested: "Mock Smart", Effective: "Mock Smart", Confirmed: true,
			},
		},
		{
			name:    "missing update remains unconfirmed",
			message: "/model mock-smart",
			wantResult: &streams.ConfigurationCommandResult{
				Status: "unconfirmed", ConfigID: "model", Requested: "Mock Smart", Confirmed: false,
			},
		},
		{
			name:     "unchanged update remains unconfirmed",
			message:  "/model mock-smart",
			reported: "mock-fast",
			wantResult: &streams.ConfigurationCommandResult{
				Status: "unconfirmed", ConfigID: "model", Requested: "Mock Smart", Effective: "Mock Fast", Confirmed: false,
			},
		},
		{
			name:      "competing setting change remains unconfirmed",
			message:   "/model mock-smart",
			reported:  "mock-smart",
			competing: true,
			wantResult: &streams.ConfigurationCommandResult{
				Status: "unconfirmed", ConfigID: "model", Requested: "Mock Smart", Effective: "Mock Smart", Confirmed: false,
			},
		},
		{name: "bare model command is not a setting attempt", message: "/model"},
		{name: "unadvertised model is not recognized", message: "/model unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newConfigurationCommandPromptFixture(t, "custom-acp", func(adapter *Adapter, agent *configurationCommandPromptAgent, ctx context.Context, _ sdk.PromptRequest) error {
				if tt.competing {
					adapter.beginConfigChange()
				}
				if tt.reported == "" {
					return nil
				}
				return reportConfigurationCommandModel(ctx, agent, tt.reported)
			})
			if err := a.Prompt(t.Context(), tt.message, nil, 73); err != nil {
				t.Fatalf("Prompt: %v", err)
			}
			var completion *AgentEvent
			for _, event := range drainEvents(a) {
				if event.Type == streams.EventTypeComplete {
					copy := event
					completion = &copy
				}
			}
			if completion == nil {
				t.Fatal("prompt did not emit a completion")
			}
			if tt.wantResult == nil {
				if completion.ConfigurationCommandResult != nil {
					t.Fatalf("configuration result = %+v, want none", completion.ConfigurationCommandResult)
				}
				return
			}
			if got := completion.ConfigurationCommandResult; got == nil || *got != *tt.wantResult {
				t.Fatalf("configuration result = %+v, want %+v", got, tt.wantResult)
			}
		})
	}
}

func TestConfigurationCommandFeedbackPromptHandlesProviderCommandShapes(t *testing.T) {
	options := []streams.ConfigOption{{
		Type: "select", ID: "model", Category: "model", CurrentValue: "mock-fast",
		Options: []streams.ConfigOptionValue{{Value: "auto", Name: "Auto"}},
	}}
	copilot := copilotConfigurationCommand("/model auto hydrafusion", nil, options, nil)
	if copilot == nil || copilot.requested != "Fusion High" || !copilot.partialConfiguration {
		t.Fatalf("Copilot command spec = %+v, want partial Fusion High request", copilot)
	}
	tracker := &configurationCommandTracker{
		sessionID: "session-1", promptGeneration: 5, spec: *copilot,
		baseline: "mock-fast", configGeneration: 2, observed: map[string]string{"auto": "Auto"},
	}
	got := configurationCommandResult(tracker, 2)
	want := streams.ConfigurationCommandResult{
		Status: "clamped", ConfigID: "model", Requested: "Fusion High", Effective: "Auto", Confirmed: false,
	}
	if got == nil || *got != want {
		t.Fatalf("Copilot result = %+v, want %+v", got, want)
	}

	commands := []streams.AvailableCommand{{
		Name:   "plan",
		Action: &streams.AvailableCommandAction{Kind: normalizedSetConfigOptionActionKind, ConfigID: "collaboration_mode", Value: "plan"},
	}}
	modeOptions := []streams.ConfigOption{{
		Type: "select", ID: "collaboration_mode", CurrentValue: "default",
		Options: []streams.ConfigOptionValue{{Value: "default", Name: "Default"}, {Value: "plan", Name: "Plan"}},
	}}
	plan := modelConfigurationCommand("/plan", nil, modeOptions, commands)
	if plan == nil || plan.configID != "collaboration_mode" || plan.requestedID != "plan" || plan.requested != "Plan" {
		t.Fatalf("Codex /plan spec = %+v, want validated collaboration_mode action", plan)
	}
}

func TestConfigurationCommandOnlyConfirmsTheExpectedTarget(t *testing.T) {
	tracker := &configurationCommandTracker{
		sessionID: "session-1", promptGeneration: 5,
		spec: configurationCommandSpec{
			configID: "model", requestedID: "mock-smart", requested: "Mock Smart",
		},
		baseline: "mock-fast", configGeneration: 2,
		observed: map[string]string{"mock-third": "Mock Third"},
	}

	got := configurationCommandResult(tracker, 2)
	want := streams.ConfigurationCommandResult{
		Status: "unconfirmed", ConfigID: "model", Requested: "Mock Smart", Effective: "Mock Third", Confirmed: false,
	}
	if got == nil || *got != want {
		t.Fatalf("result for unrelated observed model = %+v, want %+v", got, want)
	}
}

func TestCodexPlanCommandResolvesBothToggleDirections(t *testing.T) {
	commands := []streams.AvailableCommand{{
		Name: "plan",
		Action: &streams.AvailableCommandAction{
			Kind: normalizedSetConfigOptionActionKind, ConfigID: "collaboration_mode",
			Value: "plan", ResetValue: "default",
		},
	}}
	tests := []struct {
		name     string
		current  string
		observed string
		wantName string
	}{
		{name: "default enters plan", current: "default", observed: "plan", wantName: "Plan"},
		{name: "plan resets to default", current: "plan", observed: "default", wantName: "Default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := []streams.ConfigOption{{
				Type: "select", ID: "collaboration_mode", CurrentValue: tt.current,
				Options: []streams.ConfigOptionValue{
					{Value: "default", Name: "Default"}, {Value: "plan", Name: "Plan"},
				},
			}}
			spec := modelConfigurationCommand("/plan", nil, options, commands)
			if spec == nil || spec.requestedID != tt.observed || spec.requested != tt.wantName {
				t.Fatalf("Codex /plan spec = %+v, want target %q (%q)", spec, tt.observed, tt.wantName)
			}

			tracker := &configurationCommandTracker{
				sessionID: "session-1", promptGeneration: 5, spec: *spec,
				baseline: tt.current, configGeneration: 2,
				observed: map[string]string{tt.observed: tt.wantName},
			}
			got := configurationCommandResult(tracker, 2)
			want := streams.ConfigurationCommandResult{
				Status: "applied", ConfigID: "collaboration_mode", Requested: tt.wantName,
				Effective: tt.wantName, Confirmed: true,
			}
			if got == nil || *got != want {
				t.Fatalf("Codex /plan result = %+v, want %+v", got, want)
			}
		})
	}
}

func TestCopilotAutoConfigurationCommandCarriesPartialEvidence(t *testing.T) {
	a := newConfigurationCommandPromptFixture(t, copilotAgentID, func(_ *Adapter, agent *configurationCommandPromptAgent, ctx context.Context, _ sdk.PromptRequest) error {
		return reportConfigurationCommandModel(ctx, agent, "auto")
	})
	if err := a.Prompt(t.Context(), "/model auto hydrafusion", nil, 73); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	var completion *AgentEvent
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeComplete {
			copy := event
			completion = &copy
		}
	}
	if completion == nil {
		t.Fatal("prompt did not emit a completion")
	}
	want := streams.ConfigurationCommandResult{
		Status: "clamped", ConfigID: "model", Requested: "Fusion High", Effective: "Auto", Confirmed: false,
	}
	if got := completion.ConfigurationCommandResult; got == nil || *got != want {
		t.Fatalf("Copilot Auto completion result = %+v, want %+v", got, want)
	}
}
