package plugins

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/common/sandbox"
	pluginruntime "github.com/kandev/kandev/internal/plugins/runtime"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-SANDBOX-003.4
func TestSandboxProviderExternalPackageRetainsOwnedRuntime(t *testing.T) {
	archivePath := os.Getenv("KANDEV_TEST_SANDBOX_PACKAGE")
	if archivePath == "" {
		t.Skip("set KANDEV_TEST_SANDBOX_PACKAGE to a dedicated provider package")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	svc, dir, _, _ := newTestServiceWithDir(t)
	svc.SetSandboxProvidersEnabled(true)
	svc.SetRuntime(pluginruntime.NewManager(dir, svc.handleStatusChange, testLogger(t)))
	archive, err := os.Open(archivePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, archive.Close()) }()
	record, err := svc.Install(ctx, archive)
	require.NoError(t, err)
	require.Len(t, record.SandboxProviders, 1)
	binding := sandboxFixtureBinding(record, record.SandboxProviders[0].Key)
	req := sandboxFixtureCompileRequest(binding)
	req.Runtime = &pluginsdk.SandboxRuntimeFacts{Os: "linux", Architecture: "amd64", Transport: "local"}
	description, err := svc.DescribeSandboxProvider(ctx, binding, &pluginsdk.DescribeSandboxProviderRequest{Context: req.Context, Runtime: req.Runtime})
	require.NoError(t, err)
	require.NotNil(t, description.Description.RuntimeArtifact)
	runtime, err := svc.ReadSandboxRuntime(ctx, binding, description.Description.RuntimeArtifact)
	require.NoError(t, err)
	req.Runtime.RuntimeVersion = description.Description.RuntimeVersion
	req.Runtime.RuntimeDigest = runtime.Digest
	work := t.TempDir()
	child := buildExternalSandboxChild(t, ctx)
	req.Policy.Grants.Filesystem = []*pluginsdk.SandboxFilesystemGrant{{Path: work, Writable: true}, {Path: child}}
	compiled, err := svc.CompileSandboxPolicy(ctx, binding, req)
	require.NoError(t, err)
	files := materializeExternalSandbox(t, ctx, runtime, compiled.Compiled, description.Description.ApprovalWire)
	defer func() { require.NoError(t, files.Close()) }()
	command := externalSandboxCommand(t, ctx, files, compiled.Compiled, child, work)
	stdin, err := command.StdinPipe()
	require.NoError(t, err)
	stdout, err := command.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, command.Start())
	defer func() {
		if command.ProcessState == nil {
			_ = command.Cancel()
			_ = command.Wait()
		}
	}()
	reader := bufio.NewReader(stdout)
	ready, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", ready)
	verifyExternalSandboxIdentity(t, ctx, svc, binding, req, description.Description, files, command.Process.Pid)
	require.NoError(t, svc.Uninstall(ctx, record.ID))
	_, err = svc.ReadSandboxRuntime(ctx, binding, description.Description.RuntimeArtifact)
	require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
	require.FileExists(t, files.Executable)
	_, err = io.WriteString(stdin, "retained-process\n")
	require.NoError(t, err)
	require.NoError(t, stdin.Close())
	response, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "retained-process\n", string(response))
	require.NoError(t, command.Wait())
	require.FileExists(t, filepath.Join(work, "permitted.txt"))
}

func buildExternalSandboxChild(t *testing.T, ctx context.Context) string {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "child.go")
	binary := filepath.Join(root, "child")
	require.NoError(t, os.WriteFile(source, []byte(`package main
import("fmt";"io";"os")
func main(){fmt.Println("ready");data,err:=io.ReadAll(os.Stdin);if err!=nil{os.Exit(2)};if os.WriteFile("permitted.txt",data,0600)!=nil{os.Exit(3)};_,_=os.Stdout.Write(data)}`), 0600))
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, source)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	return binary
}

