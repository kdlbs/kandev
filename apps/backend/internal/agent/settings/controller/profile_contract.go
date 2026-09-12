package controller

import "github.com/kandev/kandev/internal/agent/settings/dto"

// CreateProfileRequestFromDTO is the single transport-to-domain conversion for
// profile creation. It intentionally copies values without applying defaults;
// CreateProfile owns those defaults.
func CreateProfileRequestFromDTO(request dto.ProfileCreateRequest) CreateProfileRequest {
	return CreateProfileRequest{
		AgentID:        request.AgentID,
		Name:           request.Name,
		Model:          request.Model,
		FallbackModel:  request.FallbackModel,
		AutoFallback:   request.AutoFallback,
		Mode:           request.Mode,
		ConfigOptions:  request.ConfigOptions,
		AllowIndexing:  request.AllowIndexing,
		AutoApprove:    request.AutoApprove,
		CLIPassthrough: request.CLIPassthrough,
		CLIFlags:       request.CLIFlags,
		EnvVars:        request.EnvVars,
		CommandPrefix:  request.CommandPrefix,
		Dynamic:        request.Dynamic,
	}
}

// UpdateProfileRequestFromDTO is the single transport-to-domain conversion
// for partial profile updates. Pointer fields preserve omission and explicit
// empty values all the way to the controller.
func UpdateProfileRequestFromDTO(request dto.ProfileUpdateRequest) UpdateProfileRequest {
	return UpdateProfileRequest{
		ID:             request.ID,
		Name:           request.Name,
		Model:          request.Model,
		FallbackModel:  request.FallbackModel,
		AutoFallback:   request.AutoFallback,
		Mode:           request.Mode,
		ConfigOptions:  request.ConfigOptions,
		AllowIndexing:  request.AllowIndexing,
		AutoApprove:    request.AutoApprove,
		CLIPassthrough: request.CLIPassthrough,
		Enabled:        request.Enabled,
		CLIFlags:       request.CLIFlags,
		EnvVars:        request.EnvVars,
		CommandPrefix:  request.CommandPrefix,
		Dynamic:        request.Dynamic,
		Force:          request.Force,
	}
}
