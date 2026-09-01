package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"

	"github.com/kandev/kandev/internal/agent/agents"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agent/remoteauth"
)

var kubernetesSafePathComponent = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type kubernetesPodFileUploader struct {
	streams   *kubeexecutor.StreamOperations
	pod       *corev1.Pod
	container string
}

func (u kubernetesPodFileUploader) WriteFile(
	ctx context.Context,
	destination string,
	data []byte,
	mode os.FileMode,
) error {
	destination = strings.ReplaceAll(destination, "\\", "/")
	if !kubernetesSafeAbsolutePath(destination) {
		return fmt.Errorf("unsafe Kubernetes upload destination %q", destination)
	}
	return kubernetesWriteFile(ctx, u.streams, u.pod, u.container, destination, data, mode)
}

func (r *KubernetesExecutor) materializeKubernetesRemoteFiles(
	ctx context.Context,
	runtimeClient *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	pod *corev1.Pod,
	container string,
) error {
	uploader := kubernetesPodFileUploader{streams: runtimeClient.streams, pod: pod, container: container}
	if err := r.materializeKubernetesCredentials(ctx, uploader, runtimeClient, req, pod, container); err != nil {
		return fmt.Errorf("kubernetes lifecycle: materialize agent configuration: %w", err)
	}
	if err := materializeKubernetesSkillManifest(ctx, uploader, req.Metadata); err != nil {
		return fmt.Errorf("kubernetes lifecycle: materialize skill manifest: %w", err)
	}
	return nil
}

func (r *KubernetesExecutor) materializeKubernetesCredentials(
	ctx context.Context,
	uploader kubernetesPodFileUploader,
	runtimeClient *kubernetesRuntimeClient,
	req *ExecutorCreateRequest,
	pod *corev1.Pod,
	container string,
) error {
	if req.AgentConfig == nil {
		return nil
	}
	home := kubernetesAuthHomePath
	catalog := remoteauth.BuildCatalogForHost([]agents.Agent{req.AgentConfig}, runtime.GOOS, mustUserHome())
	selectedMethods, err := kubernetesSelectedCredentialMethods(req.Metadata, catalog)
	if err != nil {
		return err
	}
	if err := UploadCredentialFiles(ctx, uploader, selectedMethods, home, r.logger); err != nil {
		return err
	}
	warnings := UploadPortableConfigBundles(
		ctx, uploader, req.AgentConfig, selectedPortableConfigBundleIDs(req.Metadata), home, r.logger,
	)
	reportPortableConfigWarnings(req.OnProgress, warnings)
	for _, spec := range catalog.Specs {
		for _, method := range spec.Methods {
			if method.Type != authMethodTypeEnv || method.EnvVar == "" || method.SetupScript == "" || req.Env[method.EnvVar] == "" {
				continue
			}
			command := "set -eu; export HOME=" + shellQuote(home) +
				"; set -a; . " + shellQuote(kubernetesAuthEnvPath) + "; set +a; " + method.SetupScript
			if execErr := runtimeClient.streams.Exec(ctx, kubeexecutor.ExecRequest{
				Namespace: pod.Namespace, Pod: pod.Name, Container: container,
				Command: []string{"sh", "-c", command},
			}); execErr != nil {
				r.logger.Warn("Kubernetes agent auth setup script failed",
					zap.String("method_id", method.MethodID), zap.Error(execErr))
			}
		}
	}
	return nil
}

func kubernetesSelectedCredentialMethods(
	metadata map[string]interface{},
	catalog remoteauth.Catalog,
) ([]remoteauth.Method, error) {
	raw := getMetadataString(metadata, "remote_credentials")
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, fmt.Errorf("parse remote_credentials: %w", err)
	}
	methods := make([]remoteauth.Method, 0, len(ids))
	for _, id := range ids {
		method, ok := catalog.FindMethod(id)
		if !ok || method.Type != authMethodTypeFiles {
			continue
		}
		methods = append(methods, method)
	}
	return methods, nil
}

func mustUserHome() string {
	home, _ := os.UserHomeDir()
	return home
}

type kubernetesManifestSupportFile struct {
	Path    string
	Content string
}

type kubernetesManifestSkill struct {
	Slug    string
	Content string
	Files   []kubernetesManifestSupportFile
}

type kubernetesManifestInstruction struct {
	Filename string
	Content  string
	IsEntry  bool
}

type kubernetesSkillManifest struct {
	Skills          []kubernetesManifestSkill
	Instructions    []kubernetesManifestInstruction
	AgentTypeID     string
	WorkspaceSlug   string
	AgentID         string
	ProjectSkillDir string
}

