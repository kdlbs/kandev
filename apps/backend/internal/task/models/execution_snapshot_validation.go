package models

import (
	"encoding/hex"
	"strings"

	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// ValidPublicFields checks the closed public representation before persistence
// and again before projecting stored data to a session reader.
func (s ExecutionSnapshot) ValidPublicFields() bool {
	if s.SchemaVersion != 1 || s.AttemptID == "" || s.SessionID == "" || s.TaskID == "" || s.ExecutionID == "" {
		return false
	}
	if !validCommandSummary(s.Agent) {
		return false
	}
	if s.Wrapper != nil && (s.Wrapper.Source != launchwrapper.SourceProfile || !snapshotDigest(s.Wrapper.Digest) || !validCommandSummary(s.Wrapper.Command)) {
		return false
	}
	return s.Sandbox == nil || (s.Sandbox.AttemptID == s.AttemptID && s.Sandbox.valid())
}

func validCommandSummary(command launchwrapper.CommandSummary) bool {
	if command.OmittedArguments < 0 || command.Executable != launchwrapper.SummarizeCommand([]string{command.Executable}).Executable {
		return false
	}
	for _, reference := range command.References {
		if reference != launchwrapper.WorkDir && reference != launchwrapper.ManagedNPMDir {
			return false
		}
	}
	return true
}

func (s ExecutionSandboxSnapshot) valid() bool {
	if s.AttemptID == "" || s.Generation == 0 || s.InstallationID == "" || s.ProviderKey == "" || s.ProviderVersion == "" || s.RuntimeVersion == "" || s.RevisionID == "" || s.RevisionNumber < 1 || s.CeilingRevisionID == "" {
		return false
	}
	switch s.SelectionSource {
	case "task", "workflow_step", "agent", "executor":
	default:
		return false
	}
	return snapshotDigest(s.PackageDigest) && snapshotDigest(s.RuntimeDigest) && snapshotDigest(s.PolicyDigest) && snapshotDigest(s.CeilingDigest)
}

func snapshotDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
