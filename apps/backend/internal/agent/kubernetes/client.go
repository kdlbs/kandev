package kubernetes

import (
	"errors"
	"time"

	kubeclient "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

const kubernetesUserAgent = "kandev-kubernetes-executor"

type ConfigLoader struct {
	Kubeconfig func(path, contextName string) (*rest.Config, error)
	InCluster  func() (*rest.Config, error)
}

type ClientsetFactory func(*rest.Config) (kubeclient.Interface, error)

type Client struct {
	RESTConfig *rest.Config
	Clientset  kubeclient.Interface
}

func DefaultConfigLoader() ConfigLoader {
	return ConfigLoader{
		Kubeconfig: loadKubeconfig,
		InCluster:  rest.InClusterConfig,
	}
}

func NewClient(config ExecutorConfig, loader ConfigLoader, factory ClientsetFactory) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	loader = withDefaultConfigLoader(loader)
	restConfig, err := loadRESTConfig(config, loader)
	if err != nil {
		return nil, sanitizeClientError("load client configuration", err)
	}
	if restConfig == nil {
		return nil, sanitizeClientError("load client configuration", errors.New("loader returned nil config"))
	}
	configured := rest.CopyConfig(restConfig)
	configured.Timeout = time.Duration(config.RequestTimeoutSeconds) * time.Second
	configured.UserAgent = kubernetesUserAgent
	if factory == nil {
		factory = func(config *rest.Config) (kubeclient.Interface, error) {
			return kubeclient.NewForConfig(config)
		}
	}
	clientset, err := factory(configured)
	if err != nil {
		return nil, sanitizeClientError("create client", err)
	}
	return &Client{RESTConfig: configured, Clientset: clientset}, nil
}

func loadKubeconfig(path, contextName string) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = path
	overrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
}

func withDefaultConfigLoader(loader ConfigLoader) ConfigLoader {
	defaults := DefaultConfigLoader()
	if loader.Kubeconfig == nil {
		loader.Kubeconfig = defaults.Kubeconfig
	}
	if loader.InCluster == nil {
		loader.InCluster = defaults.InCluster
	}
	return loader
}

func loadRESTConfig(config ExecutorConfig, loader ConfigLoader) (*rest.Config, error) {
	if config.AuthMode == AuthModeInCluster {
		return loader.InCluster()
	}
	return loader.Kubeconfig(config.KubeconfigPath, config.KubeContext)
}

type clientError struct {
	operation string
	cause     error
}

func (e *clientError) Error() string {
	return "kubernetes " + e.operation + " failed: " + routingerr.Sanitize(e.cause.Error())
}
func (e *clientError) Unwrap() error { return e.cause }

func sanitizeClientError(operation string, cause error) error {
	return &clientError{operation: operation, cause: cause}
}
