package kubernetes

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFullWorkerTemplate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("../../../../..", "k8s/worker-images/full/pod-template.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	template, err := ParsePodTemplate(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	pod, _, err := ComposePod(template, validProfile(""), podOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAdmittedPod(pod.DeepCopy(), pod, DefaultMainContainerName); err != nil {
		t.Fatal(err)
	}
	if len(pod.Spec.Containers) != 2 {
		t.Fatal("expected main and per-Pod daemon")
	}
	main, daemon := pod.Spec.Containers[0], pod.Spec.Containers[1]
	if main.SecurityContext == nil || main.SecurityContext.RunAsUser == nil || *main.SecurityContext.RunAsUser != 1000 {
		t.Fatal("main must remain non-root")
	}
	if daemon.SecurityContext == nil || daemon.SecurityContext.Privileged == nil || !*daemon.SecurityContext.Privileged {
		t.Fatal("daemon privilege must be explicit")
	}
	if len(daemon.Command) == 0 || daemon.Command[0] != "/bin/sh" || strings.Contains(strings.Join(daemon.Args, " "), "tcp://") {
		t.Fatal("daemon must bypass inherited TCP entrypoint")
	}
	if pod.Spec.HostNetwork || pod.Spec.HostPID || pod.Spec.HostIPC {
		t.Fatal("host namespaces enabled")
	}
	for _, v := range pod.Spec.Volumes {
		if v.HostPath != nil {
			t.Fatal("hostPath enabled")
		}
	}
	if *pod.Spec.AutomountServiceAccountToken {
		t.Fatal("service-account token enabled")
	}
}

func TestFullWorkerTemplateRequiresImmutableImage(t *testing.T) {
	script := "../../../../../k8s/worker-images/full/render-template.sh"
	for _, image := range []string{"", "FULL_WORKER_IMAGE_REQUIRED", "example/worker:latest"} {
		if output, err := exec.Command("bash", script, image).CombinedOutput(); err == nil {
			t.Fatalf("accepted mutable image: %s", output)
		}
	}
	image := "example/worker@sha256:" + strings.Repeat("a", 64)
	output, err := exec.Command("bash", script, image).CombinedOutput()
	if err != nil {
		t.Fatalf("render failed: %v %s", err, output)
	}
	if !strings.Contains(string(output), image) || strings.Contains(string(output), "FULL_WORKER_IMAGE_REQUIRED") {
		t.Fatal("image substitution failed")
	}
}
