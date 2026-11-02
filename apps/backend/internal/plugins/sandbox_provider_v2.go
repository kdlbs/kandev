package plugins

import (
	"slices"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func sandboxMajorFields(message proto.Message, major uint32) bool {
	if major != 1 && major != 2 {
		return false
	}
	if major == 2 {
		return true
	}
	return sandboxLegacyFields(message.ProtoReflect())
}

func sandboxLegacyFields(message protoreflect.Message) bool {
	valid := true
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Number() >= 100 {
			valid = false
			return false
		}
		if field.Kind() != protoreflect.MessageKind {
			return true
		}
		if field.IsList() {
			list := value.List()
			for index := 0; index < list.Len(); index++ {
				if !sandboxLegacyFields(list.Get(index).Message()) {
					valid = false
					return false
				}
			}
		} else {
			valid = sandboxLegacyFields(value.Message())
		}
		return valid
	})
	return valid
}

func validateSandboxV2Reply(request proto.Message, reply proto.Message) error {
	switch result := reply.(type) {
	case *pluginsdk.DescribeSandboxProviderResponse:
		if validateSandboxDescription(result.Description) != nil || !validSandboxCapabilitiesV2(result.Description.GetCapabilitiesV2()) {
			return errSandboxProviderReply
		}
		return nil
	case *pluginsdk.CompileSandboxPolicyResponse:
		req, ok := request.(*pluginsdk.CompileSandboxPolicyRequest)
		if !ok {
			return errSandboxProviderReply
		}
		return validateSandboxCompilationV2(req, result)
	case *pluginsdk.NormalizeSandboxApprovalResponse:
		req, ok := request.(*pluginsdk.NormalizeSandboxApprovalRequest)
		if !ok {
			return errSandboxProviderReply
		}
		return validateSandboxApprovalReplyV2(req, result)
	case *pluginsdk.NormalizeSandboxStatusResponse:
		req, ok := request.(*pluginsdk.NormalizeSandboxStatusRequest)
		if !ok {
			return errSandboxProviderReply
		}
		return validateSandboxObservationsV2(req, result)
	case *pluginsdk.ReadSandboxReportResponse:
		req, ok := request.(*pluginsdk.ReadSandboxReportRequest)
		if !ok {
			return errSandboxProviderReply
		}
		return validateSandboxReportReplyV2(req, result)
	case *pluginsdk.ProposeSandboxRevisionResponse:
		req, ok := request.(*pluginsdk.ProposeSandboxRevisionRequest)
		if !ok {
			return errSandboxProviderReply
		}
		return validateSandboxRevisionV2(req, result)
	default:
		return errSandboxProviderReply
	}
}

func validateSandboxCompilationV2(req *pluginsdk.CompileSandboxPolicyRequest, result *pluginsdk.CompileSandboxPolicyResponse) error {
	if !validSandboxWirePolicyV2(req.Policy) || result.Compiled == nil || !proto.Equal(req.Policy.PolicyV2, result.Compiled.EffectivePolicyV2) {
		return errSandboxProviderReply
	}
	return validateSandboxCompilation(req, result.Compiled)
}

func validateSandboxApprovalReplyV2(req *pluginsdk.NormalizeSandboxApprovalRequest, result *pluginsdk.NormalizeSandboxApprovalResponse) error {
	if req.Policy.GetPolicyV2() == nil && req.ServiceBindingV2 == nil && result.ApprovalV2 == nil && result.Operation.GetKind() == pluginsdk.SandboxOperationFilesystem {
		return validateSandboxApproval(result.Operation)
	}
	if result.Operation != nil || !validSandboxApprovalV2(req, result.ApprovalV2) {
		return errSandboxProviderReply
	}
	return nil
}

func validateSandboxReportReplyV2(req *pluginsdk.ReadSandboxReportRequest, result *pluginsdk.ReadSandboxReportResponse) error {
	if req.ChunkV2 == nil && result.ReportV2 == nil && req.Policy.GetPolicyV2() == nil {
		return validateSandboxReport(result)
	}
	if len(result.Records) > 0 || result.Complete || result.Integrity != "" || proto.Size(result) > 256<<10 {
		return errSandboxProviderReply
	}
	return validateSandboxReportV2(req, result.ReportV2)
}

