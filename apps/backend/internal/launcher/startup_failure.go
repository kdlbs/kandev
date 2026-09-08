package launcher

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/config"
)

const startupTroubleshootingURL = "https://github.com/kdlbs/kandev/blob/main/docs/public/cli.md#startup-health-check-times-out"

func formatStartupFailure(err error, endpoints backendEndpointSet, cfg *config.Config, backendLogPath string, backendStopped bool) string {
	class, observations, healthErr, endpoints := startupFailureEvidence(err, endpoints)

	var output strings.Builder
	writeStartupFailureHeading(&output, class, healthErr)
	writeStartupFailureState(&output, healthErr, backendStopped)
	writeStartupFailureEndpoints(&output, endpoints, observations)
	writeStartupConfiguration(&output, cfg)
	if backendLogPath == "" {
		backendLogPath = backendLogPathForConfig(cfg)
	}
	fmt.Fprintf(&output, "[kandev] backend log: %s\n", backendLogPath)
	fmt.Fprintf(&output, "[kandev] next step: %s\n", startupFailureNextStep(class))
	fmt.Fprintf(&output, "[kandev] troubleshooting: %s\n", startupTroubleshootingURL)
	return strings.TrimSuffix(output.String(), "\n")
}

func startupFailureEvidence(err error, endpoints backendEndpointSet) (healthFailureClass, []healthObservation, *backendHealthError, backendEndpointSet) {
	var healthErr *backendHealthError
	if !errors.As(err, &healthErr) {
		return "", nil, nil, endpoints
	}
	if len(endpoints.healthTargets) == 0 {
		endpoints = healthErr.EndpointSet
	}
	return healthErr.Class, healthErr.Observations, healthErr, endpoints
}

func writeStartupFailureHeading(output *strings.Builder, class healthFailureClass, healthErr *backendHealthError) {
	fmt.Fprintf(output, "[kandev] startup failed: %s", startupFailureLabel(class))
	if healthErr != nil {
		fmt.Fprintf(output, " - %s", healthErr.Error())
	}
	output.WriteByte('\n')
}

func writeStartupFailureState(output *strings.Builder, healthErr *backendHealthError, backendStopped bool) {
	switch {
	case healthErr != nil && healthErr.ChildExited:
		fmt.Fprintf(output, "[kandev] backend state: exited before readiness (code %d)\n", healthErr.ChildExitCode)
	case backendStopped:
		output.WriteString("[kandev] backend state: launcher stopped the backend after readiness failed\n")
	default:
		output.WriteString("[kandev] backend state: readiness did not complete\n")
	}
}

func writeStartupFailureEndpoints(output *strings.Builder, endpoints backendEndpointSet, observations []healthObservation) {
	binds := strings.Join(endpoints.bindHosts, ", ")
	if binds == "" {
		binds = "unknown"
	}
	fmt.Fprintf(output, "[kandev] effective bind addresses: %s\n", binds)
	output.WriteString("[kandev] health targets:\n")
	observationsByURL := make(map[string]healthObservation, len(observations))
	for _, observation := range observations {
		observationsByURL[observation.URL] = observation
	}
	for _, target := range endpoints.healthTargets {
		writeStartupTarget(output, target, observationsByURL)
	}
}

func writeStartupTarget(output *strings.Builder, target string, observations map[string]healthObservation) {
	observation, ok := observations[target]
	if !ok {
		fmt.Fprintf(output, "[kandev]   %s: not_attempted\n", target)
		return
	}
	detail := observation.SafeDetail
	if observation.StatusCode != 0 {
		if detail != "" {
			detail += ", "
		}
		detail += fmt.Sprintf("status=%d", observation.StatusCode)
	}
	if detail != "" {
		detail = " (" + detail + ")"
	}
	fmt.Fprintf(output, "[kandev]   %s: %s%s\n", target, observation.Outcome, detail)
}

func writeStartupConfiguration(output *strings.Builder, cfg *config.Config) {
	file := "none (defaults and environment)"
	if cfg != nil && strings.TrimSpace(cfg.Source.FilePath) != "" {
		file = cfg.Source.FilePath
	}
	fmt.Fprintf(output, "[kandev] configuration file: %s\n", file)
	fmt.Fprintf(output, "[kandev] server.host source: %s\n", startupServerHostSource(cfg))
}

func startupServerHostSource(cfg *config.Config) string {
	if cfg == nil {
		return "built-in default (0.0.0.0)"
	}
	switch cfg.SourceFor("server.host") {
	case config.SourceEnvironment:
		return "environment (KANDEV_SERVER_HOST)"
	case config.SourceConfiguration:
		return "configuration file"
	case config.SourceProfile:
		return "profile defaults"
	case config.SourceDefault:
		return "built-in default (0.0.0.0)"
	default:
		return string(cfg.SourceFor("server.host"))
	}
}

func startupFailureLabel(class healthFailureClass) string {
	switch class {
	case healthFailureEarlyExit:
		return "early backend exit"
	case healthFailureUnreachable:
		return "unreachable backend"
	case healthFailureUnhealthyHTTP:
		return "unhealthy HTTP response"
	case healthFailureForeign:
		return "foreign process"
	case healthFailureCanceled:
		return "canceled readiness check"
	default:
		return "unknown readiness failure"
	}
}

func startupFailureNextStep(class healthFailureClass) string {
	switch class {
	case healthFailureEarlyExit:
		return "read the backend log for the startup error"
	case healthFailureForeign:
		return "free the selected port or choose another backend port"
	case healthFailureUnhealthyHTTP:
		return "inspect the reported HTTP status and the backend log"
	case healthFailureUnreachable:
		return "inspect bind addresses, firewall rules, and environment overrides"
	default:
		return "read the backend log and follow the startup troubleshooting guide"
	}
}

func backendLogPathForConfig(cfg *config.Config) string {
	return filepath.Join(resolveHomeDirForConfig(cfg), "logs", "backend-logs.log")
}

func backendLogPathForDevConfig(cfg devLaunchConfig) string {
	if homeDir := strings.TrimSpace(processEnvValue(cfg.extra, "KANDEV_HOME_DIR")); homeDir != "" {
		return filepath.Join(homeDir, "logs", "backend-logs.log")
	}
	return backendLogPathForConfig(cfg.startup)
}

func startupFailureStoppedBackend(err error) bool {
	var healthErr *backendHealthError
	return !errors.As(err, &healthErr) || !healthErr.ChildExited
}
