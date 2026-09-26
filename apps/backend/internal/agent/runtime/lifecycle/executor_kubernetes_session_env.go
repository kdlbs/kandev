package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/githubauth"
	corev1 "k8s.io/api/core/v1"
)

func kubernetesSessionHome(req *ExecutorCreateRequest) string {
	if !getMetadataBool(req.Metadata, metadataKubernetesTaskOwned) {
		return kubernetesAuthHomePath
	}
	sum := sha256.Sum256([]byte(req.SessionID))
	return path.Join("/run/kandev/sessions", hex.EncodeToString(sum[:16]), "home")
}

func kubernetesSessionAuthPath(req *ExecutorCreateRequest) string {
	if !getMetadataBool(req.Metadata, metadataKubernetesTaskOwned) {
		return kubernetesAuthEnvPath
	}
	return path.Join(kubernetesSessionHome(req), "auth.env")
}

func kubernetesSessionEnvironment(req *ExecutorCreateRequest) map[string]string {
	env := make(map[string]string, len(req.Env)+8)
	for key, value := range req.Env {
		if !isKubernetesOwnedControlEnvironmentKey(key) {
			env[key] = value
		}
	}
	env["HOME"] = kubernetesSessionHome(req)
	if req.InitialMode != nil && req.AgentConfig != nil && req.AgentConfig.Runtime() != nil {
		delivery := req.AgentConfig.Runtime().InitialMode
		if relativeDir, err := initialModeConfigRelativeDir(delivery); err == nil && delivery.ConfigDirEnvVar != "" {
			env[delivery.ConfigDirEnvVar] = path.Join(kubernetesSessionHome(req), relativeDir)
		}
	}
	env[kubernetesEnvSessionID] = req.SessionID
	env[kubernetesEnvTaskID] = req.TaskID
	env[kubernetesEnvInstanceID] = req.InstanceID
	env[kubernetesEnvEnvironmentID] = req.TaskEnvironmentID
	env[kubernetesEnvAgentProfile] = req.OfficeAgentProfileID
	env[kubernetesEnvExecutionProfile] = req.AgentProfileID
	if hasManagedGitCredentialBrokerEnv(req.Env) {
		env[githubauth.CredentialHelperPathEnv] = kubernetesAgentctlPath
	}
	return env
}

func (r *KubernetesExecutor) writeKubernetesSessionAuth(ctx context.Context, runtime *kubernetesRuntimeClient, req *ExecutorCreateRequest, pod *corev1.Pod, container string) error {
	data, err := kubernetesSerializeEnvironment(kubernetesSessionEnvironment(req))
	if err != nil {
		return err
	}
	return kubernetesWriteFile(ctx, runtime.streams, pod, container, kubernetesSessionAuthPath(req), data, 0o600)
}

func (r *KubernetesExecutor) prepareSharedKubernetesCredentials(ctx context.Context, runtime *kubernetesRuntimeClient, req *ExecutorCreateRequest, pod *corev1.Pod, container string) error {
	if err := r.writeKubernetesSessionAuth(ctx, runtime, req, pod, container); err != nil {
		return err
	}
	uploader := kubernetesPodFileUploader{streams: runtime.streams, pod: pod, container: container}
	return r.materializeKubernetesCredentials(ctx, uploader, runtime, req, pod, container)
}

// normalizeKubernetesManagedGitEnvironment binds the helper to the executable
// installed in the worker, never a path inherited from the backend host.
func normalizeKubernetesManagedGitEnvironment(runtimeName agentruntime.Runtime, env map[string]string) {
	if runtimeName == agentruntime.RuntimeKubernetes && hasManagedGitCredentialBrokerEnv(env) {
		env[githubauth.CredentialHelperPathEnv] = kubernetesAgentctlPath
	}
}
