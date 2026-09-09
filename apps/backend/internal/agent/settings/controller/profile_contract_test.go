package controller

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestProfileContractConversionsPreserveExplicitValues(t *testing.T) {
	model := ""
	autoApprove := false
	flags := []dto.CLIFlagDTO{}
	request := dto.ProfileUpdateRequest{
		ID:          "profile-1",
		Model:       &model,
		AutoApprove: &autoApprove,
		CLIFlags:    &flags,
		Force:       true,
	}
	converted := UpdateProfileRequestFromDTO(request)
	if converted.ID != request.ID || converted.Model == nil || *converted.Model != "" ||
		converted.AutoApprove == nil || *converted.AutoApprove || converted.CLIFlags == nil ||
		len(*converted.CLIFlags) != 0 || !converted.Force {
		t.Fatalf("converted update = %#v, explicit values were not preserved", converted)
	}
}

func TestProfileCreateContractConversionCopiesAllFields(t *testing.T) {
	request := dto.ProfileCreateRequest{
		AgentID: "agent-1", Name: "Profile", Model: "model", FallbackModel: "fallback",
		AutoFallback: true, Mode: "plan", ConfigOptions: map[string]string{"x": "y"},
		AllowIndexing: true, AutoApprove: true, CLIPassthrough: true,
		CLIFlags: []dto.CLIFlagDTO{{Flag: "--verbose"}}, EnvVars: []dto.ProfileEnvVarDTO{{Key: "X", Value: "Y"}},
		CommandPrefix: "prefix",
	}
	converted := CreateProfileRequestFromDTO(request)
	if converted.AgentID != request.AgentID || converted.Name != request.Name || converted.CommandPrefix != request.CommandPrefix ||
		len(converted.CLIFlags) != 1 || len(converted.EnvVars) != 1 || !converted.AutoFallback || !converted.CLIPassthrough {
		t.Fatalf("converted create = %#v, fields were not copied", converted)
	}
}
