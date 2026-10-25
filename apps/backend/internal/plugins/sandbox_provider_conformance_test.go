package plugins

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	pluginruntime "github.com/kandev/kandev/internal/plugins/runtime"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// @covers AC-PLUGINS-SANDBOX-001.4
func TestSandboxProviderPackagedContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "plugin-fixture")
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, "../../cmd/plugin-fixture")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	content, err := os.ReadFile(binary)
	require.NoError(t, err)
	svc, dir, _, _ := newTestServiceWithDir(t)
	svc.SetSandboxProvidersEnabled(true)
	runtime := pluginruntime.NewManager(dir, svc.handleStatusChange, testLogger(t))
	svc.SetRuntime(runtime)
	installed, err := svc.Install(ctx, sandboxFixturePackage(t, content, "1.0.0"))
	require.NoError(t, err)
	for _, key := range []string{"fixture-alpha", "fixture-beta"} {
		t.Run(key, func(t *testing.T) { assertPackagedSandboxCalls(t, ctx, svc, installed, key) })
	}
	binding := sandboxFixtureBinding(installed, "fixture-alpha")
	req := sandboxFixtureCompileRequest(binding)
	require.NoError(t, svc.Disable(installed.ID))
	_, err = svc.CompileSandboxPolicy(ctx, binding, req)
	require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
	require.NoError(t, svc.Enable(installed.ID))
	_, err = svc.CompileSandboxPolicy(ctx, binding, req)
	require.NoError(t, err)
	upgraded, err := svc.Install(ctx, sandboxFixturePackage(t, content, "1.0.1"))
	require.NoError(t, err)
	_, err = svc.CompileSandboxPolicy(ctx, binding, req)
	require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
	binding = sandboxFixtureBinding(upgraded, "fixture-alpha")
	req = sandboxFixtureCompileRequest(binding)
	_, err = svc.CompileSandboxPolicy(ctx, binding, req)
	require.NoError(t, err)
	require.NoError(t, svc.Uninstall(ctx, installed.ID))
	_, err = svc.CompileSandboxPolicy(ctx, binding, req)
	require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
}

func sandboxFixtureBinding(record *store.Record, key string) SandboxProviderBinding {
	return SandboxProviderBinding{PluginID: record.ID, InstallationID: record.InstallationID, ProviderKey: key, Version: record.Version, InstalledAt: record.InstalledAt, PackageDigest: record.PackageDigest}
}

func sandboxFixtureCompileRequest(binding SandboxProviderBinding) *pluginsdk.CompileSandboxPolicyRequest {
	return &pluginsdk.CompileSandboxPolicyRequest{
		Context: &pluginsdk.SandboxRequestContext{ProtocolMajor: 1, InstallationId: binding.InstallationID, ProviderKey: binding.ProviderKey, OperationId: "operation-1", AttemptId: "attempt-1", ExecutionId: "execution-1", Generation: 1},
		Policy:  &pluginsdk.SandboxPolicySnapshot{PolicyId: "policy-1", Revision: 1, Digest: "policy-digest", Grants: &pluginsdk.SandboxGrants{}},
	}
}

func assertPackagedSandboxCalls(t *testing.T, ctx context.Context, svc *Service, record *store.Record, key string) {
	t.Helper()
	binding := sandboxFixtureBinding(record, key)
	req := sandboxFixtureCompileRequest(binding)
	described, err := svc.DescribeSandboxProvider(ctx, binding, &pluginsdk.DescribeSandboxProviderRequest{Context: req.Context})
	require.NoError(t, err)
	require.Equal(t, key, described.Description.DisplayName)
	compiled, err := svc.CompileSandboxPolicy(ctx, binding, req)
	require.NoError(t, err)
	require.True(t, proto.Equal(req.Policy.Grants, compiled.Compiled.EffectiveGrants))
	approval, err := svc.NormalizeSandboxApproval(ctx, binding, &pluginsdk.NormalizeSandboxApprovalRequest{
		Context: req.Context, Policy: req.Policy,
		VerifiedEvent: []byte(`{"request_id":"r1","kind":"filesystem","target":"/workspace/file","access":"read","supported_decisions":["deny","allow_once"]}`),
	})
	require.NoError(t, err)
	require.Equal(t, "r1", approval.Operation.RequestId)
	evidence, err := svc.NormalizeSandboxStatus(ctx, binding, &pluginsdk.NormalizeSandboxStatusRequest{Context: req.Context, Policy: req.Policy})
	require.NoError(t, err)
	require.Empty(t, evidence.Evidence.PolicyDigest)
	denial := &pluginsdk.SandboxDenialRecord{RecordId: "denial-1", Operation: "read", Target: "/workspace/file", PolicyDigest: req.Policy.Digest, AttemptId: req.Context.AttemptId}
	audit, err := protojson.Marshal(&pluginsdk.ReadSandboxReportResponse{Records: []*pluginsdk.SandboxDenialRecord{denial}, Complete: true, Integrity: pluginsdk.SandboxReportComplete})
	require.NoError(t, err)
	report, err := svc.ReadSandboxReport(ctx, binding, &pluginsdk.ReadSandboxReportRequest{Context: req.Context, Policy: req.Policy, Audit: audit})
	require.NoError(t, err)
	require.Len(t, report.Records, 1)
	revision, err := svc.ProposeSandboxRevision(ctx, binding, &pluginsdk.ProposeSandboxRevisionRequest{Context: req.Context, Policy: req.Policy, SelectedRecords: report.Records})
	require.NoError(t, err)
	require.Equal(t, []string{"denial-1"}, revision.SourceRecordIds)
	require.Equal(t, "/workspace/file", revision.AddedGrants.Filesystem[0].Path)
	require.Empty(t, req.Policy.Grants.Filesystem)
}

func sandboxFixturePackage(t *testing.T, binary []byte, version string) *bytes.Buffer {
	t.Helper()
	manifestYAML := fmt.Sprintf(`id: sandbox-contract-fixture
api_version: 2
version: %s
display_name: Sandbox contract fixture
runtime:
  type: binary
  executables: {%s-%s: server/plugin}
capabilities:
  sandbox_provider: true
sandbox_providers:
  - {key: fixture-alpha, display_name: fixture-alpha, protocol_major: 1}
  - {key: fixture-beta, display_name: fixture-beta, protocol_major: 1}
`, version, goruntime.GOOS, goruntime.GOARCH)
	var buffer bytes.Buffer
	require.NoError(t, pkgtartest.WritePackage(&buffer, map[string][]byte{"manifest.yaml": []byte(manifestYAML), "server/plugin": binary}))
	return &buffer
}
