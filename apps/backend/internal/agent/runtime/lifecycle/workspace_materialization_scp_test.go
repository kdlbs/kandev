package lifecycle

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/api"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/stretchr/testify/require"
)

func TestRemoteWorkspaceSCPLocators(t *testing.T) {
	t.Run("materializes a secondary repository from a conventional SCP locator", func(t *testing.T) {
		environment := newSCPMaterializationEnvironment(t)
		manager, server := environment.manager(t)

		_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{
			RepositoryURL: environment.secondaryLocator,
			Destination:   "secondary-main",
			BaseBranch:    "main",
		}})
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(server.workspace, "secondary-main", "README.md"))
		require.Equal(t, environment.secondaryLocator, gitRemoteOrigin(t, filepath.Join(server.workspace, "secondary-main")))
	})

	t.Run("rolls back only new checkouts after a later preparation failure", func(t *testing.T) {
		environment := newSCPMaterializationEnvironment(t)
		manager, server := environment.manager(t)

		_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{
			RepositoryURL: environment.secondaryLocator,
			Destination:   "reused-main",
			BaseBranch:    "main",
		}})
		require.NoError(t, err)

		_, err = manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{
			{RepositoryURL: environment.secondaryLocator, Destination: "reused-main", BaseBranch: "main"},
			{RepositoryURL: environment.thirdLocator, Destination: "third-main", BaseBranch: "main"},
			{RepositoryURL: environment.failingLocator, Destination: "failing-main", BaseBranch: "main"},
		})
		require.Error(t, err)
		require.FileExists(t, filepath.Join(server.workspace, "reused-main", "README.md"))
		require.NoDirExists(t, filepath.Join(server.workspace, "third-main"))
		require.NoDirExists(t, filepath.Join(server.workspace, "failing-main"))
	})

	t.Run("single repository preparation remains supported", func(t *testing.T) {
		environment := newSCPMaterializationEnvironment(t)
		manager, server := environment.manager(t)

		_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{
			RepositoryURL: environment.secondaryLocator,
			Destination:   "sole-main",
			BaseBranch:    "main",
		}})
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(server.workspace, "sole-main", "README.md"))
	})
}

type scpMaterializationEnvironment struct {
	t                *testing.T
	workspace        string
	gitConfigPath    string
	secondaryLocator string
	thirdLocator     string
	failingLocator   string
}

type scpMaterializationServer struct {
	workspace string
}

func newSCPMaterializationEnvironment(t *testing.T) *scpMaterializationEnvironment {
	t.Helper()
	root := t.TempDir()
	secondary := createSCPTestOrigin(t, root, "secondary")
	third := createSCPTestOrigin(t, root, "third")
	environment := &scpMaterializationEnvironment{
		t:                t,
		workspace:        t.TempDir(),
		gitConfigPath:    filepath.Join(root, "gitconfig"),
		secondaryLocator: "git@scp.test:acme/secondary.git",
		thirdLocator:     "git@scp.test:acme/third.git",
		failingLocator:   "git@scp.test:acme/failing.git",
	}
	gitConfig := fmt.Sprintf(
		"[url \"file://%s\"]\n\tinsteadOf = %s\n[url \"file://%s\"]\n\tinsteadOf = %s\n[url \"file://%s\"]\n\tinsteadOf = %s\n",
		filepath.ToSlash(secondary), environment.secondaryLocator,
		filepath.ToSlash(third), environment.thirdLocator,
		filepath.ToSlash(filepath.Join(root, "missing.git")), environment.failingLocator,
	)
	require.NoError(t, os.WriteFile(environment.gitConfigPath, []byte(gitConfig), 0o600))
	return environment
}

func (environment *scpMaterializationEnvironment) manager(t *testing.T) (*Manager, *scpMaterializationServer) {
	t.Helper()
	agentEnvironment := append([]string(nil), os.Environ()...)
	agentEnvironment = append(agentEnvironment, "GIT_CONFIG_GLOBAL="+environment.gitConfigPath, "GIT_CONFIG_NOSYSTEM=1")
	cfg := &config.InstanceConfig{WorkDir: environment.workspace, AgentEnv: agentEnvironment}
	manager := process.NewManager(cfg, newTestLogger())
	t.Cleanup(func() { require.NoError(t, manager.StopForTeardown(context.Background())) })
	server := httptest.NewServer(api.NewServer(cfg, manager, nil, nil, newTestLogger()).Router())
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	client := agentctl.NewClient(host, port, newTestLogger())
	store := NewExecutionStore()
	require.NoError(t, store.Add(&AgentExecution{
		ID: "execution-1", SessionID: "session-1", TaskEnvironmentID: "environment-1", agentctl: client,
	}))
	return &Manager{executionStore: store, logger: newTestLogger()}, &scpMaterializationServer{workspace: environment.workspace}
}

func createSCPTestOrigin(t *testing.T, root, name string) string {
	t.Helper()
	origin := filepath.Join(root, name+".git")
	seed := filepath.Join(root, name+"-seed")
	runSCPTestGit(t, root, "init", "--bare", "--initial-branch=main", origin)
	runSCPTestGit(t, root, "init", "--initial-branch=main", seed)
	require.NoError(t, os.WriteFile(filepath.Join(seed, "README.md"), []byte(name+"\n"), 0o644))
	runSCPTestGit(t, seed, "add", "README.md")
	runSCPTestGit(t, seed, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "initial")
	runSCPTestGit(t, seed, "remote", "add", "origin", origin)
	runSCPTestGit(t, seed, "push", "origin", "main")
	return origin
}

func runSCPTestGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func gitRemoteOrigin(t *testing.T, directory string) string {
	t.Helper()
	command := exec.Command("git", "remote", "get-url", "origin")
	command.Dir = directory
	output, err := command.CombinedOutput()
	require.NoError(t, err)
	return strings.TrimSpace(string(output))
}
