package launcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/config"
)

const (
	healthPollInterval = 300 * time.Millisecond
	healthProbeTimeout = 2 * time.Second
)

// healthProbeClient bounds every individual health request. http.DefaultClient
// has no timeout, so a backend that accepts the connection but never answers
// would otherwise block the launcher forever. Launcher readiness is local to
// the process, so probes must not be redirected through an inherited proxy.
var healthProbeClient = newHealthProbeClient()

func newHealthProbeClient() *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if ok {
		transport = transport.Clone()
	} else {
		transport = &http.Transport{}
	}
	transport.Proxy = nil
	return &http.Client{Timeout: healthProbeTimeout, Transport: transport}
}

type childState interface {
	Exited() (bool, int)
}

type healthOutcome string

const (
	healthOutcomeHealthy         healthOutcome = "healthy"
	healthOutcomeConnectionError healthOutcome = "connection_error"
	healthOutcomeHTTPStatus      healthOutcome = "http_status"
	healthOutcomeForeignProcess  healthOutcome = "foreign_process"
)

type healthObservation struct {
	URL        string
	Outcome    healthOutcome
	StatusCode int
	SafeDetail string
}

type healthFailureClass string

const (
	healthFailureEarlyExit     healthFailureClass = "early_exit"
	healthFailureUnreachable   healthFailureClass = "unreachable_backend"
	healthFailureUnhealthyHTTP healthFailureClass = "unhealthy_http"
	healthFailureForeign       healthFailureClass = "foreign_process"
	healthFailureCanceled      healthFailureClass = "canceled"
)

type backendHealthError struct {
	Class         healthFailureClass
	EndpointSet   backendEndpointSet
	Observations  []healthObservation
	Timeout       time.Duration
	ChildExited   bool
	ChildExitCode int
	Cause         error
}

func (e *backendHealthError) Error() string {
	switch e.Class {
	case healthFailureEarlyExit:
		return fmt.Sprintf("backend exited (code %d) before healthcheck passed", e.ChildExitCode)
	case healthFailureCanceled:
		if e.EndpointSet.accessURL != "" {
			return fmt.Sprintf("backend healthcheck canceled at %s: %v", e.EndpointSet.accessURL, e.Cause)
		}
		return fmt.Sprintf("backend healthcheck canceled: %v", e.Cause)
	case healthFailureForeign:
		for _, observation := range e.Observations {
			if observation.Outcome == healthOutcomeForeignProcess {
				return fmt.Sprintf(
					"backend port %s answered a health check from a different process "+
						"(missing/mismatched launcher token). Another Kandev instance may already own it, "+
						"or the runtime bundle predates v0.66.0",
					healthPort(observation.URL),
				)
			}
		}
	case healthFailureUnhealthyHTTP:
		for _, observation := range e.Observations {
			if observation.Outcome == healthOutcomeHTTPStatus {
				return fmt.Sprintf("backend healthcheck timed out after %s at %s (HTTP status %d)",
					e.Timeout, observation.URL, observation.StatusCode)
			}
		}
	}
	target := e.EndpointSet.accessURL
	if target == "" && len(e.EndpointSet.healthTargets) > 0 {
		target = e.EndpointSet.healthTargets[0]
	}
	return fmt.Sprintf("backend healthcheck timed out after %s at %s", e.Timeout, target)
}

func (e *backendHealthError) Unwrap() error {
	return e.Cause
}

func healthTimeout(defaultMS int) time.Duration {
	raw := os.Getenv("KANDEV_HEALTH_TIMEOUT_MS")
	if raw == "" {
		return time.Duration(defaultMS) * time.Millisecond
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return time.Duration(defaultMS) * time.Millisecond
	}
	return time.Duration(n) * time.Millisecond
}

func healthTimeoutForConfig(defaultMS int, cfg *config.Config) time.Duration {
	if raw, ok := os.LookupEnv("KANDEV_HEALTH_TIMEOUT_MS"); ok && strings.TrimSpace(raw) != "" {
		return healthTimeout(defaultMS)
	}
	if configSourceIsExplicit(cfg, "launcher.healthTimeoutMs") || cfg != nil {
		if cfg != nil && cfg.Launcher.HealthTimeoutMs > 0 {
			return time.Duration(cfg.Launcher.HealthTimeoutMs) * time.Millisecond
		}
	}
	return healthTimeout(defaultMS)
}

