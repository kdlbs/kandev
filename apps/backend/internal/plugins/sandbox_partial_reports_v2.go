package plugins

import (
	"slices"
	"strings"

	"github.com/kdlbs/kandev/pluginsdk"
)

// Partial audit identities are readable evidence only. Approval and policy
// validation always use the complete operation validator.
func validSandboxAuditOperationV2(event *pluginsdk.SandboxAuditEventV2) bool {
	if event.ExactTarget {
		return validSandboxOperationV2(event.Operation)
	}
	if event.Operation == nil {
		return false
	}
	switch value := event.Operation.Value.(type) {
	case *pluginsdk.SandboxOperationV2_Command:
		return validPartialCommandAuditV2(value.Command)
	case *pluginsdk.SandboxOperationV2_Tcp:
		destination := value.Tcp
		return destination != nil && sandboxHostV2(destination.Host) && destination.Port <= 65535 && destination.Transport == sandboxTransportTCP
	case *pluginsdk.SandboxOperationV2_HttpEndpoint:
		endpoint := value.HttpEndpoint
		return validSandboxEndpointV2(endpoint) && len(endpoint.Query) == 0
	default:
		return validSandboxOperationV2(event.Operation)
	}
}

func validSandboxAuditReasonV2(value string) bool {
	return slices.Contains([]string{"", "permission_denied", "approval_denied", "approval_timeout", "approval_granted", "authentication_failed", "upstream_unavailable", "unsupported_protocol", "command_failed", "other"}, value)
}

func validPartialCommandAuditV2(command *pluginsdk.SandboxCommandInvocationV2) bool {
	return command != nil && validSandboxText(command.CommandId, 128) && !strings.ContainsAny(command.CommandId, "/\\ ") && command.ExecutablePath == "" && command.ExecutableSha256 == "" && len(command.Argv) == 0 && command.WorkDirReference == "" && command.ResourcePolicyDigest == "" && (command.Caller == "" || validSandboxText(command.Caller, 128))
}
