package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"

	"github.com/kandev/kandev/internal/githubauth"
	corev1 "k8s.io/api/core/v1"
)

const (
	kubernetesSessionNPMCachePath = "/opt/kandev/cache/npm"
	kubernetesSessionXDGCachePath = "/opt/kandev/cache/xdg"
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
	if getMetadataBool(req.Metadata, metadataKubernetesTaskOwned) {
		npmCachePath, hasProfileNPMCache := kubernetesSessionNPMCacheOverride(req.Env)
		if !hasProfileNPMCache {
			npmCachePath = kubernetesSessionNPMCachePath
		}
		for key, value := range req.Env {
			if value == "" && strings.EqualFold(key, "npm_config_cache") {
				env[key] = npmCachePath
			}
		}
		if !hasProfileNPMCache {
			env["NPM_CONFIG_CACHE"] = npmCachePath
			env["npm_config_cache"] = npmCachePath
		}
		if env["XDG_CACHE_HOME"] == "" {
			env["XDG_CACHE_HOME"] = kubernetesSessionXDGCachePath
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

func kubernetesSessionNPMCacheOverride(env map[string]string) (string, bool) {
	for key, value := range env {
		if strings.EqualFold(key, "npm_config_cache") && value != "" {
			return value, true
		}
	}
	return "", false
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
