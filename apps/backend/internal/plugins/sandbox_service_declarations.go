package plugins

import (
	"slices"

	"github.com/kdlbs/kandev/pluginsdk"
)

const sandboxApprovalServicePrefix = "sandbox.approval.service."

func validateSandboxServiceDeclarationsV2(req *pluginsdk.CompileSandboxPolicyRequest, compiled *pluginsdk.SandboxCompiledPolicy) (map[string]bool, error) {
	services := compiled.ApprovalServicesV2
	references := make(map[string]bool, len(services))
	if len(services) == 0 {
		if slices.Contains(compiled.RequiredCapabilities, pluginsdk.SandboxServiceApprovalCapabilityV2) {
			return nil, errSandboxProviderReply
		}
		return references, nil
	}
	if len(services) > 32 || !sandboxServiceCompilationV2(req, compiled) {
		return nil, errSandboxProviderReply
	}
	for _, service := range services {
		if service == nil || !sandboxServiceKeyV2(service.ServiceKey) || !sandboxServiceKeyV2(service.RouteId) || service.OperationKind != pluginsdk.SandboxOperationNetwork {
			return nil, errSandboxProviderReply
		}
		reference := sandboxApprovalServicePrefix + service.ServiceKey + "." + service.RouteId + ".endpoint"
		if service.EndpointReference != reference || references[reference] || !sandboxServiceReferenceBound(compiled.Artifacts, reference) {
			return nil, errSandboxProviderReply
		}
		references[reference] = true
	}
	return references, nil
}

func sandboxServiceCompilationV2(req *pluginsdk.CompileSandboxPolicyRequest, compiled *pluginsdk.SandboxCompiledPolicy) bool {
	return req.GetContext().GetProtocolMajor() == 2 && req.Policy.GetPolicyV2().GetSchemaVersion() == 2 &&
		slices.Contains(compiled.RequiredCapabilities, pluginsdk.SandboxServiceApprovalCapabilityV2) &&
		!slices.Contains(compiled.RequiredCapabilities, "egress_proxy") && !sandboxServiceReferenceBound(compiled.Artifacts, sandboxEgressEndpoint)
}

func sandboxServiceKeyV2(value string) bool {
	if value == "" || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, char := range value {
		if char != '-' && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}

func sandboxServiceReferenceBound(artifacts []*pluginsdk.SandboxArtifact, reference string) bool {
	for _, artifact := range artifacts {
		for _, binding := range artifact.GetBindings() {
			if binding.GetRuntimeReference() == reference {
				return true
			}
		}
	}
	return false
}
