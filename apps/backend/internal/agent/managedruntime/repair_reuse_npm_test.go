package managedruntime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestManagedRuntimeRepairReuseNPM proves the full real-npm lifecycle: an
// incomplete shared execution tree fails, one replacement is published, every
// owner stops, and a fresh store reuses the generation without an installer or
// another registry request.
func TestManagedRuntimeRepairReuseNPM(t *testing.T) {
	if os.Getenv("KANDEV_TEST_REAL_NPM") != "1" {
		t.Skip("set KANDEV_TEST_REAL_NPM=1 to run the local npm integration test")
	}
	npm, err := exec.LookPath("npm")
	if err != nil {
		t.Fatalf("npm prerequisite: %v", err)
	}
	packageSpec := "managed-acp@1.2.3"
	archive := testManagedPackageArchive(t, packageSpec)
	registry, requests := newCountingManagedPackageRegistry(t, archive)
	env := npmTestEnvironment(t, registry.URL)
	home := t.TempDir()

	sharedCache := t.TempDir()
	sharedProject := t.TempDir()
	executionRoot := filepath.Join(sharedCache, "_npx", NpxExecutionCacheKey(packageSpec))
	installed := filepath.Join(executionRoot, "node_modules", "managed-acp")
	if err := os.MkdirAll(filepath.Join(installed, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed, "package.json"), []byte(`{"name":"managed-acp","version":"1.2.3","bin":{"managed-acp":"bin/cli.js"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed, "bin", "cli.js"), []byte("incomplete shared runtime"), 0o600); err != nil {
		t.Fatal(err)
	}
	sharedSentinel := filepath.Join(sharedCache, "unrelated", "keep.txt")
	if err := os.MkdirAll(filepath.Dir(sharedSentinel), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sharedSentinel, []byte("shared sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	failed := exec.CommandContext(ctx, npm, "--prefix", sharedProject, "--cache", sharedCache,
		"exec", "--yes", "--prefer-offline", "--package="+packageSpec, "--", "node", "-e", "")
	failed.Env = env
	failedOutput, failedErr := failed.CombinedOutput()
	if failedErr == nil {
		t.Fatalf("incomplete shared npm execution tree unexpectedly succeeded: %s", failedOutput)
	}
	if _, ok := VerifyNpxRootManifestEvidence(sharedCache, packageSpec, string(failedOutput)); !ok {
		t.Fatalf("npm did not report the exact missing execution root manifest: %s", failedOutput)
	}
	requestsBeforeRepair := requests.Load()

	store, err := OpenRepairStore(home)
	if err != nil {
		t.Fatalf("open repair store: %v", err)
	}
	repairContext := npmRepairContext(t, store, packageSpec, env)
	lease, err := store.BeginRecovery(ctx, repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	prepared, err := lease.Prepare(ctx, env, npmRepairRunner(t))
	if err != nil {
		t.Fatalf("prepare repaired generation: %v", err)
	}
	if prepared.ProjectPrefix() == sharedProject || prepared.CacheDirectory() == sharedCache {
		t.Fatalf("repaired paths = %q/%q, want an owned generation", prepared.ProjectPrefix(), prepared.CacheDirectory())
	}
	runPreparedInitialization(t, ctx, prepared, env, "managed-acp")
	if err := prepared.PublishRepair(); err != nil {
		t.Fatalf("publish repaired generation: %v", err)
	}
	if requests.Load() <= requestsBeforeRepair {
		t.Fatal("repair did not contact the loopback registry")
	}
	repairRequests := requests.Load()
	generationRoot := filepath.Dir(prepared.ProjectPrefix())
	if err := prepared.Close(); err != nil {
		t.Fatalf("close published owner: %v", err)
	}
	if _, err := os.Stat(generationRoot); err != nil {
		t.Fatalf("published generation after owner teardown = %v, want present", err)
	}

	restarted, err := OpenRepairStore(home)
	if err != nil {
		t.Fatalf("reopen repair store: %v", err)
	}
	reused, outcome, err := restarted.Lookup(ctx, repairContext)
	if err != nil || outcome != RepairOutcomeHit || reused == nil {
		t.Fatalf("lookup after restart = (%v, %s, %v)", reused, outcome, err)
	}
	if reused.ProjectPrefix() != prepared.ProjectPrefix() || reused.CacheDirectory() != prepared.CacheDirectory() {
		t.Fatalf("reused paths = %q/%q, want stable %q/%q", reused.ProjectPrefix(), reused.CacheDirectory(), prepared.ProjectPrefix(), prepared.CacheDirectory())
	}
	runPreparedInitialization(t, ctx, reused, env, "managed-acp")
	if requests.Load() != repairRequests {
		t.Fatalf("reuse contacted the registry: before=%d after=%d", repairRequests, requests.Load())
	}
	if contents, err := os.ReadFile(sharedSentinel); err != nil || string(contents) != "shared sentinel" {
		t.Fatalf("shared sentinel = %q, %v; want preserved", contents, err)
	}
	if _, err := os.Stat(filepath.Join(executionRoot, "package.json")); !os.IsNotExist(err) {
		t.Fatalf("incomplete shared execution root manifest stat = %v, want still missing", err)
	}
	if err := reused.Close(); err != nil {
		t.Fatalf("close reused handle: %v", err)
	}
}

func npmRepairContext(t *testing.T, store *RepairStore, packageSpec string, env []string) RepairContext {
	t.Helper()
	repairContext, err := store.Context(RepairContextInput{
		OwnerScope:         RepairOwnerScopeTask("owner-a"),
		InstallerEnvDigest: InstallerEnvironmentDigest(map[string]string{"NPM_CONFIG_REGISTRY": repairEnvValue(env, "NPM_CONFIG_REGISTRY")}),
		PackageSpec:        packageSpec,
		Protocol:           "acp",
		Env:                env,
	})
	if err != nil {
		t.Fatalf("complete real-npm repair context: %v", err)
	}
	return repairContext
}

func npmRepairRunner(t *testing.T) ManagedRuntimeCommandRunner {
	t.Helper()
	return func(commandCtx context.Context, request ManagedRuntimeCommand) error {
		command := exec.CommandContext(commandCtx, request.Path, request.Args...)
		command.Dir = request.Dir
		command.Env = request.Env
		output, runErr := command.CombinedOutput()
		if runErr != nil {
			t.Logf("real npm repair output: %s", output)
		}
		return runErr
	}
}

func runPreparedInitialization(t *testing.T, ctx context.Context, prepared *PreparedRuntime, env []string, expectedName string) {
	t.Helper()
	command, err := prepared.RewriteNpxCommand([]string{
		"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, prepared.PackageSpec(),
	})
	if err != nil {
		t.Fatalf("rewrite prepared command: %v", err)
	}
	commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	process := exec.CommandContext(commandCtx, command[0], command[1:]...)
	process.Dir = prepared.ProjectPrefix()
	process.Env = env
	stdin, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Start(); err != nil {
		t.Fatalf("start prepared ACP process: %v", err)
	}
	if _, err := io.WriteString(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`+"\n"); err != nil {
		t.Fatalf("send initialize: %v", err)
	}
	line, err := bufio.NewReader(io.LimitReader(stdout, 4096)).ReadString('\n')
	if err != nil {
		t.Fatalf("read initialize response: %v", err)
	}
	_ = stdin.Close()
	if err := process.Wait(); err != nil {
		t.Fatalf("wait prepared ACP process: %v", err)
	}
	if !bytes.Contains([]byte(line), []byte(`"name":"`+expectedName+`"`)) {
		t.Fatalf("initialize response = %q, want the selected package identity", line)
	}
}

func newCountingManagedPackageRegistry(t *testing.T, archive []byte) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var requests atomic.Int64
	sum := sha1.Sum(archive)
	hash := hex.EncodeToString(sum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if strings.HasSuffix(r.URL.Path, ".tgz") {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(archive)
			return
		}
		metadata := map[string]any{
			"name": "managed-acp",
			"versions": map[string]any{
				"1.2.3": map[string]any{
					"name": "managed-acp", "version": "1.2.3",
					"bin":  map[string]string{"managed-acp": "bin/cli.js"},
					"dist": map[string]string{"tarball": fmt.Sprintf("http://%s/managed-acp-1.2.3.tgz", r.Host), "shasum": hash},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metadata)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}
