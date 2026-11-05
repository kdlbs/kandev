package plugins

import (
	"strings"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestSandboxProviderRejectsMalformedResults(t *testing.T) {
	compileReq, compiled := sandboxCompilationFixture()
	for _, tc := range []struct {
		name           string
		request, reply proto.Message
	}{
		{"empty description", &pluginsdk.DescribeSandboxProviderRequest{}, &pluginsdk.DescribeSandboxProviderResponse{}},
		{"HTML description", &pluginsdk.DescribeSandboxProviderRequest{}, &pluginsdk.DescribeSandboxProviderResponse{Description: &pluginsdk.SandboxProviderDescription{DisplayName: "<b>Active</b>", ProviderVersion: "1.0.0", RuntimeVersion: "1.0.0"}}},
		{"empty approval", &pluginsdk.NormalizeSandboxApprovalRequest{}, &pluginsdk.NormalizeSandboxApprovalResponse{}},
		{"arbitrary approval", &pluginsdk.NormalizeSandboxApprovalRequest{}, &pluginsdk.NormalizeSandboxApprovalResponse{Operation: &pluginsdk.SandboxApprovalOperation{RequestId: "r1", Kind: "filesystem", Access: "read", Target: "/workspace", SupportedDecisions: []string{"save_and_allow_all"}}}},
		{"empty status", &pluginsdk.NormalizeSandboxStatusRequest{}, &pluginsdk.NormalizeSandboxStatusResponse{}},
		{"unknown status schema", &pluginsdk.NormalizeSandboxStatusRequest{}, &pluginsdk.NormalizeSandboxStatusResponse{Evidence: &pluginsdk.SandboxStatusEvidence{SchemaVersion: 999, Mechanism: "fixture"}}},
		{"too many records", &pluginsdk.ReadSandboxReportRequest{}, &pluginsdk.ReadSandboxReportResponse{Integrity: "complete", Records: make([]*pluginsdk.SandboxDenialRecord, 1001)}},
		{"large report", &pluginsdk.ReadSandboxReportRequest{}, &pluginsdk.ReadSandboxReportResponse{Integrity: "complete", NextCursor: strings.Repeat("x", (256<<10)+1)}},
		{"unselected source", &pluginsdk.ProposeSandboxRevisionRequest{Policy: compileReq.Policy, SelectedRecords: []*pluginsdk.SandboxDenialRecord{{RecordId: "chosen"}}}, &pluginsdk.ProposeSandboxRevisionResponse{Compiled: compiled, SourceRecordIds: []string{"not-chosen"}, AddedGrants: &pluginsdk.SandboxGrants{}}},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Error(t, validateSandboxReplyPayload(tc.request, tc.reply)) })
	}
}

func TestSandboxProviderAcceptsBoundedDataResults(t *testing.T) {
	req, compiled := sandboxCompilationFixture()
	for _, tc := range []struct {
		name           string
		request, reply proto.Message
	}{
		{"description", &pluginsdk.DescribeSandboxProviderRequest{}, &pluginsdk.DescribeSandboxProviderResponse{Description: &pluginsdk.SandboxProviderDescription{DisplayName: "Fixture", ProviderVersion: "1.0.0", RuntimeVersion: "1.0.0", SupportedTransports: []string{"local", "worktree"}}}},
		{"approval", &pluginsdk.NormalizeSandboxApprovalRequest{}, &pluginsdk.NormalizeSandboxApprovalResponse{Operation: &pluginsdk.SandboxApprovalOperation{RequestId: "request-1", Kind: "filesystem", Access: "read", Target: "/workspace/file", SupportedDecisions: []string{"deny", "allow_once"}}}},
		{"partial evidence", &pluginsdk.NormalizeSandboxStatusRequest{}, &pluginsdk.NormalizeSandboxStatusResponse{Evidence: &pluginsdk.SandboxStatusEvidence{SchemaVersion: 1, Mechanism: "fixture", Limitations: []string{"No confinement"}}}},
		{"report", &pluginsdk.ReadSandboxReportRequest{}, &pluginsdk.ReadSandboxReportResponse{Integrity: "complete", Complete: true}},
		{"revision", &pluginsdk.ProposeSandboxRevisionRequest{Policy: req.Policy, SelectedRecords: []*pluginsdk.SandboxDenialRecord{{RecordId: "chosen"}}}, &pluginsdk.ProposeSandboxRevisionResponse{Compiled: compiled, SourceRecordIds: []string{"chosen"}, AddedGrants: &pluginsdk.SandboxGrants{}}},
	} {
		t.Run(tc.name, func(t *testing.T) { require.NoError(t, validateSandboxReplyPayload(tc.request, tc.reply)) })
	}
}