func waitForHealth(ctx context.Context, endpoints backendEndpointSet, proc childState, timeout time.Duration, expectedToken string, onFailure func()) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if len(endpoints.healthTargets) == 0 && endpoints.accessURL != "" {
		endpoints.healthTargets = []string{strings.TrimRight(endpoints.accessURL, "/") + "/health"}
	}
	if len(endpoints.healthTargets) == 0 {
		err := &backendHealthError{
			Class:       healthFailureUnreachable,
			EndpointSet: endpoints,
			Timeout:     timeout,
			Cause:       errors.New("no health targets configured"),
		}
		return "", finishHealthFailure(err, onFailure)
	}

	latest := make(map[string]healthObservation, len(endpoints.healthTargets))
	for ctx.Err() == nil {
		if exited, code := proc.Exited(); exited {
			err := &backendHealthError{
				Class:         healthFailureEarlyExit,
				EndpointSet:   endpoints,
				Observations:  observationsInTargetOrder(endpoints.healthTargets, latest),
				Timeout:       timeout,
				ChildExited:   true,
				ChildExitCode: code,
			}
			return "", finishHealthFailure(err, onFailure)
		}

		observations, healthyURL := probeHealthTargets(ctx, endpoints.healthTargets, expectedToken)
		for _, observation := range observations {
			latest[observation.URL] = observation
		}
		if healthyURL != "" {
			return endpoints.browserURLForHealthTarget(healthyURL), nil
		}
		if exited, code := proc.Exited(); exited {
			err := &backendHealthError{
				Class:         healthFailureEarlyExit,
				EndpointSet:   endpoints,
				Observations:  observationsInTargetOrder(endpoints.healthTargets, latest),
				Timeout:       timeout,
				ChildExited:   true,
				ChildExitCode: code,
			}
			return "", finishHealthFailure(err, onFailure)
		}
		select {
		case <-ctx.Done():
		case <-time.After(healthPollInterval):
		}
	}

	observations := observationsInTargetOrder(endpoints.healthTargets, latest)
	if errors.Is(ctx.Err(), context.Canceled) {
		err := &backendHealthError{
			Class:        healthFailureCanceled,
			EndpointSet:  endpoints,
			Observations: observations,
			Timeout:      timeout,
			Cause:        ctx.Err(),
		}
		return "", finishHealthFailure(err, onFailure)
	}
	class := classifyHealthObservations(observations)
	err := &backendHealthError{
		Class:        class,
		EndpointSet:  endpoints,
		Observations: observations,
		Timeout:      timeout,
	}
	return "", finishHealthFailure(err, onFailure)
}

func finishHealthFailure(err *backendHealthError, onFailure func()) error {
	if onFailure != nil {
		onFailure()
	}
	return err
}

type healthProbeResult struct {
	index       int
	observation healthObservation
}

func probeHealthTargets(ctx context.Context, targets []string, expectedToken string) ([]healthObservation, string) {
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan healthProbeResult, len(targets))
	for index, target := range targets {
		go func(index int, target string) {
			results <- healthProbeResult{index: index, observation: probeHealth(probeCtx, target, expectedToken)}
		}(index, target)
	}

	observations := make(map[string]healthObservation, len(targets))
	received := make([]bool, len(targets))
	for completed := 0; completed < len(targets); completed++ {
		select {
		case <-ctx.Done():
			return observationValues(observations), ""
		case result := <-results:
			received[result.index] = true
			observations[result.observation.URL] = result.observation
			if healthyURL := preferredHealthyTarget(targets, observations, received); healthyURL != "" {
				return observationValues(observations), healthyURL
			}
		}
	}
	return observationValues(observations), ""
}

func preferredHealthyTarget(targets []string, observations map[string]healthObservation, received []bool) string {
	for index, target := range targets {
		if !received[index] {
			return ""
		}
		if observations[target].Outcome == healthOutcomeHealthy {
			return target
		}
	}
	return ""
}

