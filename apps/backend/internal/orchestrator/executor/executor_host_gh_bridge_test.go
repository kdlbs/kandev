package executor

import (
	"context"
	"errors"
	"os"
	osExec "os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestExecutorHostGHBridgeCredentialFill(t *testing.T) {
	ghPath := filepath.Join(t.TempDir(), "host tools", "gh")
	if err := os.MkdirAll(filepath.Dir(ghPath), 0o700); err != nil {
		t.Fatalf("create fake gh directory: %v", err)
	}
	const ghScript = `#!/bin/sh
if [ "$1" = "auth" ] && [ "$2" = "token" ]; then
  exit 0
fi
if [ "$1" = "auth" ] && [ "$2" = "git-credential" ]; then
  cat >/dev/null
  printf 'username=x-access-token\npassword=host-token\n'
  exit 0
fi
exit 2
`
	if err := os.WriteFile(ghPath, []byte(ghScript), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(ghPath)+string(os.PathListSeparator)+os.Getenv("PATH"))

	executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})
	req := &LaunchAgentRequest{
		TaskID:       "task-1",
		WorkspaceID:  "workspace-1",
		SessionID:    "session-1",
		ExecutorType: string(models.ExecutorTypeWorktree),
		Env: map[string]string{
			"GIT_CONFIG_COUNT":   "2",
			"GIT_CONFIG_KEY_0":   "notes.augment.mergeStrategy",
			"GIT_CONFIG_VALUE_0": "union",
			"GIT_CONFIG_KEY_1":   "core.hooksPath",
			"GIT_CONFIG_VALUE_1": "/Users/cfl12/.locstat/git/hooks",
		},
	}
	info := &repoInfo{
		RepositoryID: "repo-1",
		Repository: &models.Repository{
			Provider:      "github",
			ProviderOwner: "acme",
			ProviderName:  "widgets",
			RemoteURL:     "https://github.com/acme/widgets.git",
		},
	}

	if err := executor.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{info}); err != nil {
		t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
	}
	if got := req.Env["GIT_CONFIG_COUNT"]; got != "3" {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want 3", got)
	}
	if got := req.Env["GIT_CONFIG_KEY_0"]; got != "notes.augment.mergeStrategy" {
		t.Fatalf("GIT_CONFIG_KEY_0 = %q, want inherited notes entry", got)
	}
	if got := req.Env["GIT_CONFIG_VALUE_0"]; got != "union" {
		t.Fatalf("GIT_CONFIG_VALUE_0 = %q, want inherited notes value", got)
	}
	if got := req.Env["GIT_CONFIG_KEY_1"]; got != "core.hooksPath" {
		t.Fatalf("GIT_CONFIG_KEY_1 = %q, want inherited hooks entry", got)
	}
	if got := req.Env["GIT_CONFIG_VALUE_1"]; got != "/Users/cfl12/.locstat/git/hooks" {
		t.Fatalf("GIT_CONFIG_VALUE_1 = %q, want inherited hooks value", got)
	}

	env := isolatedGitEnvironment(t, req.Env)
	command := osExec.Command("git", "credential", "fill")
	command.Env = env
	command.Stdin = strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets\n\n")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git credential fill failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "username=x-access-token") ||
		!strings.Contains(string(output), "password=host-token") {
		t.Fatalf("credential output = %q, want fake host helper credentials", output)
	}
}