func TestSandboxProviderRuntimeArtifactDeclaration(t *testing.T) {
	description := &pluginsdk.SandboxProviderDescription{DisplayName: "Fixture", ProviderVersion: "1.0.0", RuntimeVersion: "1.0.0"}
	require.NoError(t, validateSandboxDescription(description))
	for _, artifact := range []*pluginsdk.SandboxRuntimeArtifact{
		{RelativePath: "../runtime", Sha256: strings.Repeat("a", 64)},
		{RelativePath: "/runtime", Sha256: strings.Repeat("a", 64)},
		{RelativePath: "runtime/tool", Sha256: ""},
		{RelativePath: "runtime/tool", Sha256: strings.Repeat("z", 64)},
	} {
		description.RuntimeArtifact = artifact
		require.Error(t, validateSandboxDescription(description))
	}
	description.RuntimeArtifact = &pluginsdk.SandboxRuntimeArtifact{RelativePath: "runtime/tool", Sha256: strings.Repeat("a", 64)}
	require.NoError(t, validateSandboxDescription(description))
}

func TestSandboxProviderApprovalWireIsBoundedJSON(t *testing.T) {
	for _, tc := range []struct {
		name  string
		wire  *pluginsdk.SandboxApprovalWire
		valid bool
	}{
		{"valid", &pluginsdk.SandboxApprovalWire{ContentType: "application/json", AllowOnceBodyUtf8: `{"decision":"allow"}`, DenyBodyUtf8: `{"decision":"deny"}`, TimeoutBodyUtf8: `{"decision":"timeout"}`}, true},
		{"HTML", &pluginsdk.SandboxApprovalWire{ContentType: "text/html", AllowOnceBodyUtf8: `{}`, DenyBodyUtf8: `{}`, TimeoutBodyUtf8: `{}`}, false},
		{"missing deny", &pluginsdk.SandboxApprovalWire{ContentType: "application/json", AllowOnceBodyUtf8: `{}`, TimeoutBodyUtf8: `{}`}, false},
		{"truncated JSON", &pluginsdk.SandboxApprovalWire{ContentType: "application/json", AllowOnceBodyUtf8: `{`, DenyBodyUtf8: `{}`, TimeoutBodyUtf8: `{}`}, false},
		{"oversized", &pluginsdk.SandboxApprovalWire{ContentType: "application/json", AllowOnceBodyUtf8: `{"value":"` + strings.Repeat("x", 4096) + `"}`, DenyBodyUtf8: `{}`, TimeoutBodyUtf8: `{}`}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			description := &pluginsdk.SandboxProviderDescription{DisplayName: "Fixture", ProviderVersion: "1", RuntimeVersion: "1", ApprovalWire: tc.wire}
			err := validateSandboxDescription(description)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestSandboxProviderRuntimeSourcesRejectUnsafeDeclarations(t *testing.T) {
	for _, source := range []*pluginsdk.SandboxRuntimeSource{
		{Key: "identity", Kind: "identity", RelativeDirectory: "../outside", FilenameSuffix: ".json", MaxDepth: 1, MaxFiles: 4, MaxBytes: 1024},
		{Key: "identity", Kind: "identity", RelativeDirectory: "records", FilenameSuffix: ".json", MaxDepth: 99, MaxFiles: 4, MaxBytes: 1024},
		{Key: "identity", Kind: "unknown", RelativeDirectory: "records", FilenameSuffix: ".json", MaxDepth: 1, MaxFiles: 4, MaxBytes: 1024},
	} {
		description := &pluginsdk.SandboxProviderDescription{DisplayName: "Fixture", ProviderVersion: "1", RuntimeVersion: "1", RuntimeSources: []*pluginsdk.SandboxRuntimeSource{source}}
		require.Error(t, validateSandboxDescription(description))
	}
}

func TestSandboxProviderRegistryIdentityDoesNotRequireConfinementClaim(t *testing.T) {
	request := &pluginsdk.NormalizeSandboxStatusRequest{Process: &pluginsdk.SandboxProcessIdentity{Pid: 42, BirthId: "verified-birth"}}
	reply := &pluginsdk.NormalizeSandboxStatusResponse{Identity: &pluginsdk.SandboxRuntimeIdentity{SupervisorPid: 42, ChildPid: 43, ProviderSessionId: "provider-session"}}
	require.NoError(t, validateSandboxReplyPayload(request, reply))
	reply.Identity.SupervisorPid = 99
	require.Error(t, validateSandboxReplyPayload(request, reply))
	reply.Identity.SupervisorPid = 42
	reply.Evidence = &pluginsdk.SandboxStatusEvidence{SchemaVersion: 1, Mechanism: "claimed"}
	require.Error(t, validateSandboxReplyPayload(request, reply), "registry identity must not imply active enforcement")
}