func materializeKubernetesSkillManifest(
	ctx context.Context,
	uploader kubernetesPodFileUploader,
	metadata map[string]interface{},
) error {
	manifest, err := decodeKubernetesSkillManifest(metadata)
	if err != nil || manifest == nil {
		return err
	}
	if err := materializeKubernetesSkills(ctx, uploader, manifest); err != nil {
		return err
	}
	instructionsRoot, err := kubernetesManifestInstructionsRoot(manifest)
	if err != nil || instructionsRoot == "" {
		return err
	}
	return materializeKubernetesInstructions(ctx, uploader, instructionsRoot, manifest.Instructions)
}

func decodeKubernetesSkillManifest(metadata map[string]interface{}) (*kubernetesSkillManifest, error) {
	raw := getMetadataString(metadata, MetadataKeySkillManifestJSON)
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var manifest kubernetesSkillManifest
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	return &manifest, nil
}

func materializeKubernetesSkills(
	ctx context.Context,
	uploader kubernetesPodFileUploader,
	manifest *kubernetesSkillManifest,
) error {
	projectDir, ok := kubernetesSafeRelativePath(manifest.ProjectSkillDir)
	if !ok {
		if manifest.ProjectSkillDir != "" {
			return fmt.Errorf("manifest project skill directory is unsafe")
		}
		projectDir = ".agents/skills"
	}
	for _, item := range manifest.Skills {
		if !kubernetesSafeComponent(item.Slug) {
			return fmt.Errorf("manifest skill slug contains an unsafe path component")
		}
		root := path.Join(kubernetesWorkspacePath, projectDir, "kandev-"+item.Slug)
		if !kubernetesPathWithinRoot(root, kubernetesWorkspacePath) {
			return fmt.Errorf("manifest skill root escapes the workspace")
		}
		if err := uploader.WriteFile(ctx, path.Join(root, "SKILL.md"), []byte(item.Content), 0o644); err != nil {
			return err
		}
		for _, support := range item.Files {
			relative, safe := kubernetesSafeRelativePath(support.Path)
			if !safe || relative == "SKILL.md" {
				return fmt.Errorf("manifest skill support path is unsafe")
			}
			destination := path.Join(root, relative)
			if !kubernetesPathWithinRoot(destination, root) {
				return fmt.Errorf("manifest skill support path escapes its skill root")
			}
			if err := uploader.WriteFile(ctx, destination, []byte(support.Content), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func kubernetesManifestInstructionsRoot(manifest *kubernetesSkillManifest) (string, error) {
	if !kubernetesSafeComponent(manifest.WorkspaceSlug) || !kubernetesSafeComponent(manifest.AgentID) {
		if len(manifest.Instructions) == 0 {
			return "", nil
		}
		return "", fmt.Errorf("manifest instruction identity contains an unsafe path component")
	}
	root := path.Join("/opt/kandev/runtime", manifest.WorkspaceSlug, "instructions", manifest.AgentID)
	if !kubernetesPathWithinRoot(root, "/opt/kandev/runtime") {
		return "", fmt.Errorf("manifest instruction root escapes the runtime directory")
	}
	return root, nil
}

func materializeKubernetesInstructions(
	ctx context.Context,
	uploader kubernetesPodFileUploader,
	root string,
	instructions []kubernetesManifestInstruction,
) error {
	for _, instruction := range instructions {
		if !kubernetesSafeComponent(instruction.Filename) {
			return fmt.Errorf("manifest instruction filename contains an unsafe path component")
		}
		destination := path.Join(root, instruction.Filename)
		if !kubernetesPathWithinRoot(destination, root) {
			return fmt.Errorf("manifest instruction path escapes its instruction root")
		}
		if err := uploader.WriteFile(ctx, destination, []byte(instruction.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func kubernetesSafeComponent(value string) bool {
	return value != "." && value != ".." && kubernetesSafePathComponent.MatchString(value)
}

func kubernetesPathWithinRoot(candidate, root string) bool {
	candidate = path.Clean(candidate)
	root = path.Clean(root)
	return candidate == root || strings.HasPrefix(candidate, root+"/")
}

func kubernetesSafeRelativePath(value string) (string, bool) {
	if value == "" || strings.Contains(value, "\\") || strings.ContainsRune(value, '\x00') || path.IsAbs(value) {
		return "", false
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

func kubernetesSafeAbsolutePath(value string) bool {
	return path.IsAbs(value) && !strings.ContainsRune(value, '\x00') && path.Clean(value) == value
}
