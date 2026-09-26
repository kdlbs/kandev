package backendapp

import (
	"testing"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
)

func TestScopedDockerLabelsAddsOnlyTheE2EScope(t *testing.T) {
	t.Setenv("KANDEV_E2E_DOCKER_SCOPE", "e2e-test-scope")
	input := map[string]string{"kandev.managed": "true"}

	got := scopedDockerLabels(input)

	if got["kandev.e2e.run"] != "e2e-test-scope" {
		t.Fatalf("scope label = %q, want e2e-test-scope", got["kandev.e2e.run"])
	}
	if got["kandev.managed"] != "true" {
		t.Fatalf("managed label = %q, want true", got["kandev.managed"])
	}
	if _, ok := input["kandev.e2e.run"]; ok {
		t.Fatal("scoping must not mutate the caller's label map")
	}
}

func TestScopeDockerUsageFiltersOnlyContainerRecords(t *testing.T) {
	t.Setenv("KANDEV_E2E_DOCKER_SCOPE", "e2e-test-scope")
	usage := agentdocker.DiskUsage{
		ImageLayerBytes: 10,
		Images:          []agentdocker.ImageUsage{{ID: "image"}},
		Containers: []agentdocker.ContainerUsage{
			{ID: "owned", Labels: map[string]string{"kandev.e2e.run": "e2e-test-scope"}},
			{ID: "other", Labels: map[string]string{"kandev.e2e.run": "other-scope"}},
		},
	}

	scoped := scopeDockerUsage(usage)

	if len(scoped.Containers) != 1 || scoped.Containers[0].ID != "owned" {
		t.Fatalf("scoped containers = %#v, want only owned record", scoped.Containers)
	}
	if len(scoped.Images) != 1 || scoped.ImageLayerBytes != 10 {
		t.Fatalf("image usage changed while scoping containers: %#v", scoped)
	}
}
