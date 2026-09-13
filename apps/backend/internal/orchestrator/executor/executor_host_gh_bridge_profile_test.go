package executor

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestExecutorHostGHBridgeCleanupPreservesUserOwnedGHHelpers(t *testing.T) {
	ghPath := setupHostGHExecutable(t)
	userHelper := "!'" + filepath.Join(t.TempDir(), "custom gh") + "' auth git-credential"
	otherHostHelper := "!'" + filepath.Join(t.TempDir(), "other gh") + "' auth git-credential"
	oldGenerated := hostGitHubCredentialHelper("/old/generated/gh")
	req := executorHostGHBridgeRequest(indexedGitConfigTestEnvironment(
		gitConfigTestEntry{key: "credential.https://github.com.helper", value: userHelper},
		gitConfigTestEntry{key: "credential.https://git.example.helper", value: otherHostHelper},
		gitConfigTestEntry{key: "credential.https://github.com.helper", value: oldGenerated},
	))
	executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})
	executor.SetHostGitHubCredentialProbe(func(context.Context, string, string, map[string]string) error {
		return nil
	})

	if err := executor.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{executorHostGHBridgeRepository("repo-1", "github.com")}); err != nil {
		t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
	}
	entries := gitConfigEntriesFromEnvironment(t, req.Env)
	if !containsGitConfigEntry(entries, "credential.https://github.com.helper", userHelper) {
		t.Fatalf("user GitHub helper was removed: %#v", entries)
	}
	if !containsGitConfigEntry(entries, "credential.https://git.example.helper", otherHostHelper) {
		t.Fatalf("other-host helper was removed: %#v", entries)
	}
	if !containsGitConfigEntry(entries, "credential.https://github.com.helper", hostGitHubCredentialHelper(ghPath)) {
		t.Fatalf("replacement generated helper missing: %#v", entries)
	}
	if containsGitConfigEntry(entries, "credential.https://github.com.helper", oldGenerated) {
		t.Fatalf("obsolete generated helper remained: %#v", entries)
	}
}

func TestExecutorHostGHBridgeCleanupPreservesUserHelperWhenCLIUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	userHelper := "!'" + filepath.Join(t.TempDir(), "custom gh") + "' auth git-credential"
	req := executorHostGHBridgeRequest(indexedGitConfigTestEnvironment(
		gitConfigTestEntry{key: "credential.https://github.com.helper", value: userHelper},
	))
	executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})
	if err := executor.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{executorHostGHBridgeRepository("repo-1", "github.com")}); err != nil {
		t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
	}
	entries := gitConfigEntriesFromEnvironment(t, req.Env)
	if !containsGitConfigEntry(entries, "credential.https://github.com.helper", userHelper) {
		t.Fatalf("user helper was removed while replacement CLI was unavailable: %#v", entries)
	}
}

func TestExecutorHostGHBridgeProbeUsesProfileCredentialDirectories(t *testing.T) {
	setupHostGHExecutable(t)
	cases := []struct {
		name         string
		parentKey    string
		parentValue  string
		profileKey   string
		profileValue string
	}{
		{name: "HOME", parentKey: "HOME", parentValue: "/backend/home", profileKey: "HOME", profileValue: "/profile/home"},
		{name: "GH_CONFIG_DIR", parentKey: "GH_CONFIG_DIR", parentValue: "/backend/gh", profileKey: "GH_CONFIG_DIR", profileValue: "/profile/gh"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.parentKey, test.parentValue)
			executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
			executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
				policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
			})
			var probedEnv map[string]string
			executor.SetHostGitHubCredentialProbe(func(_ context.Context, _ string, _ string, env map[string]string) error {
				probedEnv = cloneStringMap(env)
				if env[test.profileKey] != test.profileValue {
					return errors.New("selected profile credential store is unavailable")
				}
				return nil
			})
			req := executorHostGHBridgeRequest(nil)
			err := executor.configureGitCredentialBrokerForRepositoriesWithProfileEnv(
				context.Background(), req,
				[]*repoInfo{executorHostGHBridgeRepository("repo-1", "github.com")},
				[]models.ProfileEnvVar{{Key: test.profileKey, Value: test.profileValue}},
			)
			if err != nil {
				t.Fatalf("configureGitCredentialBrokerForRepositoriesWithProfileEnv() error = %v", err)
			}
			if probedEnv[test.profileKey] != test.profileValue {
				t.Fatalf("probe %s = %q, want profile value %q; env=%#v", test.profileKey, probedEnv[test.profileKey], test.profileValue, probedEnv)
			}
			if !hasHostGitHubHelper(req.Env) {
				t.Fatalf("profile credential store did not activate host bridge: %#v", req.Env)
			}
		})
	}
}

