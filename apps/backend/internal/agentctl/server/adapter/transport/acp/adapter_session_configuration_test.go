package acp

import (
	"context"
	"errors"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestConfigurationResultFromOptionsUsesOnlyTheCorrelatedValue(t *testing.T) {
	// @covers AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.1, AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.2
	tests := []struct {
		name     string
		options  []streams.ConfigOption
		configID string
		want     streams.ConfigurationResult
	}{
		{
			name: "reports the effective clamp",
			options: []streams.ConfigOption{
				{ID: "model", CurrentValue: "effective-model"},
				{ID: "reasoning_effort", CurrentValue: "high"},
			},
			configID: "model",
			want: streams.ConfigurationResult{
				ConfigID: "model", Requested: "requested-model", Effective: "effective-model", Confirmed: true,
			},
		},
		{
			name: "does not infer a value from another option",
			options: []streams.ConfigOption{
				{ID: "reasoning_effort", CurrentValue: "high"},
			},
			configID: "model",
			want: streams.ConfigurationResult{
				ConfigID: "model", Requested: "requested-model",
			},
		},
		{
			name:     "missing response stays unconfirmed",
			configID: "model",
			want: streams.ConfigurationResult{
				ConfigID: "model", Requested: "requested-model",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configurationResultFromOptions(tt.options, tt.configID, "requested-model")
			if got != tt.want {
				t.Fatalf("configurationResultFromOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSetConfigOptionWithResultUsesItsACPResponse(t *testing.T) {
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
		return acp.SetSessionModeResponse{}, nil
	})
	optionCategory := acp.SessionConfigOptionCategory("reasoning_effort")
	choices := acp.SessionConfigSelectOptionsUngrouped{
		{Value: "high", Name: "High"}, {Value: "medium", Name: "Medium"},
	}
	responseOption := acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
		Type: "select", Id: "reasoning_effort", Name: "Reasoning effort",
		Category: &optionCategory, CurrentValue: "medium",
		Options: acp.SessionConfigSelectOptions{Ungrouped: &choices},
	}}
	adapter.availableConfigOptions = convertACPConfigOptions([]acp.SessionConfigOption{responseOption})
	agent.configHandler = func(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
		return acp.SetSessionConfigOptionResponse{ConfigOptions: []acp.SessionConfigOption{responseOption}}, nil
	}

	result, err := adapter.SetConfigOptionWithResult(context.Background(), "reasoning_effort", "high")
	if err != nil {
		t.Fatalf("SetConfigOptionWithResult: %v", err)
	}
	if result != (streams.ConfigurationResult{
		ConfigID: "reasoning_effort", Requested: "high", Effective: "medium", Confirmed: true,
	}) {
		t.Fatalf("configuration result = %+v, want provider-reported clamp", result)
	}
}

func TestSetConfigOptionKeepsUnconfirmedModeAsErrorForLegacyCallers(t *testing.T) {
	var adapter *Adapter
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
		return acp.SetSessionModeResponse{}, nil
	})
	agent.configHandler = func(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
		adapter.mu.Lock()
		adapter.availableConfigOptions = []streams.ConfigOption{{
			Type: "select", ID: "reasoning_effort", Category: "reasoning_effort", CurrentValue: "medium",
		}}
		adapter.mu.Unlock()
		return acp.SetSessionConfigOptionResponse{}, nil
	}
	adapter.availableConfigOptions = []streams.ConfigOption{{
		Type: "select", ID: "codex_mode", Category: "mode", CurrentValue: "default",
		Options: []streams.ConfigOptionValue{{Value: "default"}, {Value: "plan"}},
	}}

	err := adapter.SetConfigOption(context.Background(), "codex_mode", "plan")
	if err == nil {
		t.Fatal("SetConfigOption succeeded without provider confirmation after the mode catalog was replaced")
	}
}

func TestSetConfigOptionResultMarksRemoteRequestRejection(t *testing.T) {
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
		return acp.SetSessionModeResponse{}, nil
	})
	optionCategory := acp.SessionConfigOptionCategory("reasoning_effort")
	adapter.availableConfigOptions = []streams.ConfigOption{{
		Type: "select", ID: "reasoning_effort", Category: string(optionCategory), CurrentValue: "medium",
	}}
	agent.configHandler = func(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
		return acp.SetSessionConfigOptionResponse{}, &acp.RequestError{Code: -32602, Message: "unsupported value"}
	}

	_, err := adapter.SetConfigOptionWithResult(context.Background(), "reasoning_effort", "high")
	var rejected *streams.SessionConfigurationRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("SetConfigOptionWithResult error = %v, want typed provider rejection", err)
	}
}

func TestSetModelWithResultLeavesLegacyResponseUnconfirmed(t *testing.T) {
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
		return acp.SetSessionModeResponse{}, nil
	})
	modelCategory := acp.SessionConfigOptionCategoryModel
	modelOption := acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
		Type: "select", Id: "primary_model", Name: "Model", Category: &modelCategory,
		CurrentValue: "model-before",
	}}
	adapter.availableConfigOptions = convertACPConfigOptions([]acp.SessionConfigOption{modelOption})
	agent.configHandler = func(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
		return acp.SetSessionConfigOptionResponse{}, nil
	}

	result, err := adapter.SetModelWithResult(context.Background(), "model-requested")
	if err != nil {
		t.Fatalf("SetModelWithResult: %v", err)
	}
	if result.Confirmed || result.ConfigID != "primary_model" || result.Requested != "model-requested" || result.Effective != "" {
		t.Fatalf("configuration result = %+v, want missing response evidence to stay unconfirmed", result)
	}
}
