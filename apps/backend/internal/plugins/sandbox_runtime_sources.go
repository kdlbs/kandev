package plugins

import (
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kdlbs/kandev/pluginsdk"
)

func validateSandboxRuntimeSources(sources []*pluginsdk.SandboxRuntimeSource) error {
	if len(sources) > 8 {
		return errSandboxProviderReply
	}
	seen := map[string]bool{}
	for _, source := range sources {
		if source == nil || seen[source.Key] {
			return errSandboxProviderReply
		}
		declaration := sandbox.RuntimeSource{Key: source.Key, Kind: source.Kind, Directory: source.RelativeDirectory, Suffix: source.FilenameSuffix, MaxDepth: source.MaxDepth, MaxFiles: source.MaxFiles, MaxBytes: source.MaxBytes}
		if declaration.Validate() != nil {
			return errSandboxProviderReply
		}
		seen[source.Key] = true
	}
	return nil
}

func validateSandboxStatus(request *pluginsdk.NormalizeSandboxStatusRequest, result *pluginsdk.NormalizeSandboxStatusResponse) error {
	if identity := result.Identity; identity != nil {
		if result.Evidence != nil || request.Process == nil || request.Process.Pid <= 0 || request.Process.BirthId == "" {
			return errSandboxProviderReply
		}
		if identity.SupervisorPid != request.Process.Pid || identity.ChildPid <= 0 || identity.ChildPid >= 1<<31 || identity.ChildPid == identity.SupervisorPid || !validSandboxText(identity.ProviderSessionId, 256) {
			return errSandboxProviderReply
		}
		return nil
	}
	if result.Evidence == nil || result.Evidence.SchemaVersion != 1 || !validSandboxText(result.Evidence.Mechanism, 128) {
		return errSandboxProviderReply
	}
	return nil
}
