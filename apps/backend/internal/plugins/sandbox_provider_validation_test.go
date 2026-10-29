package plugins

import (
	"strings"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func sandboxCompilationFixture() (*pluginsdk.CompileSandboxPolicyRequest, *pluginsdk.SandboxCompiledPolicy) {
	grants := &pluginsdk.SandboxGrants{Filesystem: []*pluginsdk.SandboxFilesystemGrant{{Path: "/workspace", Writable: true}}}
	return &pluginsdk.CompileSandboxPolicyRequest{Policy: &pluginsdk.SandboxPolicySnapshot{Grants: grants}}, &pluginsdk.SandboxCompiledPolicy{
		Prefix: []*pluginsdk.SandboxArgument{
			{Value: &pluginsdk.SandboxArgument_RuntimeReference{RuntimeReference: "sandbox.runtime"}},
			{Value: &pluginsdk.SandboxArgument_Literal{Literal: "--profile"}},
			{Value: &pluginsdk.SandboxArgument_ArtifactReference{ArtifactReference: "profile.json"}},
		},
		Artifacts:       []*pluginsdk.SandboxArtifact{{Name: "profile.json", Kind: "profile", ContentUtf8: "{}"}},
		EffectiveGrants: grants,
	}
}

// @covers AC-PLUGINS-SANDBOX-001.3
func TestSandboxProviderCompilationRejectsUnsafeArtifactsAndAuthority(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*pluginsdk.SandboxCompiledPolicy)
	}{
		{"traversal", func(c *pluginsdk.SandboxCompiledPolicy) { c.Artifacts[0].Name = "../profile" }},
		{"absolute", func(c *pluginsdk.SandboxCompiledPolicy) { c.Artifacts[0].Name = "/profile" }},
		{"backslash", func(c *pluginsdk.SandboxCompiledPolicy) { c.Artifacts[0].Name = `..\profile` }},
		{"executable", func(c *pluginsdk.SandboxCompiledPolicy) { c.Artifacts[0].Kind = "executable" }},
		{"binary", func(c *pluginsdk.SandboxCompiledPolicy) { c.Artifacts[0].ContentUtf8 = string([]byte{0xff}) }},
		{"oversized", func(c *pluginsdk.SandboxCompiledPolicy) { c.Artifacts[0].ContentUtf8 = strings.Repeat("x", (1<<20)+1) }},
		{"duplicates", func(c *pluginsdk.SandboxCompiledPolicy) { c.Artifacts = append(c.Artifacts, c.Artifacts[0]) }},
		{"too many", func(c *pluginsdk.SandboxCompiledPolicy) { c.Artifacts = make([]*pluginsdk.SandboxArtifact, 33) }},
		{"unknown artifact", func(c *pluginsdk.SandboxCompiledPolicy) {
			c.Prefix[2].Value = &pluginsdk.SandboxArgument_ArtifactReference{ArtifactReference: "missing"}
		}},
		{"unknown runtime reference", func(c *pluginsdk.SandboxCompiledPolicy) {
			c.Prefix[2].Value = &pluginsdk.SandboxArgument_RuntimeReference{RuntimeReference: "operator.token"}
		}},
		{"replacement executable", func(c *pluginsdk.SandboxCompiledPolicy) {
			c.Prefix[0].Value = &pluginsdk.SandboxArgument_Literal{Literal: "/bin/sh"}
		}},
		{"empty prefix", func(c *pluginsdk.SandboxCompiledPolicy) { c.Prefix = nil }},
		{"nil argument", func(c *pluginsdk.SandboxCompiledPolicy) { c.Prefix[1] = nil }},
		{"broader grants", func(c *pluginsdk.SandboxCompiledPolicy) {
			c.EffectiveGrants = &pluginsdk.SandboxGrants{KandevMcp: true}
		}},
		{"arbitrary status path", func(c *pluginsdk.SandboxCompiledPolicy) { c.StatusArtifacts = []string{"/etc/shadow"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, compiled := sandboxCompilationFixture()
			require.NoError(t, validateSandboxCompilation(req, compiled))
			tc.mutate(compiled)
			require.Error(t, validateSandboxCompilation(req, compiled))
		})
	}
}

func TestSandboxProviderApprovalSecretsCannotEnterArguments(t *testing.T) {
	for _, key := range []string{sandboxApprovalEndpoint, sandboxApprovalToken, sandboxEgressEndpoint} {
		for _, declared := range []bool{false, true} {
			req, compiled := sandboxCompilationFixture()
			if declared {
				req.References = []*pluginsdk.SandboxRuntimeReference{{Key: key, Value: "private-value"}}
			}
			compiled.Prefix = append(compiled.Prefix, &pluginsdk.SandboxArgument{Value: &pluginsdk.SandboxArgument_RuntimeReference{RuntimeReference: key}})
			require.Error(t, validateSandboxCompilation(req, compiled), "accepted callback material in supervisor argv")
		}
	}
}

func TestSandboxProviderArtifactBindingsRemainPrivateAndBounded(t *testing.T) {
	for _, tc := range []struct {
		name    string
		binding *pluginsdk.SandboxArtifactBinding
		content string
		valid   bool
	}{
		{"private callback", &pluginsdk.SandboxArtifactBinding{Path: []string{"callback"}, RuntimeReference: sandboxApprovalEndpoint}, `{"callback":""}`, true},
		{"private egress", &pluginsdk.SandboxArtifactBinding{Path: []string{"upstream"}, RuntimeReference: sandboxEgressEndpoint}, `{"upstream":""}`, true},
		{"unknown reference", &pluginsdk.SandboxArtifactBinding{Path: []string{"callback"}, RuntimeReference: "operator.token"}, `{"callback":""}`, false},
		{"missing destination", &pluginsdk.SandboxArtifactBinding{Path: []string{"missing"}, RuntimeReference: sandboxApprovalEndpoint}, `{"callback":""}`, false},
		{"overwrite permission", &pluginsdk.SandboxArtifactBinding{Path: []string{"enabled"}, RuntimeReference: sandboxApprovalEndpoint}, `{"enabled":true}`, false},
		{"nil binding", nil, `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, compiled := sandboxCompilationFixture()
			compiled.Artifacts[0].ContentUtf8 = tc.content
			compiled.Artifacts[0].Bindings = []*pluginsdk.SandboxArtifactBinding{tc.binding}
			err := validateSandboxCompilation(req, compiled)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
