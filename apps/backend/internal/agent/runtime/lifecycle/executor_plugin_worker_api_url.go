package lifecycle

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

const (
	pluginExecutorCapabilityWorkerAPIURL = "worker_api_url"
	loopbackHostName                     = "localhost"
)

// ErrWorkerAPIURLInvalid marks a provider-supplied worker API URL the host
// refuses to probe.
var ErrWorkerAPIURLInvalid = errors.New("provider worker API URL is invalid")

// probeBaseURL is the base URL the worker is asked to request: the worker-side
// address the provider reported, or the host's own URL when it reported none.
func (l *pluginExecutorLaunchState) probeBaseURL() string {
	if l.workerAPIURL != "" {
		return l.workerAPIURL
	}
	return l.hostAPIURL
}

// checkProvisionedDescriptor validates the resource descriptor of a provision
// response and returns the rollback stage of the first failing check.
func (l *pluginExecutorLaunchState) checkProvisionedDescriptor(resource *pluginsdk.ExecutorResourceDescriptor) (string, error) {
	if err := validatePluginExecutorResourceForProvider(l.profile.Provider, resource); err != nil {
		return "resource_descriptor", err
	}
	if err := l.adoptWorkerAPIURL(resource); err != nil {
		return "worker_api_url", err
	}
	return "", nil
}

// adoptWorkerAPIURL records the validated worker API URL of a provision response.
func (l *pluginExecutorLaunchState) adoptWorkerAPIURL(resource *pluginsdk.ExecutorResourceDescriptor) error {
	workerAPIURL, err := resolvePluginExecutorWorkerAPIURL(l.inventory.NegotiatedCapabilities, l.hostAPIURL != "", resource)
	if err != nil {
		return err
	}
	l.workerAPIURL = workerAPIURL
	return nil
}

// resolvePluginExecutorWorkerAPIURL validates the worker API URL of a
// provision response. A provider that did not declare the capability must not
// return one, and a launch that is not provider_forwarded has nothing to probe.
func resolvePluginExecutorWorkerAPIURL(negotiated []string, forwarded bool, resource *pluginsdk.ExecutorResourceDescriptor) (string, error) {
	raw := resource.GetWorkerApiUrl()
	if raw == "" {
		return "", nil
	}
	if !negotiatedDeclares(negotiated, pluginExecutorCapabilityWorkerAPIURL) {
		return "", fmt.Errorf("%w: provider returned a worker API URL without declaring the capability", models.ErrExecutorConflict)
	}
	if !forwarded {
		return "", fmt.Errorf("%w: only a provider_forwarded launch takes a worker API URL", ErrWorkerAPIURLInvalid)
	}
	return validatePluginWorkerAPIURL(raw)
}

// validatePluginWorkerAPIURL accepts only an absolute http base URL whose host
// is a loopback name and which carries a port and nothing else. The value is
// returned without a trailing slash; the probe path is appended by the caller.
func validatePluginWorkerAPIURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Opaque != "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: must be an absolute http URL", ErrWorkerAPIURLInvalid)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%w: must not carry credentials", ErrWorkerAPIURLInvalid)
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: must be a base URL without a path, query or fragment", ErrWorkerAPIURLInvalid)
	}
	if !isLoopbackWorkerHost(parsed.Hostname()) {
		return "", fmt.Errorf("%w: host must be loopback", ErrWorkerAPIURLInvalid)
	}
	if port, err := strconv.Atoi(parsed.Port()); err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("%w: must carry a port", ErrWorkerAPIURLInvalid)
	}
	return "http://" + parsed.Host, nil
}

func isLoopbackWorkerHost(host string) bool {
	if host == loopbackHostName {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
