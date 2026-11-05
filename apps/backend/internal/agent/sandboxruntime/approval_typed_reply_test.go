//go:build linux

package sandboxruntime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestTypedApprovalReplyIsNotTreatedAsLegacy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	providers, _, _ := runtimeFixture(t, ctx, typedApprovalFixturePackage(t, ctx))
	listed, err := providers.ListSandboxProviders(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	prepared := &Prepared{orgID: "org", binding: listed[0].Binding,
		request: &pluginsdk.SandboxRequestContext{AttemptId: "attempt", ExecutionId: "execution", Generation: 1},
		policy: &pluginsdk.SandboxPolicySnapshot{PolicyV2: &pluginsdk.SandboxPolicyV2{SchemaVersion: 2,
			StaticGrants: &pluginsdk.SandboxPermissionsV2{}, ApprovalEnvelope: &pluginsdk.SandboxPermissionsV2{}}}}
	body, err := protojson.Marshal(&pluginsdk.SandboxApprovalV2{RequestId: "typed-request",
		Operation:          &pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_Filesystem{Filesystem: &pluginsdk.SandboxFilesystemOperationV2{Path: t.TempDir(), Access: "read"}}},
		Actor:              &pluginsdk.SandboxApprovalActorV2{Value: &pluginsdk.SandboxApprovalActorV2_ChildProcess{ChildProcess: &pluginsdk.SandboxChildActorV2{Pid: 123, ProviderSessionId: "session"}}},
		SupportedDecisions: []string{"deny", "allow_once"}})
	require.NoError(t, err)
	response, err := providers.NormalizeSandboxApproval(ctx, prepared.binding, &pluginsdk.NormalizeSandboxApprovalRequest{Context: prepared.operation(), Policy: prepared.policy, VerifiedEvent: body})
	require.NoError(t, err)
	require.NotNil(t, response.ApprovalV2, "the real gRPC boundary must accept this typed reply")
	require.Nil(t, response.Operation)
	manager := &Manager{providers: providers, attempts: map[string]*Prepared{"attempt": prepared}}
	attempt := &policy.Attempt{ID: "attempt", Snapshot: policy.AttemptSnapshot{OrgID: "org"}}
	require.NotPanics(t, func() {
		_, normalizeErr := manager.normalizeApproval(ctx, attempt, body)
		require.ErrorIs(t, normalizeErr, policy.ErrIneligible)
	})
}

func typedApprovalFixturePackage(t *testing.T, ctx context.Context) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "fixture")
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, "../../../cmd/plugin-fixture")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	content, err := os.ReadFile(binary)
	require.NoError(t, err)
	manifest := fmt.Sprintf(`id: typed-approval-fixture
api_version: 2
version: 1.0.0
display_name: Typed approval fixture
runtime:
  type: binary
  executables: {%s-%s: server/plugin}
capabilities:
  sandbox_provider: true
sandbox_providers:
  - {key: fixture-alpha, display_name: Fixture, protocol_major: 2}
`, runtime.GOOS, runtime.GOARCH)
	var archive bytes.Buffer
	require.NoError(t, pkgtartest.WritePackage(&archive, map[string][]byte{"manifest.yaml": []byte(manifest), "server/plugin": content}))
	path := filepath.Join(t.TempDir(), "fixture.tar.gz")
	require.NoError(t, os.WriteFile(path, archive.Bytes(), 0600))
	return path
}
