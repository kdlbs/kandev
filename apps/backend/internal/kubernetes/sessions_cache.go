package kubernetes

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
)

type environmentStatusResult struct {
	record *models.KubernetesEnvironment
	err    error
}
type podStatusResult struct {
	pod *corev1.Pod
	err error
}

// A request projects one observed physical state onto every authorized session.
// Identity and task authorization remain checked separately for every row.
type sessionStatusCache struct {
	client       kubeclient.Interface
	environments map[string]environmentStatusResult
	pods         map[string]podStatusResult
}

func newSessionStatusCache(client kubeclient.Interface) *sessionStatusCache {
	return &sessionStatusCache{client: client, environments: make(map[string]environmentStatusResult), pods: make(map[string]podStatusResult)}
}
func (c *sessionStatusCache) pod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	key := namespace + "/" + name
	result, exists := c.pods[key]
	if !exists {
		result.pod, result.err = c.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		c.pods[key] = result
	}
	return result.pod, result.err
}
