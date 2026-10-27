//go:build linux

package sandboxruntime

import (
	"bufio"
	"context"
	"github.com/kandev/kandev/internal/common/processidentity"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins"
	pluginruntime "github.com/kandev/kandev/internal/plugins/runtime"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/stretchr/testify/require"
)

func TestSandboxRuntimePreparesAndBindsPackagedProvider(t *testing.T) {
	packagePath := os.Getenv("KANDEV_TEST_SANDBOX_PACKAGE")
	if packagePath == "" {
		t.Skip("set KANDEV_TEST_SANDBOX_PACKAGE to a dedicated provider package")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	pluginService, policies, selection := runtimeFixture(t, ctx, packagePath)
	work := t.TempDir()
	child := buildRuntimeChild(t, ctx)
	providers, err := pluginService.ListSandboxProviders(ctx)
	require.NoError(t, err)
	require.Len(t, providers, 1)
	binding := providers[0].Binding
	revision, err := policies.Publish(ctx, policy.Policy{ID: "work", Name: "Work", InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Path: work, Writable: true}, {Path: child}}}}, 0)
	require.NoError(t, err)
	_, err = policies.PutBinding(ctx, policy.Binding{Scope: policy.ScopeExecutor, TargetID: selection.ExecutorID, Executor: &policy.ExecutorPolicy{Mode: policy.ModeRequired, InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, CeilingRevisionID: revision.ID, DefaultRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}}}, 0)
	require.NoError(t, err)
	manager := NewManager(policies, pluginService, t.TempDir())
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	prepared, err := manager.Prepare(ctx, PrepareRequest{Selection: selection, SessionID: "session", ExecutionID: "execution", Generation: 1, WorkDir: work, Arguments: []string{child}})
	require.NoError(t, err)
	require.NotNil(t, prepared)
	args, env, err := contract.PrepareLaunch(prepared.Launch(), []string{child}, nil)
	require.NoError(t, err)
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	command.Env = env
	command.Dir = work
	input, err := command.StdinPipe()
	require.NoError(t, err)
	output, err := command.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, prepared.MarkStart(ctx))
	containmentLease, err := prepared.Launch().Containment.Configure(command)
	require.NoError(t, err)
	require.NoError(t, command.Start())
	require.NoError(t, containmentLease.Close())
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	owner, err := contract.CaptureProcessOwner(prepared.Launch(), command.Process.Pid)
	require.NoError(t, err)
	ready, err := bufio.NewReader(output).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", ready)
	detachedPID, err := os.ReadFile(filepath.Join(work, "detached.pid"))
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(detachedPID))
	require.NoError(t, err)
	detached, err := processidentity.Capture(pid)
	require.NoError(t, err)
	t.Cleanup(func() {
		if state, _ := processidentity.Inspect(detached); state == processidentity.StateAlive {
			process, _ := os.FindProcess(pid)
			_ = process.Kill()
		}
	})
	sources := prepared.description.RuntimeSources
	prepared.description.RuntimeSources = nil
	require.Error(t, prepared.BindOwner(ctx, "agentctl-runtime", owner))
	unbound, err := policies.ReadAttempt(ctx, "org", prepared.Launch().AttemptID)
	require.NoError(t, err)
	require.NotNil(t, unbound.Owner, "verified supervisor ownership must survive provider evidence failure")
	require.Equal(t, owner.Supervisor.BirthToken, unbound.Owner.SupervisorBirth)
	require.Equal(t, policy.AttemptStarting, unbound.State)
	prepared.description.RuntimeSources = sources
	require.NoError(t, prepared.BindOwner(ctx, "agentctl-runtime", owner))
	attempt, err := policies.ReadAttempt(ctx, "org", prepared.Launch().AttemptID)
	require.NoError(t, err)
	require.Equal(t, policy.AttemptRunning, attempt.State)
	require.NotEmpty(t, attempt.Owner.ChildBirth)
	require.NoError(t, manager.VerifyLive(ctx, attempt))
	observed, err := manager.Observe(ctx, "org", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, "unknown", observed.Status, "registry identity cannot prove confinement")
	require.Equal(t, "available", observed.ProviderHealth)
	require.Equal(t, "evidence_unavailable", observed.Reason)
	require.Nil(t, observed.EvidenceAt)
	require.Nil(t, observed.ValidUntil)
	require.Equal(t, attempt.ID, observed.AttemptID)
	require.Equal(t, attempt.Snapshot.Launch.Generation, observed.Generation)
	require.NoError(t, pluginService.Disable(binding.PluginID))
	require.Error(t, manager.VerifyLive(ctx, attempt), "provider removal blocks new grants")
	observed, err = manager.Observe(ctx, "org", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, "unknown", observed.Status)
	require.Equal(t, "unavailable", observed.ProviderHealth)
	restartedManager := NewManager(policies, pluginService, manager.base)
	t.Cleanup(func() { require.NoError(t, restartedManager.Close()) })
	_, err = restartedManager.ReconcileAttempt(ctx, "org", prepared.Launch().AttemptID)
	require.Error(t, err, "a live supervisor cannot be treated as a stopped attempt")
	require.Error(t, prepared.Close(), "live processes prevent artifact removal")
	recovery, err := policies.ReadAttempt(ctx, "org", prepared.Launch().AttemptID)
	require.NoError(t, err)
	require.Equal(t, policy.AttemptRecoveryRequired, recovery.State)
	require.DirExists(t, prepared.Launch().Root)
	_, err = io.WriteString(input, "finish\n")
	require.NoError(t, err)
	require.NoError(t, input.Close())
	require.NoError(t, command.Wait())
	require.Error(t, prepared.BindOwner(ctx, "agentctl-runtime", owner), "exited ownership cannot be rebound")
	reconciled, err := restartedManager.ReconcileAttempt(ctx, "org", prepared.Launch().AttemptID)
	require.NoError(t, err)
	require.Equal(t, policy.AttemptStopped, reconciled.State)
	observed, err = restartedManager.Observe(ctx, "org", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, "stopped", observed.Status)
	require.Nil(t, observed.ValidUntil)
	detachedState, err := processidentity.Inspect(detached)
	require.NoError(t, err)
	require.Equal(t, processidentity.StateExited, detachedState, "recovery must end descendants outside the supervisor session")
	require.NoError(t, prepared.Close())
	_, err = os.Stat(prepared.Launch().Root)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func runtimeFixture(t *testing.T, ctx context.Context, packagePath string) (*plugins.Service, *policy.Service, policy.SelectionContext) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	root := t.TempDir()
	service := plugins.NewService(store.NewFSStore(root), plugins.NewRegistry(), nil, log)
	require.NoError(t, service.SetPluginsDir(root))
	service.SetSandboxProvidersEnabled(true)
	service.SetRuntime(pluginruntime.NewManager(root, nil, log))
	t.Cleanup(func() { _ = service.Close() })
	archive, err := os.Open(packagePath)
	require.NoError(t, err)
	_, err = service.Install(ctx, archive)
	require.NoError(t, archive.Close())
	require.NoError(t, err)
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "policies.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(raw, "sqlite3")
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	policies, err := policy.NewService(database, true, nil)
	require.NoError(t, err)
	policies.SetTargetResolver(func(context.Context, string, string) (policy.TargetScope, error) {
		return policy.TargetScope{OrgID: "org"}, nil
	})
	return service, policies, policy.SelectionContext{OrgID: "org", ExecutorID: "local"}
}

func buildRuntimeChild(t *testing.T, ctx context.Context) string {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "child.go")
	binary := filepath.Join(root, "child")
	require.NoError(t, os.WriteFile(source, []byte(`package main
import("fmt";"bufio";"os";"os/exec";"syscall";"time";"strconv")
func main(){if len(os.Args)>1 {time.Sleep(30*time.Second);return};child:=exec.Command(os.Args[0],"detached");child.Stdin=os.Stdin;child.Stdout=os.Stdout;child.Stderr=os.Stderr;child.SysProcAttr=&syscall.SysProcAttr{Setsid:true};if child.Start()!=nil{os.Exit(4)};if os.WriteFile("detached.pid",[]byte(strconv.Itoa(child.Process.Pid)),0600)!=nil{os.Exit(5)};fmt.Println("ready");_,err:=bufio.NewReader(os.Stdin).ReadString('\n');if err!=nil{os.Exit(2)};if os.WriteFile("permitted.txt",[]byte("ok"),0600)!=nil{os.Exit(3)}}`), 0600))
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, source)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	return binary
}