func TestExecutorHostGHBridgePreservesIndexedConfig(t *testing.T) {
	setupHostGHExecutable(t)

	tests := []struct {
		name          string
		env           map[string]string
		wantCount     string
		wantErrorText string
		wantEntries   []gitConfigTestEntry
	}{
		{
			name:      "absent block",
			wantCount: "1",
			wantEntries: []gitConfigTestEntry{
				{key: "credential.https://github.com.helper"},
			},
		},
		{
			name: "empty block",
			env: map[string]string{
				"GIT_CONFIG_COUNT": "0",
			},
			wantCount: "1",
			wantEntries: []gitConfigTestEntry{
				{key: "credential.https://github.com.helper"},
			},
		},
		{
			name: "meaningful repeated helpers",
			env: indexedGitConfigTestEnvironment(
				gitConfigTestEntry{key: "credential.https://github.com.helper", value: "!first-helper"},
				gitConfigTestEntry{key: "credential.https://github.com.helper", value: "!second-helper"},
			),
			wantCount: "3",
			wantEntries: []gitConfigTestEntry{
				{key: "credential.https://github.com.helper", value: "!first-helper"},
				{key: "credential.https://github.com.helper", value: "!second-helper"},
				{key: "credential.https://github.com.helper"},
			},
		},
		{
			name: "malformed block",
			env: map[string]string{
				"GIT_CONFIG_COUNT": "1",
				"GIT_CONFIG_KEY_0": "core.hooksPath",
			},
			wantErrorText: "entry 0",
		},
		{
			name: "combined count limit",
			env: indexedGitConfigTestEnvironment(func() []gitConfigTestEntry {
				entries := make([]gitConfigTestEntry, 0, 256)
				for index := 0; index < 256; index++ {
					entries = append(entries, gitConfigTestEntry{key: "test.key", value: "value"})
				}
				return entries
			}()...),
			wantErrorText: "maximum is 256",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
			executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
				policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
			})
			executor.SetHostGitHubCredentialProbe(func(context.Context, string, string, map[string]string) error {
				return nil
			})
			req := executorHostGHBridgeRequest(test.env)
			info := executorHostGHBridgeRepository("repo-1", "github.com")

			err := executor.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{info})
			if test.wantErrorText != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErrorText) {
					t.Fatalf("error = %v, want %q", err, test.wantErrorText)
				}
				return
			}
			if err != nil {
				t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
			}
			if got := req.Env["GIT_CONFIG_COUNT"]; got != test.wantCount {
				t.Fatalf("GIT_CONFIG_COUNT = %q, want %q", got, test.wantCount)
			}
			for index, want := range test.wantEntries {
				if got := req.Env["GIT_CONFIG_KEY_"+strconv.Itoa(index)]; got != want.key {
					t.Errorf("GIT_CONFIG_KEY_%d = %q, want %q", index, got, want.key)
				}
				gotValue := req.Env["GIT_CONFIG_VALUE_"+strconv.Itoa(index)]
				if index == len(test.wantEntries)-1 && isHostGitHubCredentialHelper(gotValue) {
					continue
				}
				if gotValue != want.value {
					t.Errorf("GIT_CONFIG_VALUE_%d = %q, want %q", index, gotValue, want.value)
				}
			}
		})
	}
}

