package kubernetes

import (
	"errors"
	"reflect"
	"testing"
)

func TestExecutorConfigValidateReportsStableFieldPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  ExecutorConfig
		path string
	}{
		{name: "auth mode", cfg: ExecutorConfig{Namespace: "agents", RequestTimeoutSeconds: 30}, path: "config.auth_mode"},
		{name: "kubeconfig path", cfg: ExecutorConfig{AuthMode: AuthModeKubeconfig, KubeconfigPath: "relative", Namespace: "agents", RequestTimeoutSeconds: 30}, path: "config.kubeconfig_path"},
		{name: "in-cluster path", cfg: ExecutorConfig{AuthMode: AuthModeInCluster, KubeconfigPath: "/secret/config", Namespace: "agents", RequestTimeoutSeconds: 30}, path: "config.kubeconfig_path"},
		{name: "namespace", cfg: ExecutorConfig{AuthMode: AuthModeInCluster, Namespace: "Bad_Namespace", RequestTimeoutSeconds: 30}, path: "config.namespace"},
		{name: "timeout", cfg: ExecutorConfig{AuthMode: AuthModeInCluster, Namespace: "agents", RequestTimeoutSeconds: 301}, path: "config.request_timeout_seconds"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			var fieldErr *FieldError
			if !errors.As(err, &fieldErr) {
				t.Fatalf("Validate() error = %v, want FieldError", err)
			}
			if fieldErr.Path != tt.path {
				t.Fatalf("Validate() path = %q, want %q", fieldErr.Path, tt.path)
			}
		})
	}
}

func TestParseExecutorConfigSupportsBothAuthModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   map[string]string
		want ExecutorConfig
	}{
		{
			name: "kubeconfig",
			in: map[string]string{
				"auth_mode": "kubeconfig", "kubeconfig_path": "/etc/kandev/cluster.yaml",
				"kube_context": "production", "namespace": "kandev-agents", "request_timeout_seconds": "45",
			},
			want: ExecutorConfig{AuthMode: AuthModeKubeconfig, KubeconfigPath: "/etc/kandev/cluster.yaml", KubeContext: "production", Namespace: "kandev-agents", RequestTimeoutSeconds: 45},
		},
		{
			name: "in cluster defaults timeout",
			in:   map[string]string{"auth_mode": "in_cluster", "namespace": "agents"},
			want: ExecutorConfig{AuthMode: AuthModeInCluster, Namespace: "agents", RequestTimeoutSeconds: DefaultRequestTimeoutSeconds},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExecutorConfig(tt.in)
			if err != nil {
				t.Fatalf("ParseExecutorConfig() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseExecutorConfig() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestProfileConfigValidateSupportsAllWorkspaceModes(t *testing.T) {
	t.Parallel()

	template := "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: kandev-agent\n        image: example/agent:latest\n"
	tests := []WorkspaceConfig{
		{Mode: WorkspaceModeManagedPVC, Size: "20Gi", AccessModes: []string{"ReadWriteOnce"}},
		{Mode: WorkspaceModeEmptyDir},
		{Mode: WorkspaceModeExistingClaim, ClaimName: "shared-workspace"},
	}

	for _, workspace := range tests {
		workspace := workspace
		t.Run(string(workspace.Mode), func(t *testing.T) {
			t.Parallel()
			cfg := ProfileConfig{Platform: PlatformLinuxAMD64, MainContainer: "kandev-agent", PodTemplateYAML: template, Workspace: workspace}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestProfileConfigValidateRejectsModeSpecificFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		workspace WorkspaceConfig
		path      string
	}{
		{workspace: WorkspaceConfig{Mode: WorkspaceModeManagedPVC}, path: "config.workspace.size"},
		{
			workspace: WorkspaceConfig{
				Mode: WorkspaceModeManagedPVC, Size: "10Gi",
				StorageClass: "Invalid_Storage_Class", AccessModes: []string{"ReadWriteOnce"},
			},
			path: "config.workspace.storage_class",
		},
		{workspace: WorkspaceConfig{Mode: WorkspaceModeEmptyDir, ClaimName: "foreign"}, path: "config.workspace.claim_name"},
		{workspace: WorkspaceConfig{Mode: WorkspaceModeExistingClaim}, path: "config.workspace.claim_name"},
	}

	for _, tt := range tests {
		cfg := ProfileConfig{Platform: PlatformLinuxARM64, MainContainer: "kandev-agent", PodTemplateYAML: validPodTemplate(""), Workspace: tt.workspace}
		err := cfg.Validate()
		var fieldErr *FieldError
		if !errors.As(err, &fieldErr) || fieldErr.Path != tt.path {
			t.Fatalf("Validate(%s) error = %v, want field %q", tt.workspace.Mode, err, tt.path)
		}
	}
}

func TestParseProfileConfigBuildsTypedWorkspaceConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		values    map[string]string
		workspace WorkspaceConfig
	}{
		{
			name: "managed pvc",
			values: map[string]string{
				"workspace_mode": "managed_pvc", "workspace_size": "20Gi",
				"workspace_storage_class": "fast", "workspace_access_modes": `["ReadWriteOnce","ReadWriteMany"]`,
			},
			workspace: WorkspaceConfig{Mode: WorkspaceModeManagedPVC, Size: "20Gi", StorageClass: "fast", AccessModes: []string{"ReadWriteOnce", "ReadWriteMany"}},
		},
		{name: "empty dir", values: map[string]string{"workspace_mode": "empty_dir"}, workspace: WorkspaceConfig{Mode: WorkspaceModeEmptyDir}},
		{name: "existing claim", values: map[string]string{"workspace_mode": "existing_claim", "workspace_claim_name": "shared-pvc"}, workspace: WorkspaceConfig{Mode: WorkspaceModeExistingClaim, ClaimName: "shared-pvc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.values["platform"] = "linux/arm64"
			tt.values["pod_template_yaml"] = validPodTemplate("")
			got, err := ParseProfileConfig(tt.values)
			if err != nil {
				t.Fatalf("ParseProfileConfig() error = %v", err)
			}
			if got.Platform != PlatformLinuxARM64 || got.MainContainer != "kandev-agent" || !reflect.DeepEqual(got.Workspace, tt.workspace) {
				t.Fatalf("ParseProfileConfig() = %#v, want platform/main/workspace %#v", got, tt.workspace)
			}
		})
	}
}

func TestParseProfileConfigRejectsMalformedAccessModes(t *testing.T) {
	t.Parallel()

	_, err := ParseProfileConfig(map[string]string{
		"platform": "linux/amd64", "pod_template_yaml": validPodTemplate(""),
		"workspace_mode": "managed_pvc", "workspace_size": "10Gi", "workspace_access_modes": "ReadWriteOnce",
	})
	assertFieldPath(t, err, "config.workspace.access_modes")
}
