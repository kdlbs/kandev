package profile

import (
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

const ProviderMCPScopeMetadataKey = "kandev.provider_mcp_scope"

// ProviderMCPScope identifies one immutable provider attachment inside a retained
// workspace execution. It contains no credential and grants no tool by itself.
type ProviderMCPScope struct {
	BindingID         string `json:"binding_id"`
	ExecutionID       string `json:"execution_id"`
	HelperID          string `json:"helper_id"`
	ProcessGeneration uint64 `json:"process_generation"`
	HarnessGeneration uint64 `json:"harness_generation"`
	ProfileDigest     string `json:"profile_digest"`
}

var providerBindingIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func (s ProviderMCPScope) Validate() error {
	if !providerBindingIDPattern.MatchString(s.BindingID) || s.ExecutionID == "" || s.HelperID == "" || len(s.ExecutionID) > 256 || len(s.HelperID) > 256 || strings.TrimSpace(s.ExecutionID) != s.ExecutionID || strings.TrimSpace(s.HelperID) != s.HelperID || s.ProcessGeneration == 0 || s.HarnessGeneration == 0 {
		return errors.New("provider MCP scope identity is incomplete")
	}
	if len(s.ProfileDigest) != 64 || strings.ToLower(s.ProfileDigest) != s.ProfileDigest {
		return errors.New("provider MCP scope digest is invalid")
	}
	if _, err := hex.DecodeString(s.ProfileDigest); err != nil {
		return errors.New("provider MCP scope digest is invalid")
	}
	return nil
}
