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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPreparedRuntimeLifetime(t *testing.T) {
	packageSpec := "managed-acp@1.2.3"
	registry := "https://registry.example.test"
	prepare := func() *PreparedRuntime {
		t.Helper()
		prepared, err := PreparePrivateRuntime(context.Background(), packageSpec,
			[]string{"NPM_CONFIG_REGISTRY=" + registry}, func(_ context.Context, command ManagedRuntimeCommand) error {
				if command.Dir != filepath.Join(filepath.Dir(command.Dir), "project") {
					t.Fatalf("npm project directory = %q, want private project", command.Dir)
				}
				if !containsArgumentPair(command.Args, "--prefix", command.Dir) {
					t.Fatalf("npm args = %#v, want private project prefix", command.Args)
				}
				cache, ok := argumentValue(command.Args, "--cache")
				if !ok || cache == "" || cache == "shared-cache" {
					t.Fatalf("npm cache argument = %q, present=%v, want private cache", cache, ok)
				}
				if !containsEnvironment(command.Env, "NPM_CONFIG_REGISTRY", registry) {
					t.Fatalf("npm environment = %#v, want configured registry preserved", command.Env)
				}
				return populatePreparedRuntimeCache(command.Args, packageSpec)
			})
		if err != nil {
			t.Fatalf("PreparePrivateRuntime: %v", err)
		}
		t.Cleanup(func() {
			if err := prepared.Close(); err != nil {
				t.Errorf("prepared runtime cleanup: %v", err)
			}
		})
		return prepared
	}
	first, second := prepare(), prepare()
	firstRoot, secondRoot := filepath.Dir(first.ProjectPrefix()), filepath.Dir(second.ProjectPrefix())
	if firstRoot == secondRoot {
		t.Fatalf("prepared runtime roots = %q, want independent owners", firstRoot)
	}
	firstSentinel := filepath.Join(first.CacheDirectory(), "_npx", NpxExecutionCacheKey(packageSpec), "package.json")
	secondSentinel := filepath.Join(second.CacheDirectory(), "_npx", NpxExecutionCacheKey(packageSpec), "package.json")
	if _, err := os.Stat(firstSentinel); err != nil {
		t.Fatalf("first prepared runtime ended before consumer cleanup: %v", err)
	}
	if _, err := os.Stat(secondSentinel); err != nil {
		t.Fatalf("second prepared runtime ended before consumer cleanup: %v", err)
	}

	rewritten, err := first.RewriteNpxCommand([]string{"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, packageSpec, "acp"})
	if err != nil {
		t.Fatalf("RewriteNpxCommand: %v", err)
	}
	if !containsArgumentPair(rewritten[1:], "--prefix", first.ProjectPrefix()) ||
		!containsArgumentPair(rewritten[1:], "--cache", first.CacheDirectory()) || rewritten[len(rewritten)-1] != "acp" {
		t.Fatalf("rewritten npx command = %#v, want private prefix/cache and unchanged ACP args", rewritten)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first prepared runtime: %v", err)
	}
	if _, err := os.Stat(firstRoot); !os.IsNotExist(err) {
		t.Fatalf("first owner root stat = %v, want removed", err)
	}
	if _, err := os.Stat(secondSentinel); err != nil {
		t.Fatalf("first owner cleanup affected sibling: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("repeated first owner cleanup: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second prepared runtime: %v", err)
	}
}

func TestPreparedRuntimeCancellationWaitsForOwnedCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan ManagedRuntimeCommand, 1)
	canceled := make(chan struct{})
	release := make(chan struct{})
	type preparedResult struct {
		prepared *PreparedRuntime
		err      error
	}
	done := make(chan preparedResult, 1)
	go func() {
		prepared, err := PreparePrivateRuntime(ctx, "managed-acp@1.2.3", nil,
			func(ctx context.Context, command ManagedRuntimeCommand) error {
				started <- command
				<-ctx.Done()
				close(canceled)
				<-release
				return ctx.Err()
			})
		done <- preparedResult{prepared: prepared, err: err}
	}()
	command := <-started
	cancel()
	<-canceled
	root := filepath.Dir(command.Dir)
	if _, err := os.Stat(filepath.Join(root, preparedRuntimeOwnerMarker)); err != nil {
		t.Fatalf("private owner marker before child reaped = %v, want present", err)
	}
	select {
	case <-done:
		t.Fatal("preparation returned before its owned installer command was joined")
	default:
	}
	close(release)
	got := <-done
	if !errors.Is(got.err, context.Canceled) || got.prepared != nil {
		t.Fatalf("canceled preparation = (%v, %v), want cancellation and successful cleanup", got.prepared, got.err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("canceled owner root stat = %v, want removed after installer joined", err)
	}
}

func TestPreparedRuntimeNPM(t *testing.T) {
	if os.Getenv("KANDEV_TEST_REAL_NPM") != "1" {
		t.Skip("set KANDEV_TEST_REAL_NPM=1 to run the local npm integration test")
	}
	npm, err := exec.LookPath("npm")
	if err != nil {
		t.Fatalf("npm prerequisite: %v", err)
	}
	packageSpec := "managed-acp@1.2.3"
	archive := testManagedPackageArchive(t, packageSpec)
	registry := newManagedPackageRegistry(t, archive)
	env := npmTestEnvironment(t, registry.URL)

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

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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

	prepared, err := PreparePrivateRuntime(ctx, packageSpec, env, func(ctx context.Context, request ManagedRuntimeCommand) error {
		command := exec.CommandContext(ctx, request.Path, request.Args...)
		command.Dir = request.Dir
		command.Env = request.Env
		output, runErr := command.CombinedOutput()
		if runErr != nil {
			t.Logf("local npm preparation output: %s", output)
		}
		return runErr
	})
	if err != nil {
		t.Fatalf("PreparePrivateRuntime: %v", err)
	}
	t.Cleanup(func() {
		if err := prepared.Close(); err != nil {
			t.Errorf("prepared runtime cleanup: %v", err)
		}
	})

	command, err := prepared.RewriteNpxCommand([]string{
		"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, packageSpec,
	})
	if err != nil {
		t.Fatalf("RewriteNpxCommand: %v", err)
	}
	commandCtx, commandCancel := context.WithTimeout(ctx, 20*time.Second)
	defer commandCancel()
	protocolProcess := exec.CommandContext(commandCtx, command[0], command[1:]...)
	protocolProcess.Dir = prepared.ProjectPrefix()
	protocolProcess.Env = env
	stdin, err := protocolProcess.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := protocolProcess.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := protocolProcess.Start(); err != nil {
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
	if err := protocolProcess.Wait(); err != nil {
		t.Fatalf("wait prepared ACP process: %v", err)
	}
	if !bytes.Contains([]byte(line), []byte(`"name":"managed-acp"`)) || !bytes.Contains([]byte(line), []byte(`"version":"1.2.3"`)) {
		t.Fatalf("initialize response = %q, want exact installed package identity", line)
	}
	if contents, err := os.ReadFile(sharedSentinel); err != nil || string(contents) != "shared sentinel" {
		t.Fatalf("shared sentinel = %q, error = %v, want preserved", contents, err)
	}
	if _, err := os.Stat(filepath.Join(executionRoot, "package.json")); !os.IsNotExist(err) {
		t.Fatalf("incomplete shared execution root manifest stat = %v, want missing", err)
	}
	if prepared.ProjectPrefix() == sharedProject || prepared.CacheDirectory() == sharedCache {
		t.Fatalf("prepared paths = %q and %q, want private project/cache", prepared.ProjectPrefix(), prepared.CacheDirectory())
	}
	if runtime.GOOS == preparedRuntimeWindowsOS && !strings.EqualFold(filepath.Base(command[0]), "npx.cmd") {
		t.Logf("Windows npx command resolved as %q", command[0])
	}
}

func testManagedPackageArchive(t *testing.T, packageSpec string) []byte {
	t.Helper()
	packageName, version, ok := strings.Cut(packageSpec, "@")
	if !ok {
		t.Fatalf("invalid test package spec %q", packageSpec)
	}
	manifest, err := json.Marshal(map[string]any{
		"name": packageName, "version": version, "bin": map[string]string{"managed-acp": "bin/cli.js"},
	})
	if err != nil {
		t.Fatal(err)
	}
	program := "#!/usr/bin/env node\n" +
		"let input = ''; process.stdin.setEncoding('utf8'); " +
		"process.stdin.on('data', chunk => { input += chunk; " +
		"for (const line of input.split('\\n').slice(0, -1)) { " +
		"const req = JSON.parse(line); process.stdout.write(JSON.stringify({jsonrpc:'2.0',id:req.id,result:{agentInfo:{name:'managed-acp',version:'1.2.3'}}})+'\\n'); } " +
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

func newManagedPackageRegistry(t *testing.T, archive []byte) *httptest.Server {
	t.Helper()
	hash := sha1.Sum(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
					"dist": map[string]string{"tarball": serverURL(r) + "/managed-acp-1.2.3.tgz", "shasum": hex.EncodeToString(hash[:])},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metadata)
	}))
	t.Cleanup(server.Close)
	return server
}

func serverURL(request *http.Request) string {
	return fmt.Sprintf("http://%s", request.Host)
}

func npmTestEnvironment(t *testing.T, registryURL string) []string {
	t.Helper()
	values := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	values["NPM_CONFIG_REGISTRY"] = registryURL
	values["npm_config_registry"] = registryURL
	values["NPM_CONFIG_UPDATE_NOTIFIER"] = "false"
	values["NPM_CONFIG_AUDIT"] = "false"
	values["NPM_CONFIG_FUND"] = "false"
	values["NPM_CONFIG_CACHE"] = t.TempDir()
	env := make([]string, 0, len(values))
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}

func populatePreparedRuntimeCache(args []string, packageSpec string) error {
	if _, ok := argumentValue(args, "--prefix"); !ok {
		return errors.New("npm prefix was not supplied")
	}
	cache, ok := argumentValue(args, "--cache")
	if !ok {
		return errors.New("npm cache was not supplied")
	}
	separator := strings.LastIndexByte(packageSpec, '@')
	packageName, version := packageSpec[:separator], packageSpec[separator+1:]
	executionRoot := filepath.Join(cache, "_npx", NpxExecutionCacheKey(packageSpec))
	packageDirectory := filepath.Join(executionRoot, "node_modules", filepath.FromSlash(packageName))
	if err := os.MkdirAll(filepath.Join(packageDirectory, "bin"), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(executionRoot, "package.json"), []byte(`{"private":true}`), 0o600); err != nil {
		return err
	}
	manifest := fmt.Sprintf(`{"name":%q,"version":%q,"bin":{"%s":"bin/cli.js"}}`, packageName, version, npmPackageBase(packageName))
	if err := os.WriteFile(filepath.Join(packageDirectory, "package.json"), []byte(manifest), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(packageDirectory, "bin", "cli.js"), []byte("#!/usr/bin/env node\n"), 0o700); err != nil {
		return err
	}
	binDirectory := filepath.Join(executionRoot, "node_modules", ".bin")
	if err := os.MkdirAll(binDirectory, 0o700); err != nil {
		return err
	}
	if runtime.GOOS == preparedRuntimeWindowsOS {
		shim := `@ECHO off` + "\r\n" + `"%~dp0\..\managed-acp\bin\cli.js" %*` + "\r\n"
		return os.WriteFile(filepath.Join(binDirectory, "managed-acp.cmd"), []byte(shim), 0o600)
	}
	return os.Symlink(filepath.Join("..", "managed-acp", "bin", "cli.js"), filepath.Join(binDirectory, "managed-acp"))
}

func argumentValue(args []string, name string) (string, bool) {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1], true
		}
	}
	return "", false
}

func containsArgumentPair(args []string, name, value string) bool {
	actual, ok := argumentValue(args, name)
	return ok && actual == value
}

func containsEnvironment(env []string, name, value string) bool {
	for _, item := range env {
		key, actual, ok := strings.Cut(item, "=")
		if ok && strings.EqualFold(key, name) && actual == value {
			return true
		}
	}
	return false
}