func TestExecutorHostGHBridgeProbeRevealsOnlyWinningProfileDirectory(t *testing.T) {
	executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	executor.secretStore = &mockSecretStore{secrets: map[string]string{
		"executor-home": "/executor/home",
	}}
	probeEnv, ok := executor.hostGitHubProbeEnvironment(context.Background(), nil, []models.ProfileEnvVar{
		{Key: "HOME", SecretID: "missing-agent-home"},
		{Key: "HOME", SecretID: "executor-home"},
	})
	if !ok {
		t.Fatal("hostGitHubProbeEnvironment() failed while resolving the winning profile directory")
	}
	if got := probeEnv["HOME"]; got != "/executor/home" {
		t.Fatalf("probe HOME = %q, want winning executor-profile directory", got)
	}
}

func TestExecutorHostGHBridgeResolvesEffectiveAgentAndExecutorProfiles(t *testing.T) {
	setupHostGHExecutable(t)
	manager := &mockAgentManager{
		resolveAgentProfileFunc: func(_ context.Context, profileID string) (*AgentProfileInfo, error) {
			if profileID != "agent-profile" {
				t.Fatalf("resolved profile ID = %q, want agent-profile", profileID)
			}
			return &AgentProfileInfo{
				EnvVars: []models.ProfileEnvVar{{Key: "HOME", Value: "/agent/home"}},
			}, nil
		},
	}
	executor := newTestExecutor(t, manager, newMockRepository())
	executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})
	var probedEnv map[string]string
	executor.SetHostGitHubCredentialProbe(func(_ context.Context, _ string, _ string, env map[string]string) error {
		probedEnv = cloneStringMap(env)
		return nil
	})

	profileEnv, resolved := executor.resolveHostGitHubBridgeProfileEnv(
		context.Background(), "agent-profile",
		[]models.ProfileEnvVar{{Key: "GH_CONFIG_DIR", Value: "/executor/gh"}},
	)
	if !resolved {
		t.Fatal("profile environment was not resolved")
	}
	req := executorHostGHBridgeRequest(map[string]string{"HOME": "/request/home"})
	if err := executor.configureGitCredentialBrokerForRepositoriesWithProfileEnvAndBridge(
		context.Background(), req,
		[]*repoInfo{executorHostGHBridgeRepository("repo-1", "github.com")},
		profileEnv, resolved,
	); err != nil {
		t.Fatalf("configureGitCredentialBrokerForRepositoriesWithProfileEnvAndBridge() error = %v", err)
	}
	if probedEnv["HOME"] != "/request/home" {
		t.Fatalf("probe HOME = %q, want request value", probedEnv["HOME"])
	}
	if probedEnv["GH_CONFIG_DIR"] != "/executor/gh" {
		t.Fatalf("probe GH_CONFIG_DIR = %q, want executor-profile value", probedEnv["GH_CONFIG_DIR"])
	}
	if !hasHostGitHubHelper(req.Env) {
		t.Fatalf("effective profile stores did not activate host bridge: %#v", req.Env)
	}
}