func materializeExternalSandbox(t *testing.T, ctx context.Context, runtime *SandboxRuntimeBinary, compiled *pluginsdk.SandboxCompiledPolicy, wire *pluginsdk.SandboxApprovalWire) *sandbox.RuntimeFiles {
	t.Helper()
	require.NotNil(t, wire)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", wire.ContentType)
		_, _ = io.WriteString(w, wire.DenyBodyUtf8)
	}))
	t.Cleanup(callback.Close)
	artifacts := make([]sandbox.RuntimeArtifact, 0, len(compiled.Artifacts))
	for _, artifact := range compiled.Artifacts {
		value := sandbox.RuntimeArtifact{Name: artifact.Name, Content: artifact.ContentUtf8}
		for _, binding := range artifact.Bindings {
			value.Bindings = append(value.Bindings, sandbox.ArtifactBinding{Path: binding.Path, Reference: binding.RuntimeReference})
		}
		artifacts = append(artifacts, value)
	}
	files, err := sandbox.MaterializeRuntime(ctx, t.TempDir(), runtime.Content, runtime.Digest, artifacts, map[string]string{sandboxApprovalEndpoint: callback.URL})
	require.NoError(t, err)
	return files
}

func externalSandboxCommand(t *testing.T, ctx context.Context, files *sandbox.RuntimeFiles, compiled *pluginsdk.SandboxCompiledPolicy, child, work string) *exec.Cmd {
	t.Helper()
	args := []string{files.Executable}
	for _, argument := range compiled.Prefix[1:] {
		switch value := argument.Value.(type) {
		case *pluginsdk.SandboxArgument_Literal:
			args = append(args, value.Literal)
		case *pluginsdk.SandboxArgument_ArtifactReference:
			args = append(args, files.Artifacts[value.ArtifactReference])
		default:
			t.Fatal("unexpected runtime reference")
		}
	}
	args = append(args, child)
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Dir = work
	var err error
	command.Env, err = sandbox.IsolatedEnvironment(map[string]string{"PATH": strings.Join([]string{"/usr/bin", "/bin"}, ":")}, nil, files.Directories)
	require.NoError(t, err)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	command.WaitDelay = time.Second
	return command
}

func verifyExternalSandboxIdentity(t *testing.T, ctx context.Context, svc *Service, binding SandboxProviderBinding, request *pluginsdk.CompileSandboxPolicyRequest, description *pluginsdk.SandboxProviderDescription, files *sandbox.RuntimeFiles, pid int) {
	t.Helper()
	var declaration *pluginsdk.SandboxRuntimeSource
	for _, source := range description.RuntimeSources {
		if source.Kind == "identity" {
			declaration = source
			break
		}
	}
	require.NotNil(t, declaration)
	source := sandbox.RuntimeSource{Key: declaration.Key, Kind: declaration.Kind, Directory: declaration.RelativeDirectory, Suffix: declaration.FilenameSuffix, MaxDepth: declaration.MaxDepth, MaxFiles: declaration.MaxFiles, MaxBytes: declaration.MaxBytes}
	var records []sandbox.RuntimeRecord
	require.Eventually(t, func() bool {
		var err error
		records, err = sandbox.ReadRuntimeSource(ctx, files.Directories.State, source)
		return err == nil && len(records) == 1
	}, 3*time.Second, 10*time.Millisecond)
	identity, err := processidentity.Capture(pid)
	require.NoError(t, err)
	response, err := svc.NormalizeSandboxStatus(ctx, binding, &pluginsdk.NormalizeSandboxStatusRequest{Context: request.Context, Policy: request.Policy, Process: &pluginsdk.SandboxProcessIdentity{Pid: int64(pid), BirthId: identity.BirthToken}, Evidence: records[0].Content, EvidenceSchema: source.Key})
	require.NoError(t, err)
	require.Nil(t, response.Evidence)
	require.Equal(t, int64(pid), response.Identity.SupervisorPid)
	child, err := processidentity.Capture(int(response.Identity.ChildPid))
	require.NoError(t, err)
	require.NotEmpty(t, child.BirthToken)
	owned, err := processidentity.IsDescendant(ctx, identity, child)
	require.NoError(t, err)
	require.True(t, owned)
}
