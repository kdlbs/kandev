package service

import "testing"

func TestInjectKandevCLIKubernetesUsesRuntimeVolumeBinary(t *testing.T) {
	integration := &SchedulerIntegration{}
	env := map[string]string{"KANDEV_CLI": "/host/bin/agentctl"}

	integration.injectKandevCLI(env, "k8s")

	if got := env["KANDEV_CLI"]; got != "/opt/kandev/agentctl" {
		t.Fatalf("KANDEV_CLI = %q, want Kubernetes runtime binary", got)
	}
}

func TestInstructionsDirForKubernetesUsesRuntimeVolume(t *testing.T) {
	got := instructionsDirForExecutor("/host/kandev", "workspace", "agent", "k8s")
	want := "/opt/kandev/runtime/workspace/instructions/agent"
	if got != want {
		t.Fatalf("instructions dir = %q, want %q", got, want)
	}
}

func TestInstructionsDirForUnknownExecutorFailsClosed(t *testing.T) {
	if got := instructionsDirForExecutor("/host/kandev", "workspace", "agent", "future_executor"); got != "" {
		t.Fatalf("instructions dir = %q, want empty path for unknown executor", got)
	}
}