func TestExecutorHostGHBridgeCredentialPrecedence(t *testing.T) {
	setupHostGHExecutable(t)

	tests := []struct {
		name              string
		policy            TaskGitCredentialPolicy
		env               map[string]string
		repository        *models.Repository
		brokerError       error
		wantProbeCalls    int
		wantHelper        bool
		wantBrokerCalls   int
		wantExistingEntry string
	}{
		{
			name:           "request public token",
			policy:         TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
			env:            map[string]string{envGHToken: "request-token"},
			wantProbeCalls: 0,
		},
		{
			name:       "request enterprise token",
			policy:     TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
			env:        map[string]string{"GH_ENTERPRISE_TOKEN": "enterprise-token"},
			repository: &models.Repository{Provider: gitHubProviderID, ProviderHost: "https://ghe.example"},
		},
		{
			name:              "existing user helper remains earlier",
			policy:            TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
			env:               indexedGitConfigTestEnvironment(gitConfigTestEntry{key: "credential.https://github.com.helper", value: "!user-helper"}),
			wantProbeCalls:    1,
			wantHelper:        true,
			wantExistingEntry: "!user-helper",
		},
		{
			name:            "managed broker failure has no host fallback",
			policy:          TaskGitCredentialPolicy{Mode: taskGitCredentialsModeManaged},
			brokerError:     errors.New("broker unavailable"),
			wantProbeCalls:  0,
			wantBrokerCalls: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
			issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}, err: test.brokerError}
			executor.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/github/credentials/resolve")
			executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{policy: test.policy})
			probeCalls := 0
			executor.SetHostGitHubCredentialProbe(func(context.Context, string, string, map[string]string) error {
				probeCalls++
				return nil
			})
			repository := test.repository
			if repository == nil {
				repository = &models.Repository{
					Provider:      gitHubProviderID,
					ProviderHost:  "https://github.com",
					ProviderOwner: "acme",
					ProviderName:  "widgets",
					RemoteURL:     "https://github.com/acme/widgets.git",
				}
			}
			req := executorHostGHBridgeRequest(test.env)
			err := executor.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{{RepositoryID: "repo-1", Repository: repository}})
			if test.brokerError != nil {
				if err == nil || !strings.Contains(err.Error(), "broker unavailable") {
					t.Fatalf("error = %v, want broker failure", err)
				}
			} else if err != nil {
				t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
			}
			if probeCalls != test.wantProbeCalls {
				t.Fatalf("probe calls = %d, want %d", probeCalls, test.wantProbeCalls)
			}
			if issuer.calls != test.wantBrokerCalls {
				t.Fatalf("broker calls = %d, want %d", issuer.calls, test.wantBrokerCalls)
			}
			if hasHostGitHubHelper(req.Env) != test.wantHelper {
				t.Fatalf("host helper present = %v, want %v; env=%#v", hasHostGitHubHelper(req.Env), test.wantHelper, req.Env)
			}
			if test.wantExistingEntry != "" && req.Env["GIT_CONFIG_VALUE_0"] != test.wantExistingEntry {
				t.Fatalf("existing helper = %q, want %q", req.Env["GIT_CONFIG_VALUE_0"], test.wantExistingEntry)
			}
		})
	}
}

