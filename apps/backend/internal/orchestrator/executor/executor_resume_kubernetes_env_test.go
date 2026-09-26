package executor

import (
	"context"
	"errors"
	"reflect"
	"testing"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestKubernetesResumeRestoresRecordedProfileEnvironment(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	session := repo.sessions["sess-1"]
	session.ExecutorProfileID = "untrusted-profile"
	repo.executors["recorded-kubernetes-executor"] = &models.Executor{ID: "recorded-kubernetes-executor", Type: models.ExecutorTypeKubernetes, Config: map[string]string{"auth_mode": "kubeconfig", "kubeconfig_path": "/tmp/test-config", "namespace": "kandev-agents"}}
	repo.executorsRunning[session.ID] = &models.ExecutorRunning{
		ID: session.ID, SessionID: session.ID, TaskID: session.TaskID,
		ExecutorID: "recorded-kubernetes-executor", Runtime: agentruntime.RuntimeKubernetes, Metadata: recordedKubernetesResumeMetadata(),
	}
	repo.executorProfiles["recorded-profile"] = &models.ExecutorProfile{
		ID: "recorded-profile", ExecutorID: "recorded-kubernetes-executor",
		Config:  map[string]string{"pod_template_yaml": "edited template must not replace snapshot"},
		EnvVars: []models.ProfileEnvVar{{Key: "PYTHONPATH", Value: "/artifacts/runtime"}, {Key: "INTEL_DATABASE_URL", SecretID: "database-secret"}},
	}
	repo.executorProfiles["untrusted-profile"] = &models.ExecutorProfile{
		ID: "untrusted-profile", ExecutorID: "recorded-kubernetes-executor", EnvVars: []models.ProfileEnvVar{{Key: "INJECTED", Value: "bad"}},
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req, _, _, _, _, err := exec.buildResumeRequest(context.Background(), &v1.Task{ID: session.TaskID, WorkspaceID: "workspace-1"}, session, true)
	if err != nil {
		t.Fatal(err)
	}
	var got []runtimeenv.Definition
	for _, def := range req.EnvironmentDefinitions {
		if def.Origin == runtimeenv.OriginExecutorProfile {
			got = append(got, def)
		}
	}
	want := []runtimeenv.Definition{
		{Key: "PYTHONPATH", Literal: "/artifacts/runtime", Origin: runtimeenv.OriginExecutorProfile},
		{Key: "INTEL_DATABASE_URL", SecretID: "database-secret", Origin: runtimeenv.OriginExecutorProfile},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profile environment = %#v, want %#v", got, want)
	}
	if req.Metadata[lifecycle.MetadataKeyKubernetesProfileSnapshot] != recordedKubernetesResumeMetadata()[lifecycle.MetadataKeyKubernetesProfileSnapshot] {
		t.Fatal("recorded workload snapshot changed")
	}
}

func TestKubernetesResumeProfileEnvironmentFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		profile   *models.ExecutorProfile
		lookupErr error
		wantError bool
	}{
		{name: "deleted profile", lookupErr: repoerrors.ErrExecutorProfileNotFound},
		{name: "absent profile"},
		{name: "database failure", lookupErr: errors.New("database unavailable"), wantError: true},
		{name: "different executor", profile: &models.ExecutorProfile{ExecutorID: "other"}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &resumeProfileRepository{mockRepository: newMockRepository(), profile: tc.profile, err: tc.lookupErr}
			exec := newTestExecutor(t, &mockAgentManager{}, repo.mockRepository)
			exec.repo = repo
			config := executorConfig{ExecutorID: "recorded"}
			err := exec.restoreKubernetesProfileEnvironment(context.Background(), &config, map[string]interface{}{lifecycle.MetadataKeyExecutorProfileID: "profile"})
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, want error %v", err, tc.wantError)
			}
			if len(config.ProfileEnvVars) != 0 {
				t.Fatal("loaded environment from unavailable or foreign profile")
			}
		})
	}
}

func TestKubernetesResumeWithoutRecordedProfileSkipsLookup(t *testing.T) {
	for _, recorded := range []map[string]interface{}{nil, {}, {lifecycle.MetadataKeyExecutorProfileID: ""}} {
		repo := &resumeProfileRepository{mockRepository: newMockRepository(), err: errors.New("unexpected profile lookup")}
		exec := newTestExecutor(t, &mockAgentManager{}, repo.mockRepository)
		exec.repo = repo
		config := executorConfig{ExecutorID: "recorded"}
		if err := exec.restoreKubernetesProfileEnvironment(context.Background(), &config, recorded); err != nil {
			t.Fatalf("resume without a recorded profile: %v", err)
		}
		if len(config.ProfileEnvVars) != 0 {
			t.Fatal("loaded environment without a recorded profile")
		}
	}
}

type resumeProfileRepository struct {
	*mockRepository
	profile *models.ExecutorProfile
	err     error
}

func (r *resumeProfileRepository) GetExecutorProfile(context.Context, string) (*models.ExecutorProfile, error) {
	return r.profile, r.err
}
