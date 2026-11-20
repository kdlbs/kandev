package managedruntime

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
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
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestManagedRuntimeRepairReuseNPMWithCLIArgs proves the incident-shaped
// scoped package and permission flag recover once and then reuse on a cold
// launch without another installer or archive download.
func TestManagedRuntimeRepairReuseNPMWithCLIArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reusable repair requires native ACL verification on Windows")
	}
	if os.Getenv("KANDEV_TEST_REAL_NPM") != "1" {
		t.Skip("set KANDEV_TEST_REAL_NPM=1 to run the local npm integration test")
	}
	npm, err := exec.LookPath("npm")
	if err != nil {
		t.Fatalf("npm prerequisite: %v", err)
	}
	packageName := "@agentclientprotocol/claude-agent-acp"
	version := "0.100.0"
	binName := "claude-agent-acp"
	packageSpec := packageName + "@" + version
	archive := testScopedManagedPackageArchive(t, packageName, version, binName)
	registry, requests, downloads := newScopedManagedPackageRegistry(t, packageName, version, binName, archive)
	env := npmTestEnvironment(t, registry.URL)
	home := t.TempDir()
	argvCapture := filepath.Join(t.TempDir(), "argv")
	env = append(env, "ARGV_CAPTURE="+argvCapture)
	flags := []string{"--dangerously-skip-permissions"}
	sharedCache := t.TempDir()
	sharedProject := t.TempDir()
	executionRoot := filepath.Join(sharedCache, "_npx", NpxExecutionCacheKey(packageSpec))
	installed := filepath.Join(executionRoot, "node_modules", filepath.FromSlash(packageName))
	if err := os.MkdirAll(filepath.Join(installed, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed, "package.json"), []byte(`{"name":"`+packageName+`","version":"`+version+`","bin":{"`+binName+`":"bin/cli.js"}}`), 0o600); err != nil {
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
		t.Fatalf("incomplete scoped shared tree unexpectedly succeeded: %s", failedOutput)
	}
	if _, ok := VerifyNpxRootManifestEvidence(sharedCache, packageSpec, string(failedOutput)); !ok {
		t.Fatalf("npm did not report the exact missing execution root manifest: %s", failedOutput)
	}
	downloadsBeforeRepair := downloads.Load()

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
	runPreparedInitializationWithArgs(t, ctx, prepared, env, binName, flags)
	assertCapturedCLIArg(t, argvCapture, flags)
	if err := prepared.PublishRepair(); err != nil {
		t.Fatalf("publish repaired generation: %v", err)
	}
	if downloads.Load() <= downloadsBeforeRepair {
		t.Fatal("repair did not download the selected archive")
	}
	repairDownloads := downloads.Load()
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
	runPreparedInitializationWithArgs(t, ctx, reused, env, binName, flags)
	assertCapturedCLIArg(t, argvCapture, flags)
	if downloads.Load() != repairDownloads {
		t.Fatalf("reuse downloaded another archive: before=%d after=%d", repairDownloads, downloads.Load())
	}
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

func runPreparedInitializationWithArgs(
	t *testing.T,
	ctx context.Context,
	prepared *PreparedRuntime,
	env []string,
	expectedName string,
	flags []string,
) {
	t.Helper()
	command := []string{"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, prepared.PackageSpec()}
	command = append(command, flags...)
	rewritten, err := prepared.RewriteNpxCommand(command)
	if err != nil {
		t.Fatalf("rewrite prepared command: %v", err)
	}
	commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	process := exec.CommandContext(commandCtx, rewritten[0], rewritten[1:]...)
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

func testScopedManagedPackageArchive(t *testing.T, packageName, version, binName string) []byte {
	t.Helper()
	manifest, err := json.Marshal(map[string]any{
		"name": packageName, "version": version, "bin": map[string]string{binName: "bin/cli.js"},
	})
	if err != nil {
		t.Fatal(err)
	}
	program := "#!/usr/bin/env node\n" +
		"const fs = require('node:fs'); if (process.env.ARGV_CAPTURE) fs.writeFileSync(process.env.ARGV_CAPTURE, JSON.stringify(process.argv.slice(2)));\n" +
		"let input = ''; process.stdin.setEncoding('utf8'); " +
		"process.stdin.on('data', chunk => { input += chunk; " +
		"for (const line of input.split('\\n').slice(0, -1)) { " +
		"const req = JSON.parse(line); process.stdout.write(JSON.stringify({jsonrpc:'2.0',id:req.id,result:{agentInfo:{name:'" + binName + "',version:'" + version + "'}}})+'\\n'); } " +
		"input = input.split('\\n').slice(-1)[0]; });\n"
	var archive bytes.Buffer
	compressor := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(compressor)
	for name, content := range map[string][]byte{"package/package.json": manifest, "package/bin/cli.js": []byte(program)} {
		mode := int64(0o644)
		if strings.HasSuffix(name, ".js") {
			mode = 0o755
		}
		header := &tar.Header{Name: name, Mode: mode, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func assertCapturedCLIArg(t *testing.T, path string, flags []string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read captured provider argv: %v", err)
	}
	var argv []string
	if err := json.Unmarshal(contents, &argv); err != nil {
		t.Fatalf("decode captured provider argv: %v", err)
	}
	for _, flag := range flags {
		count := 0
		for _, arg := range argv {
			if arg == flag {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("captured provider argv = %#v, want %q exactly once", argv, flag)
		}
	}
}

func newScopedManagedPackageRegistry(
	t *testing.T,
	packageName, version, binName string,
	archive []byte,
) (*httptest.Server, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	var requests atomic.Int64
	var downloads atomic.Int64
	sum := sha1.Sum(archive)
	hash := hex.EncodeToString(sum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if strings.HasSuffix(r.URL.Path, ".tgz") {
			downloads.Add(1)
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(archive)
			return
		}
		metadata := map[string]any{
			"name": packageName,
			"versions": map[string]any{
				version: map[string]any{
					"name": packageName, "version": version,
					"bin":  map[string]string{binName: "bin/cli.js"},
					"dist": map[string]string{"tarball": fmt.Sprintf("http://%s/claude-agent-acp-%s.tgz", r.Host, version), "shasum": hash},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metadata)
	}))
	t.Cleanup(server.Close)
	return server, &requests, &downloads
}