func TestExecutorHostGHBridgeEligibility(t *testing.T) {
	t.Run("local and worktree only", func(t *testing.T) {
		setupHostGHExecutable(t)
		for _, executorType := range []models.ExecutorType{models.ExecutorTypeLocal, models.ExecutorTypeWorktree} {
			t.Run(string(executorType), func(t *testing.T) {
				executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
				executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor}})
				calls := 0
				executor.SetHostGitHubCredentialProbe(func(context.Context, string, string, map[string]string) error {
					calls++
					return nil
				})
				req := executorHostGHBridgeRequest(nil)
				req.ExecutorType = string(executorType)
				if err := executor.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{{RepositoryID: "repo-1", Repository: &models.Repository{Provider: gitHubProviderID, RemoteURL: "https://github.com/acme/widgets.git"}}}); err != nil {
					t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
				}
				if calls != 1 {
					t.Fatalf("probe calls = %d, want 1", calls)
				}
			})
		}
	})

	for _, executorType := range []models.ExecutorType{
		models.ExecutorTypeLocalDocker,
		models.ExecutorTypeRemoteDocker,
		models.ExecutorTypeSprites,
		models.ExecutorTypeSSH,
		models.ExecutorTypeKubernetes,
		models.ExecutorTypeMockRemote,
		"unknown",
	} {
		t.Run(string(executorType), func(t *testing.T) {
			executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
			executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor}})
			calls := 0
			executor.SetHostGitHubCredentialProbe(func(context.Context, string, string, map[string]string) error {
				calls++
				return nil
			})
			req := executorHostGHBridgeRequest(nil)
			req.ExecutorType = string(executorType)
			if err := executor.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{{RepositoryID: "repo-1", Repository: &models.Repository{Provider: gitHubProviderID, RemoteURL: "https://github.com/acme/widgets.git"}}}); err != nil {
				t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
			}
			if calls != 0 {
				t.Fatalf("probe calls = %d, want 0", calls)
			}
		})
	}

	t.Run("mixed repositories deduplicate trusted hosts", func(t *testing.T) {
		setupHostGHExecutable(t)
		executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
		executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor}})
		var hosts []string
		executor.SetHostGitHubCredentialProbe(func(_ context.Context, _ string, host string, _ map[string]string) error {
			hosts = append(hosts, host)
			return nil
		})
		req := executorHostGHBridgeRequest(nil)
		infos := []*repoInfo{
			{RepositoryID: "public-1", Repository: &models.Repository{Provider: gitHubProviderID, RemoteURL: "https://github.com/acme/one.git"}},
			{RepositoryID: "public-2", Repository: &models.Repository{Provider: gitHubProviderID, RemoteURL: "git@github.com:acme/two.git"}},
			{RepositoryID: "enterprise", Repository: &models.Repository{Provider: gitHubProviderID, ProviderHost: "https://ghe.example", RemoteURL: "https://ghe.example/acme/three.git"}},
			{RepositoryID: "gitlab", Repository: &models.Repository{Provider: gitLabProviderID, RemoteURL: "https://gitlab.example/acme/four.git"}},
			{RepositoryID: "local-ambiguous", Repository: &models.Repository{SourceType: sourceTypeLocal, RemoteURL: "https://github.com/acme/five.git"}},
			{RepositoryID: "invalid", Repository: &models.Repository{Provider: gitHubProviderID, ProviderOwner: "acme", ProviderName: "six", RemoteURL: "https://github.com/other/six.git"}},
		}
		if err := executor.configureGitCredentialBrokerForRepositories(context.Background(), req, infos); err != nil {
			t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
		}
		if strings.Join(hosts, ",") != "github.com,ghe.example" {
			t.Fatalf("probed hosts = %v, want [github.com ghe.example]", hosts)
		}
	})

	t.Run("unavailable CLI skips bridge", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
		executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor}})
		calls := 0
		executor.SetHostGitHubCredentialProbe(func(context.Context, string, string, map[string]string) error {
			calls++
			return nil
		})
		req := executorHostGHBridgeRequest(nil)
		if err := executor.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{{RepositoryID: "repo-1", Repository: &models.Repository{Provider: gitHubProviderID, RemoteURL: "https://github.com/acme/widgets.git"}}}); err != nil {
			t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
		}
		if calls != 0 || hasHostGitHubHelper(req.Env) {
			t.Fatalf("unavailable CLI still activated bridge: calls=%d env=%#v", calls, req.Env)
		}
	})
}

func TestExecutorHostGHBridgeProbeTimeoutAndCancellation(t *testing.T) {
	setupHostGHExecutable(t)
	baseInfo := []*repoInfo{executorHostGHBridgeRepository("repo-1", "github.com")}

	t.Run("probe timeout is optional", func(t *testing.T) {
		executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
		executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor}})
		executor.SetHostGitHubCredentialProbe(func(ctx context.Context, _, _ string, _ map[string]string) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("probe context has no deadline")
			}
			return context.DeadlineExceeded
		})
		req := executorHostGHBridgeRequest(nil)
		if err := executor.configureGitCredentialBrokerForRepositories(context.Background(), req, baseInfo); err != nil {
			t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
		}
		if hasHostGitHubHelper(req.Env) {
			t.Fatalf("timed out probe activated bridge: %#v", req.Env)
		}
	})

	t.Run("caller cancellation is returned", func(t *testing.T) {
		executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
		executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor}})
		ctx, cancel := context.WithCancel(context.Background())
		executor.SetHostGitHubCredentialProbe(func(_ context.Context, _, _ string, _ map[string]string) error {
			cancel()
			return context.Canceled
		})
		err := executor.configureGitCredentialBrokerForRepositories(ctx, executorHostGHBridgeRequest(nil), baseInfo)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context cancellation", err)
		}
	})
}

