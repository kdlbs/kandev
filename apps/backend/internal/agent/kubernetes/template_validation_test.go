package kubernetes

import (
	"errors"
	"testing"
)

func TestProfileConfigValidateRejectsInvalidPodTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		path string
	}{
		{
			name: "unknown field",
			yaml: validPodTemplate("    unknownField: true\n"),
			path: "config.pod_template_yaml",
		},
		{
			name: "multiple documents",
			yaml: validPodTemplate("") + "---\napiVersion: v1\nkind: PodTemplate\ntemplate: {}\n",
			path: "config.pod_template_yaml",
		},
		{
			name: "wrong kind",
			yaml: "apiVersion: v1\nkind: Pod\nmetadata: {}\nspec: {}\n",
			path: "config.pod_template_yaml",
		},
		{
			name: "missing main image",
			yaml: "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: kandev-agent\n",
			path: "config.pod_template_yaml.template.spec.containers[0].image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validProfile(tt.yaml).Validate()
			assertFieldPath(t, err, tt.path)
		})
	}
}

func TestProfileConfigValidateRejectsReservedPodFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		path string
	}{
		{
			name: "pod name",
			yaml: "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  metadata:\n    name: foreign\n  spec:\n    containers:\n      - name: kandev-agent\n        image: example/agent:latest\n",
			path: "config.pod_template_yaml.template.metadata.name",
		},
		{
			name: "identity label",
			yaml: "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  metadata:\n    labels:\n      kandev.ai/session-id: foreign\n  spec:\n    containers:\n      - name: kandev-agent\n        image: example/agent:latest\n",
			path: "config.pod_template_yaml.template.metadata.labels.kandev.ai/session-id",
		},
		{
			name: "restart policy",
			yaml: validPodTemplate("    restartPolicy: Never\n"),
			path: "config.pod_template_yaml.template.spec.restartPolicy",
		},
		{
			name: "architecture selector",
			yaml: validPodTemplate("    nodeSelector:\n      kubernetes.io/arch: arm64\n"),
			path: "config.pod_template_yaml.template.spec.nodeSelector.kubernetes.io/arch",
		},
		{
			name: "main command",
			yaml: validMainContainerTemplate("        command: [\"sleep\"]\n"),
			path: "config.pod_template_yaml.template.spec.containers[0].command",
		},
		{
			name: "reserved env",
			yaml: validMainContainerTemplate("        env:\n          - name: AGENTCTL_PORT\n            value: \"9999\"\n"),
			path: "config.pod_template_yaml.template.spec.containers[0].env[0].name",
		},
		{
			name: "reserved future agentctl env",
			yaml: validMainContainerTemplate("        env:\n          - name: AGENTCTL_FUTURE_CONTROL\n            value: foreign\n"),
			path: "config.pod_template_yaml.template.spec.containers[0].env[0].name",
		},
		{
			name: "reserved future kandev env",
			yaml: validMainContainerTemplate("        env:\n          - name: KANDEV_FUTURE_CONTROL\n            value: foreign\n"),
			path: "config.pod_template_yaml.template.spec.containers[0].env[0].name",
		},
		{
			name: "reserved home env",
			yaml: validMainContainerTemplate("        env:\n          - name: HOME\n            value: /foreign\n"),
			path: "config.pod_template_yaml.template.spec.containers[0].env[0].name",
		},
		{
			name: "reserved volume",
			yaml: validPodTemplate("    volumes:\n      - name: kandev-runtime\n        emptyDir: {}\n"),
			path: "config.pod_template_yaml.template.spec.volumes[0].name",
		},
		{
			name: "reserved runtime descendant mount",
			yaml: validMainContainerTemplate("        volumeMounts:\n          - name: foreign\n            mountPath: /opt/kandev/agentctl\n"),
			path: "config.pod_template_yaml.template.spec.containers[0].volumeMounts[0]",
		},
		{
			name: "reserved auth descendant mount",
			yaml: validMainContainerTemplate("        volumeMounts:\n          - name: foreign\n            mountPath: /run/kandev/home\n"),
			path: "config.pod_template_yaml.template.spec.containers[0].volumeMounts[0]",
		},
		{
			name: "reserved workspace descendant mount",
			yaml: validMainContainerTemplate("        volumeMounts:\n          - name: foreign\n            mountPath: /workspace/.git\n"),
			path: "config.pod_template_yaml.template.spec.containers[0].volumeMounts[0]",
		},
		{
			name: "reserved runtime descendant device",
			yaml: validMainContainerTemplate("        volumeDevices:\n          - name: foreign\n            devicePath: /opt/kandev/device\n"),
			path: "config.pod_template_yaml.template.spec.containers[0].volumeDevices[0].devicePath",
		},
		{
			name: "reserved architecture match field",
			yaml: validPodTemplate("    affinity:\n      nodeAffinity:\n        requiredDuringSchedulingIgnoredDuringExecution:\n          nodeSelectorTerms:\n            - matchFields:\n                - key: kubernetes.io/arch\n                  operator: In\n                  values: [amd64]\n"),
			path: "config.pod_template_yaml.template.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchFields[0].key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertFieldPath(t, validProfile(tt.yaml).Validate(), tt.path)
		})
	}
}

func TestProfileConfigValidateRejectsEnvFromForEveryContainerKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		path string
	}{
		{
			name: "main container",
			yaml: validMainContainerTemplate("        envFrom:\n          - configMapRef:\n              name: app-env\n"),
			path: "config.pod_template_yaml.template.spec.containers[0].envFrom",
		},
		{
			name: "sidecar",
			yaml: "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: kandev-agent\n        image: example/agent:latest\n      - name: sidecar\n        image: example/sidecar:latest\n        envFrom:\n          - secretRef:\n              name: sidecar-env\n",
			path: "config.pod_template_yaml.template.spec.containers[1].envFrom",
		},
		{
			name: "init container",
			yaml: "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    initContainers:\n      - name: init\n        image: example/init:latest\n        envFrom:\n          - configMapRef:\n              name: init-env\n    containers:\n      - name: kandev-agent\n        image: example/agent:latest\n",
			path: "config.pod_template_yaml.template.spec.initContainers[0].envFrom",
		},
		{
			name: "ephemeral container",
			yaml: "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: kandev-agent\n        image: example/agent:latest\n    ephemeralContainers:\n      - name: debug\n        image: example/debug:latest\n        envFrom:\n          - secretRef:\n              name: debug-env\n",
			path: "config.pod_template_yaml.template.spec.ephemeralContainers[0].envFrom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertFieldPath(t, validProfile(tt.yaml).Validate(), tt.path)
		})
	}
}

func validProfile(template string) ProfileConfig {
	return ProfileConfig{
		Platform: PlatformLinuxAMD64, MainContainer: "kandev-agent",
		PodTemplateYAML: template, Workspace: WorkspaceConfig{Mode: WorkspaceModeEmptyDir},
	}
}

func validPodTemplate(extra string) string {
	return "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n" + extra +
		"    containers:\n      - name: kandev-agent\n        image: example/agent:latest\n"
}

func validMainContainerTemplate(extra string) string {
	return "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n" +
		"    containers:\n      - name: kandev-agent\n        image: example/agent:latest\n" + extra
}

func assertFieldPath(t *testing.T, err error, want string) {
	t.Helper()
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("error = %v, want FieldError at %q", err, want)
	}
	if fieldErr.Path != want {
		t.Fatalf("error path = %q, want %q", fieldErr.Path, want)
	}
}
