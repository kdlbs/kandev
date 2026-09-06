package scriptengine

import (
	"slices"
	"testing"
)

func TestKubernetesAdvertisesDockerCompatiblePlaceholders(t *testing.T) {
	for _, placeholder := range DefaultPlaceholders {
		dockerCompatible := slices.Contains(placeholder.ExecutorTypes, "local_docker")
		kubernetesCompatible := slices.Contains(placeholder.ExecutorTypes, "k8s")
		if dockerCompatible != kubernetesCompatible {
			t.Errorf(
				"placeholder %q: local_docker compatibility = %v, k8s compatibility = %v",
				placeholder.Key,
				dockerCompatible,
				kubernetesCompatible,
			)
		}
	}
}