func TestExecutorHostGHBridgePreparedWorkspace(t *testing.T) {
	var captured map[string]string
	var gotSessionID, gotEnvironmentID string
	manager := &mockAgentManager{
		executorProfileEnvFunc: func(_ context.Context, sessionID, taskEnvironmentID string) (map[string]string, error) {
			gotSessionID = sessionID
			gotEnvironmentID = taskEnvironmentID
			return map[string]string{envGHToken: "profile-token"}, nil
		},
		setExecutionEnvFunc: func(_ context.Context, _ string, env map[string]string) error {
			captured = env
			return nil
		},
	}
	executor := newTestExecutor(t, manager, newMockRepository())
	request := &LaunchAgentRequest{
		TaskID:            "task-1",
		WorkspaceID:       "workspace-1",
		SessionID:         "session-1",
		TaskEnvironmentID: "environment-1",
		ExecutorType:      string(models.ExecutorTypeWorktree),
		Env: indexedGitConfigTestEnvironment(
			gitConfigTestEntry{key: "notes.augment.mergeStrategy", value: "union"},
			gitConfigTestEntry{key: "credential.https://github.com.helper", value: hostGitHubCredentialHelper("/tmp/host tools/gh")},
		),
	}
	session := &models.TaskSession{ID: "session-1", TaskID: "task-1"}

	err := executor.configureExistingWorkspace(
		context.Background(),
		&v1.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		session,
		"execution-1",
		"",
		request,
	)
	if err != nil {
		t.Fatalf("configureExistingWorkspace() error = %v", err)
	}
	if gotSessionID != session.ID || gotEnvironmentID != request.TaskEnvironmentID {
		t.Fatalf("profile environment lookup = (%q, %q), want (%q, %q)", gotSessionID, gotEnvironmentID, session.ID, request.TaskEnvironmentID)
	}
	if captured[envGHToken] != "profile-token" {
		t.Fatalf("prepared workspace token = %q, want current profile token", captured[envGHToken])
	}
	if captured["GIT_CONFIG_COUNT"] != "2" || captured["GIT_CONFIG_KEY_0"] != "notes.augment.mergeStrategy" ||
		captured["GIT_CONFIG_VALUE_0"] != "union" || !isHostGitHubCredentialHelper(captured["GIT_CONFIG_VALUE_1"]) {
		t.Fatalf("prepared workspace Git environment = %#v", captured)
	}
}

func TestExecutorHostGHBridgePreparedWorkspacePropagatesEnvironmentDeliveryFailure(t *testing.T) {
	manager := &mockAgentManager{
		setExecutionEnvFunc: func(context.Context, string, map[string]string) error {
			return errors.New("agentctl unavailable")
		},
	}
	executor := newTestExecutor(t, manager, newMockRepository())
	err := executor.configureExistingWorkspace(
		context.Background(),
		&v1.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		&models.TaskSession{ID: "session-1", TaskID: "task-1"},
		"execution-1",
		"",
		&LaunchAgentRequest{WorkspaceID: "workspace-1", Env: map[string]string{"GH_TOKEN": "token"}},
	)
	if err == nil || !strings.Contains(err.Error(), "agentctl unavailable") {
		t.Fatalf("configureExistingWorkspace() error = %v, want environment delivery failure", err)
	}
}

func TestExecutorHostGHBridgeRepeatedPreparationReplacesGeneratedEntry(t *testing.T) {
	setupHostGHExecutable(t)
	executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})
	probes := 0
	executor.SetHostGitHubCredentialProbe(func(context.Context, string, string, map[string]string) error {
		probes++
		return nil
	})
	repositories := []*repoInfo{executorHostGHBridgeRepository("repo-1", "github.com")}
	request := executorHostGHBridgeRequest(nil)
	if err := executor.configureGitCredentialBrokerForRepositories(context.Background(), request, repositories); err != nil {
		t.Fatalf("first configuration: %v", err)
	}
	if err := executor.configureGitCredentialBrokerForRepositories(context.Background(), request, repositories); err != nil {
		t.Fatalf("second configuration: %v", err)
	}
	if probes != 2 {
		t.Fatalf("probe calls = %d, want one fresh probe per preparation", probes)
	}
	if got := request.Env["GIT_CONFIG_COUNT"]; got != "1" {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want one generated helper", got)
	}
	if !hasHostGitHubHelper(request.Env) {
		t.Fatalf("repeated preparation removed the generated helper: %#v", request.Env)
	}
}

