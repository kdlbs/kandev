// Package kubernetes defines Kubernetes executor configuration and resource composition.
package kubernetes

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	DefaultRequestTimeoutSeconds = 30
	MaxRequestTimeoutSeconds     = 300
	DefaultMainContainerName     = "kandev-agent"
)

type FieldError struct {
	Path    string
	Message string
}

func (e *FieldError) Error() string { return fmt.Sprintf("%s: %s", e.Path, e.Message) }

type AuthMode string

const (
	AuthModeKubeconfig AuthMode = "kubeconfig"
	AuthModeInCluster  AuthMode = "in_cluster"
)

type ExecutorConfig struct {
	AuthMode              AuthMode `json:"auth_mode"`
	KubeconfigPath        string   `json:"kubeconfig_path,omitempty"`
	KubeContext           string   `json:"kube_context,omitempty"`
	Namespace             string   `json:"namespace"`
	RequestTimeoutSeconds int      `json:"request_timeout_seconds"`
}

func (c ExecutorConfig) Validate() error {
	switch c.AuthMode {
	case AuthModeKubeconfig:
		if !filepath.IsAbs(c.KubeconfigPath) {
			return fieldError("config.kubeconfig_path", "must be an absolute path for kubeconfig auth")
		}
	case AuthModeInCluster:
		if c.KubeconfigPath != "" {
			return fieldError("config.kubeconfig_path", "must be empty for in-cluster auth")
		}
		if c.KubeContext != "" {
			return fieldError("config.kube_context", "must be empty for in-cluster auth")
		}
	default:
		return fieldError("config.auth_mode", "must be kubeconfig or in_cluster")
	}
	if errs := validation.IsDNS1123Label(c.Namespace); len(errs) > 0 {
		return fieldError("config.namespace", "must be a valid Kubernetes namespace")
	}
	if c.RequestTimeoutSeconds < 1 || c.RequestTimeoutSeconds > MaxRequestTimeoutSeconds {
		return fieldError("config.request_timeout_seconds", "must be between 1 and 300")
	}
	return nil
}

func ParseExecutorConfig(values map[string]string) (ExecutorConfig, error) {
	timeout, err := parseTimeout(values["request_timeout_seconds"])
	if err != nil {
		return ExecutorConfig{}, err
	}
	cfg := ExecutorConfig{
		AuthMode:              AuthMode(strings.TrimSpace(values["auth_mode"])),
		KubeconfigPath:        strings.TrimSpace(values["kubeconfig_path"]),
		KubeContext:           strings.TrimSpace(values["kube_context"]),
		Namespace:             strings.TrimSpace(values["namespace"]),
		RequestTimeoutSeconds: timeout,
	}
	if err := cfg.Validate(); err != nil {
		return ExecutorConfig{}, err
	}
	return cfg, nil
}

type Platform string

const (
	PlatformLinuxAMD64 Platform = "linux/amd64"
	PlatformLinuxARM64 Platform = "linux/arm64"
)

type WorkspaceMode string

const (
	WorkspaceModeManagedPVC    WorkspaceMode = "managed_pvc"
	WorkspaceModeEmptyDir      WorkspaceMode = "empty_dir"
	WorkspaceModeExistingClaim WorkspaceMode = "existing_claim"
)

type WorkspaceConfig struct {
	Mode         WorkspaceMode `json:"mode"`
	Size         string        `json:"size,omitempty"`
	StorageClass string        `json:"storage_class,omitempty"`
	AccessModes  []string      `json:"access_modes,omitempty"`
	ClaimName    string        `json:"claim_name,omitempty"`
}

type ProfileConfig struct {
	Platform        Platform        `json:"platform"`
	MainContainer   string          `json:"main_container"`
	PodTemplateYAML string          `json:"pod_template_yaml"`
	Workspace       WorkspaceConfig `json:"workspace"`
}

func (c ProfileConfig) Validate() error {
	if c.Platform != PlatformLinuxAMD64 && c.Platform != PlatformLinuxARM64 {
		return fieldError("config.platform", "must be linux/amd64 or linux/arm64")
	}
	if errs := validation.IsDNS1123Label(c.MainContainer); len(errs) > 0 {
		return fieldError("config.main_container", "must be a valid container name")
	}
	if strings.TrimSpace(c.PodTemplateYAML) == "" {
		return fieldError("config.pod_template_yaml", "is required")
	}
	template, err := ParsePodTemplate(c.PodTemplateYAML)
	if err != nil {
		return err
	}
	if err := ValidatePodTemplate(template, c.MainContainer); err != nil {
		return err
	}
	return c.Workspace.validate()
}