func validSandboxCapabilitiesV2(value *pluginsdk.SandboxCapabilitiesV2) bool {
	if value == nil || len(value.PolicySchemas) == 0 || len(value.PolicySchemas) > 2 {
		return false
	}
	for _, schema := range value.PolicySchemas {
		if schema != 1 && schema != 2 {
			return false
		}
	}
	for _, kind := range value.OperationKinds {
		if !slices.Contains([]string{pluginsdk.SandboxOperationFilesystem, pluginsdk.SandboxOperationNetwork, sandboxOperationCommand, sandboxOperationHTTPEndpoint}, kind) {
			return false
		}
	}
	for _, kind := range value.ActorKinds {
		if kind != sandboxV2ChildProcess && kind != sandboxV2RuntimeService {
			return false
		}
	}
	if !slices.Contains([]string{sandboxNetworkModeNone, sandboxV2StaticEgress, sandboxV2NetworkPreflight}, value.NetworkMode) {
		return false
	}
	for _, values := range [][]string{value.OperationKinds, value.ActorKinds, value.ObservationSchemas, value.AuditVerificationFormats} {
		if !sandboxUniqueTextV2(values, 32, 128) {
			return false
		}
	}
	return true
}

func validSandboxApprovalV2(req *pluginsdk.NormalizeSandboxApprovalRequest, value *pluginsdk.SandboxApprovalV2) bool {
	if value == nil || !validSandboxText(value.RequestId, 256) || !validSandboxOperationV2(value.Operation) || value.Actor == nil || !validSandboxDecisionsV2(value.SupportedDecisions) {
		return false
	}
	switch actor := value.Actor.Value.(type) {
	case *pluginsdk.SandboxApprovalActorV2_ChildProcess:
		return req.ServiceBindingV2 == nil && actor.ChildProcess != nil && actor.ChildProcess.Pid > 0 && actor.ChildProcess.Pid < 1<<31 && validSandboxText(actor.ChildProcess.ProviderSessionId, 256)
	case *pluginsdk.SandboxApprovalActorV2_RuntimeService:
		return validSandboxServiceBindingV2(req.ServiceBindingV2) && proto.Equal(req.ServiceBindingV2, actor.RuntimeService)
	default:
		return false
	}
}

func validSandboxDecisionsV2(values []string) bool {
	if !sandboxUniqueTextV2(values, 2, 16) || !slices.Contains(values, pluginsdk.SandboxDecisionDeny) {
		return false
	}
	for _, decision := range values {
		if decision != pluginsdk.SandboxDecisionDeny && decision != pluginsdk.SandboxDecisionAllowOnce {
			return false
		}
	}
	return true
}

func validSandboxServiceBindingV2(value *pluginsdk.SandboxServiceBindingV2) bool {
	return value != nil && validSandboxText(value.ChannelId, 256) && validSandboxText(value.ServiceKey, 128) && validSandboxText(value.RouteId, 256)
}

func validateSandboxObservationsV2(req *pluginsdk.NormalizeSandboxStatusRequest, value *pluginsdk.NormalizeSandboxStatusResponse) error {
	if len(value.ObservationsV2) > 32 {
		return errSandboxProviderReply
	}
	if value.Evidence != nil || value.Identity != nil {
		if err := validateSandboxStatus(req, value); err != nil {
			return err
		}
	}
	if value.Evidence == nil && value.Identity == nil && len(value.ObservationsV2) == 0 {
		return errSandboxProviderReply
	}
	for _, fact := range value.ObservationsV2 {
		if fact == nil || fact.SourceId != req.EvidenceSchema || !validSandboxText(fact.SourceId, 128) || fact.ObservedAtUnixMs <= 0 || !slices.Contains([]string{sandboxPlatformLinux, sandboxV2MacOS}, fact.Platform) || !sandboxUniqueTextV2(fact.Limitations, 32, 512) {
			return errSandboxProviderReply
		}
	}
	return nil
}

func sandboxUniqueTextV2(values []string, maximum, limit int) bool {
	if len(values) > maximum {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if !validSandboxText(value, limit) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}
