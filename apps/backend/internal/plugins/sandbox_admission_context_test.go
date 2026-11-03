package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"testing/synctest"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestSandboxProviderAdmissionHonorsCancellationDuringLifecycleWrite(t *testing.T) {
	for _, kind := range []string{"rpc", "runtime"} {
		t.Run(kind, func(t *testing.T) {
			service, _, binding, request := sandboxWireService(t)
			call := func(ctx context.Context) error {
				_, err := service.CompileSandboxPolicy(ctx, binding, request)
				return err
			}
			if kind == "runtime" {
				content := []byte("captured runtime")
				installed, err := service.Install(t.Context(), sandboxFixturePackage(t, content, "1.0.0"))
				require.NoError(t, err)
				binding = sandboxFixtureBinding(installed, "fixture-alpha")
				digest := sha256.Sum256(content)
				artifact := &pluginsdk.SandboxRuntimeArtifact{RelativePath: "server/plugin", Sha256: hex.EncodeToString(digest[:])}
				call = func(ctx context.Context) error {
					_, err := service.ReadSandboxRuntime(ctx, binding, artifact)
					return err
				}
			}
			synctest.Test(t, func(t *testing.T) {
				service.dispatchLocks = newKeyedContextRWMutex()
				lock := service.dispatchLocks.lockFor(binding.PluginID)
				lock.Lock()
				defer lock.Unlock()
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- call(ctx) }()
				synctest.Wait()
				cancel()
				synctest.Wait()
				select {
				case err := <-done:
					require.ErrorIs(t, err, context.Canceled)
					require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
				default:
					t.Error("cancelled sandbox admission still waits for the lifecycle write lease")
				}
			})
			service.dispatchLocks = newKeyedContextRWMutex()
			require.NoError(t, call(t.Context()), "releasing the write lease admits an uncancelled request")
		})
	}
}