func ParseProfileConfig(values map[string]string) (ProfileConfig, error) {
	mainContainer := strings.TrimSpace(values["main_container"])
	if mainContainer == "" {
		mainContainer = DefaultMainContainerName
	}
	accessModes, err := parseAccessModes(configValue(values, "workspace.access_modes", "workspace_access_modes"))
	if err != nil {
		return ProfileConfig{}, err
	}
	mode := WorkspaceMode(strings.TrimSpace(configValue(values, "workspace.mode", "workspace_mode")))
	if mode == WorkspaceModeManagedPVC && len(accessModes) == 0 {
		accessModes = []string{"ReadWriteOnce"}
	}
	cfg := ProfileConfig{
		Platform:        Platform(strings.TrimSpace(values["platform"])),
		MainContainer:   mainContainer,
		PodTemplateYAML: values["pod_template_yaml"],
		Workspace: WorkspaceConfig{
			Mode:         mode,
			Size:         strings.TrimSpace(configValue(values, "workspace.size", "workspace_size")),
			StorageClass: strings.TrimSpace(configValue(values, "workspace.storage_class", "workspace_storage_class")),
			AccessModes:  accessModes,
			ClaimName:    strings.TrimSpace(configValue(values, "workspace.claim_name", "workspace_claim_name")),
		},
	}
	if err := cfg.Validate(); err != nil {
		return ProfileConfig{}, err
	}
	return cfg, nil
}

func (c WorkspaceConfig) validate() error {
	switch c.Mode {
	case WorkspaceModeManagedPVC:
		return c.validateManagedPVC()
	case WorkspaceModeEmptyDir:
		return c.validateEmptyDir()
	case WorkspaceModeExistingClaim:
		return c.validateExistingClaim()
	default:
		return fieldError("config.workspace.mode", "must be managed_pvc, empty_dir, or existing_claim")
	}
}

func (c WorkspaceConfig) validateManagedPVC() error {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(c.Size))
	if err != nil || quantity.Sign() <= 0 {
		return fieldError("config.workspace.size", "must be a positive Kubernetes quantity")
	}
	if c.ClaimName != "" {
		return fieldError("config.workspace.claim_name", "must be empty for managed_pvc")
	}
	if c.StorageClass != "" && len(validation.IsDNS1123Subdomain(c.StorageClass)) > 0 {
		return fieldError("config.workspace.storage_class", "must be a valid storage class name")
	}
	if len(c.AccessModes) == 0 {
		return fieldError("config.workspace.access_modes", "must contain at least one access mode")
	}
	for _, mode := range c.AccessModes {
		if !validAccessMode(mode) {
			return fieldError("config.workspace.access_modes", "contains an unsupported access mode")
		}
	}
	return nil
}

func (c WorkspaceConfig) validateEmptyDir() error {
	switch {
	case c.ClaimName != "":
		return fieldError("config.workspace.claim_name", "must be empty for empty_dir")
	case c.Size != "":
		return fieldError("config.workspace.size", "must be empty for empty_dir")
	case c.StorageClass != "":
		return fieldError("config.workspace.storage_class", "must be empty for empty_dir")
	case len(c.AccessModes) > 0:
		return fieldError("config.workspace.access_modes", "must be empty for empty_dir")
	default:
		return nil
	}
}

func (c WorkspaceConfig) validateExistingClaim() error {
	if errs := validation.IsDNS1123Subdomain(c.ClaimName); len(errs) > 0 {
		return fieldError("config.workspace.claim_name", "must be a valid claim name")
	}
	switch {
	case c.Size != "":
		return fieldError("config.workspace.size", "must be empty for existing_claim")
	case c.StorageClass != "":
		return fieldError("config.workspace.storage_class", "must be empty for existing_claim")
	case len(c.AccessModes) > 0:
		return fieldError("config.workspace.access_modes", "must be empty for existing_claim")
	default:
		return nil
	}
}

func validAccessMode(mode string) bool {
	switch mode {
	case "ReadWriteOnce", "ReadOnlyMany", "ReadWriteMany", "ReadWriteOncePod":
		return true
	default:
		return false
	}
}

func parseTimeout(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return DefaultRequestTimeoutSeconds, nil
	}
	timeout, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fieldError("config.request_timeout_seconds", "must be an integer")
	}
	return timeout, nil
}

func parseAccessModes(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var modes []string
	if err := json.Unmarshal([]byte(raw), &modes); err != nil || modes == nil {
		return nil, fieldError("config.workspace.access_modes", "must be a JSON array of access modes")
	}
	for i := range modes {
		modes[i] = strings.TrimSpace(modes[i])
	}
	return modes, nil
}

func configValue(values map[string]string, preferred, fallback string) string {
	if value, ok := values[preferred]; ok {
		return value
	}
	return values[fallback]
}

func fieldError(path, message string) error {
	return &FieldError{Path: path, Message: message}
}
