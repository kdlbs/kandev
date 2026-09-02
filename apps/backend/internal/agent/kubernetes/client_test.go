package kubernetes

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestNewClientLoadsKubeconfigLazilyAndCopiesRESTConfig(t *testing.T) {
	t.Parallel()

	source := &rest.Config{Host: "https://cluster.example", BearerToken: "secret-token"}
	var gotPath, gotContext string
	loader := ConfigLoader{
		Kubeconfig: func(path, contextName string) (*rest.Config, error) {
			gotPath, gotContext = path, contextName
			return source, nil
		},
		InCluster: func() (*rest.Config, error) {
			t.Fatal("in-cluster loader called for kubeconfig auth")
			return nil, nil
		},
	}
	var factoryConfig *rest.Config
	factory := func(config *rest.Config) (kubeclient.Interface, error) {
		factoryConfig = config
		return fake.NewSimpleClientset(), nil
	}
	cfg := ExecutorConfig{
		AuthMode: AuthModeKubeconfig, KubeconfigPath: "/etc/kandev/config",
		KubeContext: "production", Namespace: "agents", RequestTimeoutSeconds: 45,
	}

	client, err := NewClient(cfg, loader, factory)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if gotPath != cfg.KubeconfigPath || gotContext != cfg.KubeContext {
		t.Fatalf("loader args = %q/%q", gotPath, gotContext)
	}
	if client.RESTConfig == source || factoryConfig != client.RESTConfig {
		t.Fatal("NewClient() did not use one defensive REST config copy")
	}
	if client.RESTConfig.Timeout != 45*time.Second || source.Timeout != 0 {
		t.Fatalf("timeouts = client %s, source %s", client.RESTConfig.Timeout, source.Timeout)
	}
	if client.RESTConfig.UserAgent != "kandev-kubernetes-executor" {
		t.Fatalf("user agent = %q", client.RESTConfig.UserAgent)
	}
}

func TestNewClientUsesInClusterLoader(t *testing.T) {
	t.Parallel()

	called := false
	loader := ConfigLoader{
		Kubeconfig: func(string, string) (*rest.Config, error) {
			t.Fatal("kubeconfig loader called for in-cluster auth")
			return nil, nil
		},
		InCluster: func() (*rest.Config, error) {
			called = true
			return &rest.Config{Host: "https://kubernetes.default.svc"}, nil
		},
	}
	_, err := NewClient(
		ExecutorConfig{AuthMode: AuthModeInCluster, Namespace: "agents", RequestTimeoutSeconds: 30},
		loader,
		func(*rest.Config) (kubeclient.Interface, error) { return fake.NewSimpleClientset(), nil },
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if !called {
		t.Fatal("in-cluster loader was not called")
	}
}

func TestNewClientRedactsLoaderErrorsAndPreservesCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("permission denied by https://cluster.example/private?token=kandev_pat_super-secret-value")
	_, err := NewClient(
		ExecutorConfig{AuthMode: AuthModeInCluster, Namespace: "agents", RequestTimeoutSeconds: 30},
		ConfigLoader{InCluster: func() (*rest.Config, error) { return nil, cause }},
		nil,
	)
	if !errors.Is(err, cause) {
		t.Fatalf("NewClient() error = %v, want wrapped cause", err)
	}
	if strings.Contains(err.Error(), "super-secret-value") {
		t.Fatalf("NewClient() exposed credential: %q", err)
	}
	if !strings.Contains(err.Error(), "permission denied") || !strings.Contains(err.Error(), "https://cluster.example") {
		t.Fatalf("NewClient() discarded safe causal diagnostics: %q", err)
	}
	if strings.Contains(err.Error(), "/private") {
		t.Fatalf("NewClient() exposed URL path: %q", err)
	}
}

func TestDefaultConfigLoaderHonorsExplicitContext(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config")
	contents := []byte(`apiVersion: v1
kind: Config
clusters:
  - name: first
    cluster:
      server: https://first.example
  - name: second
    cluster:
      server: https://second.example
contexts:
  - name: first
    context:
      cluster: first
      user: user
  - name: second
    context:
      cluster: second
      user: user
current-context: first
users:
  - name: user
    user:
      token: fixture-token
`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := DefaultConfigLoader().Kubeconfig(path, "second")
	if err != nil {
		t.Fatalf("Kubeconfig() error = %v", err)
	}
	if config.Host != "https://second.example" || config.BearerToken != "fixture-token" {
		t.Fatalf("REST config = host %q token %q", config.Host, config.BearerToken)
	}
}

func TestDefaultConfigLoaderSupportsOIDCAuthProvider(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config")
	contents := []byte(`apiVersion: v1
kind: Config
clusters:
  - name: cluster
    cluster:
      server: https://cluster.example
contexts:
  - name: oidc
    context:
      cluster: cluster
      user: oidc
current-context: oidc
users:
  - name: oidc
    user:
      auth-provider:
        name: oidc
        config:
          idp-issuer-url: https://issuer.example
          client-id: kandev
`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := DefaultConfigLoader().Kubeconfig(path, "oidc")
	if err != nil {
		t.Fatalf("Kubeconfig() error = %v", err)
	}
	if _, err := rest.TransportFor(config); err != nil {
		t.Fatalf("TransportFor() error = %v", err)
	}
}
