package plugins

import (
	"strings"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

const testServiceEndpoint = "sandbox.approval.service.network-proxy.connect.endpoint"

func serviceCompilationFixture() (*pluginsdk.CompileSandboxPolicyRequest, *pluginsdk.SandboxCompiledPolicy) {
	request, compiled := sandboxCompilationFixture()
	request.Context = &pluginsdk.SandboxRequestContext{ProtocolMajor: 2}
	request.Policy.Grants, compiled.EffectiveGrants = nil, nil
	request.Policy.PolicyV2 = &pluginsdk.SandboxPolicyV2{SchemaVersion: 2,
		StaticGrants:     &pluginsdk.SandboxPermissionsV2{Tcp: []*pluginsdk.SandboxTCPDestinationV2{{Host: "api.example.test", Port: 443, Transport: "tcp"}}},
		ApprovalEnvelope: &pluginsdk.SandboxPermissionsV2{}}
	compiled.EffectivePolicyV2 = request.Policy.PolicyV2
	compiled.RequiredCapabilities = []string{pluginsdk.SandboxServiceApprovalCapabilityV2}
	compiled.ApprovalServicesV2 = []*pluginsdk.SandboxApprovalServiceV2{{ServiceKey: "network-proxy", RouteId: "connect", OperationKind: "network", EndpointReference: testServiceEndpoint}}
	compiled.Artifacts[0].ContentUtf8 = `{"callback":""}`
	compiled.Artifacts[0].Bindings = []*pluginsdk.SandboxArtifactBinding{{Path: []string{"callback"}, RuntimeReference: testServiceEndpoint}}
	return request, compiled
}

func TestSandboxServiceCompilationAcceptsOnlyBoundPrivateDeclarations(t *testing.T) {
	request, compiled := serviceCompilationFixture()
	require.NoError(t, validateSandboxCompilationV2(request, &pluginsdk.CompileSandboxPolicyResponse{Compiled: compiled}))
}

func TestSandboxServiceCompilationRejectsUnboundIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*pluginsdk.CompileSandboxPolicyRequest, *pluginsdk.SandboxCompiledPolicy)
	}{
		{"missing artifact binding", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.Artifacts[0].Bindings = nil
		}},
		{"missing capability", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.RequiredCapabilities = nil
		}},
		{"upstream proxy bypass", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.RequiredCapabilities = append(c.RequiredCapabilities, "egress_proxy")
		}},
		{"undeclared upstream binding", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.Artifacts[0].ContentUtf8 = `{"callback":"","upstream":""}`
			c.Artifacts[0].Bindings = append(c.Artifacts[0].Bindings, &pluginsdk.SandboxArtifactBinding{Path: []string{"upstream"}, RuntimeReference: sandboxEgressEndpoint})
		}},
		{"nil service", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.ApprovalServicesV2[0] = nil
		}},
		{"duplicate route", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.ApprovalServicesV2 = append(c.ApprovalServicesV2, c.ApprovalServicesV2[0])
		}},
		{"unsupported kind", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.ApprovalServicesV2[0].OperationKind = "filesystem"
		}},
		{"foreign endpoint reference", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.ApprovalServicesV2[0].EndpointReference = sandboxApprovalEndpoint
		}},
		{"unsafe service key", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.ApprovalServicesV2[0].ServiceKey = "../proxy"
		}},
		{"ambiguous route separator", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.ApprovalServicesV2[0].RouteId = "other.connect"
		}},
		{"too many services", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.ApprovalServicesV2 = make([]*pluginsdk.SandboxApprovalServiceV2, 33)
		}},
		{"undeclared private endpoint", func(_ *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			c.ApprovalServicesV2 = nil
		}},
		{"legacy schema", func(r *pluginsdk.CompileSandboxPolicyRequest, c *pluginsdk.SandboxCompiledPolicy) {
			r.Policy.PolicyV2, c.EffectivePolicyV2 = nil, nil
			r.Policy.Grants = &pluginsdk.SandboxGrants{}
			c.EffectiveGrants = r.Policy.Grants
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, compiled := serviceCompilationFixture()
			test.mutate(request, compiled)
			require.Error(t, validateSandboxCompilationV2(request, &pluginsdk.CompileSandboxPolicyResponse{Compiled: compiled}))
		})
	}
}

func TestSandboxServiceEndpointCannotBecomePublicInputOrArgument(t *testing.T) {
	for _, reference := range []string{testServiceEndpoint, "sandbox.approval.service.unknown.route.endpoint", "sandbox.approval.service." + strings.Repeat("x", 1000)} {
		request, compiled := sandboxCompilationFixture()
		request.Context = &pluginsdk.SandboxRequestContext{ProtocolMajor: 2}
		request.References = []*pluginsdk.SandboxRuntimeReference{{Key: reference, Value: "injected-endpoint"}}
		require.False(t, validSandboxRequestPayload(request), "a compile caller cannot supply a private endpoint")
		compiled.Prefix = append(compiled.Prefix, &pluginsdk.SandboxArgument{Value: &pluginsdk.SandboxArgument_RuntimeReference{RuntimeReference: reference}})
		require.Error(t, validateSandboxCompilation(request, compiled), "private references cannot enter supervisor argv")
	}
}

func TestSandboxServiceDeclarationCannotEnterMajorOne(t *testing.T) {
	_, compiled := serviceCompilationFixture()
	require.False(t, sandboxMajorFields(compiled, 1))
	require.True(t, sandboxMajorFields(compiled, 2))
}

func TestSandboxServiceEndpointCannotEnterPublicEndpointCatalog(t *testing.T) {
	for _, major := range []uint32{1, 2} {
		t.Run(map[uint32]string{1: "legacy", 2: "v2"}[major], func(t *testing.T) {
			request, _ := sandboxCompilationFixture()
			request.Context = &pluginsdk.SandboxRequestContext{ProtocolMajor: major}
			request.Endpoints = []*pluginsdk.SandboxEndpoint{{Key: testServiceEndpoint, Host: "127.0.0.1", Port: 1234, Scheme: "http", Path: "/injected"}}
			require.False(t, validSandboxRequestPayload(request))
		})
	}
}