type gitConfigTestEntry struct {
	key   string
	value string
}

func indexedGitConfigTestEnvironment(entries ...gitConfigTestEntry) map[string]string {
	env := map[string]string{"GIT_CONFIG_COUNT": strconv.Itoa(len(entries))}
	for index, entry := range entries {
		env["GIT_CONFIG_KEY_"+strconv.Itoa(index)] = entry.key
		env["GIT_CONFIG_VALUE_"+strconv.Itoa(index)] = entry.value
	}
	return env
}

func gitConfigEntriesFromEnvironment(t *testing.T, env map[string]string) []gitConfigTestEntry {
	t.Helper()
	count, err := strconv.Atoi(env["GIT_CONFIG_COUNT"])
	if err != nil {
		t.Fatalf("GIT_CONFIG_COUNT = %q: %v", env["GIT_CONFIG_COUNT"], err)
	}
	entries := make([]gitConfigTestEntry, 0, count)
	for index := 0; index < count; index++ {
		entries = append(entries, gitConfigTestEntry{
			key:   env["GIT_CONFIG_KEY_"+strconv.Itoa(index)],
			value: env["GIT_CONFIG_VALUE_"+strconv.Itoa(index)],
		})
	}
	return entries
}

func containsGitConfigEntry(entries []gitConfigTestEntry, key, value string) bool {
	for _, entry := range entries {
		if entry.key == key && entry.value == value {
			return true
		}
	}
	return false
}

func executorHostGHBridgeRequest(env map[string]string) *LaunchAgentRequest {
	return &LaunchAgentRequest{
		TaskID:       "task-1",
		WorkspaceID:  "workspace-1",
		SessionID:    "session-1",
		ExecutorType: string(models.ExecutorTypeWorktree),
		Env:          env,
	}
}

func executorHostGHBridgeRepository(id, host string) *repoInfo {
	return &repoInfo{
		RepositoryID: id,
		Repository: &models.Repository{
			Provider:      gitHubProviderID,
			ProviderOwner: "acme",
			ProviderName:  "widgets",
			ProviderHost:  "https://" + host,
			RemoteURL:     "https://" + host + "/acme/widgets.git",
		},
	}
}

func setupHostGHExecutable(t *testing.T) string {
	t.Helper()
	ghPath := filepath.Join(t.TempDir(), "host tools", "gh")
	if err := os.MkdirAll(filepath.Dir(ghPath), 0o700); err != nil {
		t.Fatalf("create fake gh directory: %v", err)
	}
	if err := os.WriteFile(ghPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(ghPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return ghPath
}

func hasHostGitHubHelper(env map[string]string) bool {
	count, _ := strconv.Atoi(env["GIT_CONFIG_COUNT"])
	for index := 0; index < count; index++ {
		if isHostGitHubCredentialHelper(env["GIT_CONFIG_VALUE_"+strconv.Itoa(index)]) {
			return true
		}
	}
	return false
}

func isolatedGitEnvironment(t *testing.T, overrides map[string]string) []string {
	t.Helper()
	envMap := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || strings.HasPrefix(key, "GIT_CONFIG_") {
			continue
		}
		envMap[key] = value
	}
	envMap["HOME"] = filepath.Join(t.TempDir(), "home")
	envMap["PATH"] = "/usr/bin:/bin"
	envMap["GIT_CONFIG_NOSYSTEM"] = "1"
	for key, value := range overrides {
		envMap[key] = value
	}

	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+envMap[key])
	}
	return env
}