func observationValues(observations map[string]healthObservation) []healthObservation {
	values := make([]healthObservation, 0, len(observations))
	for _, observation := range observations {
		values = append(values, observation)
	}
	return values
}

func observationsInTargetOrder(targets []string, observations map[string]healthObservation) []healthObservation {
	ordered := make([]healthObservation, 0, len(observations))
	for _, target := range targets {
		if observation, ok := observations[target]; ok {
			ordered = append(ordered, observation)
		}
	}
	return ordered
}

func classifyHealthObservations(observations []healthObservation) healthFailureClass {
	for _, observation := range observations {
		if observation.Outcome == healthOutcomeForeignProcess {
			return healthFailureForeign
		}
	}
	for _, observation := range observations {
		if observation.Outcome == healthOutcomeHTTPStatus {
			return healthFailureUnhealthyHTTP
		}
	}
	return healthFailureUnreachable
}

// probeHealth reports the latest safe outcome for a single health request. The
// body is drained and closed so the connection can be reused by the next poll.
func probeHealth(ctx context.Context, healthURL, expectedToken string) healthObservation {
	observation := healthObservation{URL: healthURL}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		observation.Outcome = healthOutcomeConnectionError
		observation.SafeDetail = "invalid health target"
		return observation
	}
	resp, err := healthProbeClient.Do(req)
	if err != nil {
		observation.Outcome = healthOutcomeConnectionError
		observation.SafeDetail = safeProbeError(err)
		return observation
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		observation.Outcome = healthOutcomeHTTPStatus
		observation.StatusCode = resp.StatusCode
		observation.SafeDetail = "HTTP response was not successful"
		return observation
	}
	if expectedToken == "" || resp.Header.Get("X-Kandev-Desktop-Health-Token") == expectedToken {
		observation.Outcome = healthOutcomeHealthy
		return observation
	}
	observation.Outcome = healthOutcomeForeignProcess
	observation.SafeDetail = "missing or mismatched launcher token"
	return observation
}

func safeProbeError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	if strings.Contains(strings.ToLower(err.Error()), "connection refused") {
		return "connection refused"
	}
	return "network error"
}

// waitForReady polls GET /ready until the backend reports readiness or the
// process exits. Unlike waitForHealth, it has no timeout of its own and never
// triggers a kill: /health (and its healthTimeoutReleaseMS/healthTimeoutDevMS
// budget) already made the keep-or-kill decision by the time this runs. This
// only decides when the bootstrap handler has handed off to the real router,
// so the launcher can print "backend ready"/open a browser onto real content
// instead of the bootstrap stub's 503 while startup recovery is still
// in flight. Do not fold this into waitForHealth's timeout — that would
// recreate the crash loop docs/specs/startup-listener-before-recovery/spec.md
// exists to fix.
func waitForReady(ctx context.Context, baseURL string, proc childState) error {
	readyURL := baseURL + "/ready"
	for ctx.Err() == nil {
		if exited, code := proc.Exited(); exited {
			return fmt.Errorf("backend exited (code %d) before it reported ready", code)
		}
		if probeReady(ctx, readyURL) {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("backend readiness wait canceled at %s: %w", readyURL, err)
			}
			if exited, code := proc.Exited(); exited {
				return fmt.Errorf("backend exited (code %d) before it reported ready", code)
			}
			return nil
		}
		select {
		case <-ctx.Done():
		case <-time.After(healthPollInterval):
		}
	}
	return fmt.Errorf("backend readiness wait canceled at %s: %w", readyURL, ctx.Err())
}

// probeReady reports whether a single readiness request returned 2xx. Unlike
// probeHealth, it does not check the desktop health token: readyHandler never
// sets X-Kandev-Desktop-Health-Token (that header is /health-only, see
// docs/specs/health-endpoint-version/spec.md AC-21).
func probeReady(ctx context.Context, readyURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
	if err != nil {
		return false
	}
	resp, err := healthProbeClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func healthPort(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err == nil && parsed.Port() != "" {
		return parsed.Port()
	}
	return baseURL
}
