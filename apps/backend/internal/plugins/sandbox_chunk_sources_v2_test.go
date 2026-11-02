package plugins

import (
	"strings"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestSandboxChunkSourceBindingCannotEscapeOrLoseItsSession(t *testing.T) {
	valid := &pluginsdk.SandboxAuditChunkV2{OperationHandle: "operation", ArtifactSetDigest: strings.Repeat("a", 64), SourceId: strings.Repeat("b", 64), Sequence: 0, Content: []byte("event\n"), Final: true, TotalBytes: 6, SourceDigest: strings.Repeat("c", 64), SourceSchema: "audit.v2", SourceRelativePath: "audit/session/events.ndjson", ProviderSessionId: "session", SourceIndex: 0, SourceCount: 1}
	require.True(t, validSandboxChunkV2(valid))
	for _, mutate := range []func(*pluginsdk.SandboxAuditChunkV2){
		func(v *pluginsdk.SandboxAuditChunkV2) { v.SourceRelativePath = "../foreign/events.ndjson" },
		func(v *pluginsdk.SandboxAuditChunkV2) { v.SourceRelativePath = "/private/events.ndjson" },
		func(v *pluginsdk.SandboxAuditChunkV2) { v.ProviderSessionId = "" },
		func(v *pluginsdk.SandboxAuditChunkV2) { v.ProviderSessionId = "sibling/session" },
		func(v *pluginsdk.SandboxAuditChunkV2) { v.SourceSchema = "" },
		func(v *pluginsdk.SandboxAuditChunkV2) { v.SourceIndex = 1 },
		func(v *pluginsdk.SandboxAuditChunkV2) { v.SourceCount = 0 },
		func(v *pluginsdk.SandboxAuditChunkV2) { v.SourceCount = 33 },
	} {
		value := proto.Clone(valid).(*pluginsdk.SandboxAuditChunkV2)
		mutate(value)
		require.False(t, validSandboxChunkV2(value))
	}
}

func TestSandboxChunkCanRepresentAnEmptyFrozenSource(t *testing.T) {
	chunk := &pluginsdk.SandboxAuditChunkV2{OperationHandle: "operation", ArtifactSetDigest: strings.Repeat("a", 64), SourceId: "audit.v2", Final: true, SourceDigest: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}
	require.True(t, validSandboxChunkV2(chunk), "an empty existing file differs from missing evidence")
	chunk.Final = false
	require.False(t, validSandboxChunkV2(chunk), "an empty non-final chunk cannot make progress")
}

func TestSandboxAuditSourceBoundsPreserveMajorOne(t *testing.T) {
	description := &pluginsdk.SandboxProviderDescription{DisplayName: "Fixture", ProviderVersion: "1", RuntimeVersion: "1", RuntimeSources: []*pluginsdk.SandboxRuntimeSource{{Key: "audit.v2", Kind: "audit", RelativeDirectory: "audit", FilenameSuffix: "events.ndjson", MaxDepth: 2, MaxFiles: 4, MaxBytes: 32 << 20}}}
	require.NoError(t, validateSandboxDescriptionForProtocol(description, 2))
	require.Error(t, validateSandboxDescription(description))
	description.RuntimeSources[0].Kind = "identity"
	require.Error(t, validateSandboxDescriptionForProtocol(description, 2))
}