func TestLaunchPreparedSessionProbesEffectiveProfileCredentialStore(t *testing.T) {
	setupHostGHExecutable(t)
	cases := []struct {
		name          string
		agentEnv      []models.ProfileEnvVar
		executorEnv   []models.ProfileEnvVar
		selectionKey  string
		selectionPath string
	}{
		{
			name:          "agent profile HOME",
			agentEnv:      []models.ProfileEnvVar{{Key: "HOME", Value: "/profile/home"}},
			selectionKey:  "HOME",
			selectionPath: "/profile/home",
		},
		{
			name:          "executor profile GH_CONFIG_DIR",
			executorEnv:   []models.ProfileEnvVar{{Key: "GH_CONFIG_DIR", Value: "/profile/gh"}},
			selectionKey:  "GH_CONFIG_DIR",
			selectionPath: "/profile/gh",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.selectionKey, "/backend/credential-store")
			repo := newMockRepository()
			const taskID = "task-effective-profile-store"
			const sessionID = "session-effective-profile-store"
			repo.repositories["repo-1"] = &models.Repository{
				ID:            "repo-1",
				Name:          "widgets",
				SourceType:    sourceTypeLocal,
				LocalPath:     t.TempDir(),
				Provider:      gitHubProviderID,
				ProviderOwner: "acme",
				ProviderName:  "widgets",
				RemoteURL:     "https://github.com/acme/widgets.git",
			}
			repo.taskRepositories["task-repo-1"] = &models.TaskRepository{
				ID: "task-repo-1", TaskID: taskID, RepositoryID: "repo-1", Position: 0,
			}
			repo.executors["executor-1"] = &models.Executor{
				ID: "executor-1", Type: models.ExecutorTypeWorktree, Status: models.ExecutorStatusActive,
			}
			repo.executorProfiles["executor-profile"] = &models.ExecutorProfile{
				ID: "executor-profile", ExecutorID: "executor-1", EnvVars: test.executorEnv,
			}
			repo.sessions[sessionID] = &models.TaskSession{
				ID: sessionID, TaskID: taskID, AgentProfileID: "agent-profile",
				ExecutorID: "executor-1", ExecutorProfileID: "executor-profile",
				State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
			}

			var probedEnv map[string]string
			manager := &mockAgentManager{
				resolveAgentProfileFunc: func(_ context.Context, profileID string) (*AgentProfileInfo, error) {
					if profileID != "agent-profile" {
						t.Fatalf("resolved profile ID = %q, want agent-profile", profileID)
					}
					return &AgentProfileInfo{ProfileID: profileID, EnvVars: test.agentEnv}, nil
				},
				launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
					if !hasHostGitHubHelper(req.Env) {
						t.Fatalf("production launch request has no host helper: %#v", req.Env)
					}
					return &LaunchAgentResponse{AgentExecutionID: "execution-1", Status: v1.AgentStatusStarting}, nil
				},
			}
			executor := newTestExecutor(t, manager, repo)
			executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
				policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
			})
			executor.SetHostGitHubCredentialProbe(func(_ context.Context, _ string, _ string, env map[string]string) error {
				probedEnv = cloneStringMap(env)
				if env[test.selectionKey] != test.selectionPath {
					return errors.New("probe selected the wrong credential store")
				}
				return nil
			})

			_, err := executor.LaunchPreparedSession(context.Background(), &v1.Task{
				ID: taskID, WorkspaceID: "workspace-1", Title: "effective profile store",
			}, sessionID, LaunchOptions{
				AgentProfileID: "agent-profile", ExecutorID: "executor-1", StartAgent: false,
			})
			if err != nil {
				t.Fatalf("LaunchPreparedSession() error = %v", err)
			}
			if probedEnv[test.selectionKey] != test.selectionPath {
				t.Fatalf("probe %s = %q, want %q; env=%#v", test.selectionKey, probedEnv[test.selectionKey], test.selectionPath, probedEnv)
			}
		})
	}
}
