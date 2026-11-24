package controller

import (
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/profileconfig"
)

// ErrInvalidDevinPermissionIntent reports a Devin mode/sandbox pair the provider
// cannot accept. It keeps the owning helper authoritative.
var ErrInvalidDevinPermissionIntent = profileconfig.ErrInvalidDevinPermissionIntent

func profileConfigEnvEntries(envVars []dto.ProfileEnvVarDTO) []profileconfig.EnvEntry {
	entries := make([]profileconfig.EnvEntry, 0, len(envVars))
	for _, envVar := range envVars {
		entries = append(entries, profileconfig.EnvEntry{
			Key: envVar.Key, Value: envVar.Value, SecretID: envVar.SecretID,
		})
	}
	return entries
}

func profileConfigEnvVars(entries []profileconfig.EnvEntry) []dto.ProfileEnvVarDTO {
	envVars := make([]dto.ProfileEnvVarDTO, 0, len(entries))
	for _, entry := range entries {
		envVars = append(envVars, dto.ProfileEnvVarDTO{
			Key: entry.Key, Value: entry.Value, SecretID: entry.SecretID,
		})
	}
	return envVars
}

// devinProfilePermissionDefaults applies new-profile permission intent only on
// creation paths; saved records keep their explicit or inherited values.
func devinProfilePermissionDefaults(
	agentName, mode string,
	envVars []dto.ProfileEnvVarDTO,
	addDisabledDefault bool,
) (string, []dto.ProfileEnvVarDTO) {
	mode, entries := profileconfig.ApplyDevinProfileDefaults(
		agentName, mode, profileConfigEnvEntries(envVars), addDisabledDefault,
	)
	return mode, profileConfigEnvVars(entries)
}

func validateDevinProfilePermissionIntent(agentName, mode string, envVars []dto.ProfileEnvVarDTO) error {
	return profileconfig.ValidateDevinPermissionIntent(agentName, mode, profileConfigEnvEntries(envVars))
}

// resolutionModeOverride returns the model-resolution mode as a permission
// intent override only when the caller supplied one. An omitted mode keeps the
// saved or draft value.
func resolutionModeOverride(mode string) *string {
	if mode == "" {
		return nil
	}
	return &mode
}
