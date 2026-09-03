package backendapp

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/agentctl/launcher"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"go.uber.org/zap"
)

// agentctlLauncherResult holds the outputs of provideAgentctlLauncher.
type agentctlLauncherResult struct {
	cleanup    func() error
	binaryPath string
}

// provideAgentctlLauncher starts or adopts the agentctl control server for
// standalone runtime. agentctl is a core service that always runs - it's used
// by the Standalone runtime for agent execution on the host machine.
//
// When the agent-survival capability is enabled (Layer 6.1 of
// agent-survival-across-restart), a surviving control server from a prior
// launch is tried first via lifecycle.AttemptAdoptControlServer; only when
// that fails or there is no record does this spawn a fresh one, same as
// when the capability is disabled. Either way, cfg.Agent.Standalone* is
// updated to point at whichever server this launch ends up using.
func provideAgentctlLauncher(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	availability *agentctlclient.Availability,
	store lifecycle.AdoptionRecordStore,
	secretStore secrets.SecretStore,
) (*agentctlLauncherResult, error) {
	if cfg.Features.AgentSurvival {
		if result := adoptSurvivingAgentctl(ctx, cfg, log, store, secretStore); result != nil {
			availability.MarkAvailable()
			return result, nil
		}
	}
	return spawnFreshAgentctl(ctx, cfg, log, availability, store, secretStore)
}

// adoptSurvivingAgentctl attempts to adopt a control server recorded by a
// prior launch. Returns nil when adoption did not complete for any reason
// (design 02's "Startup" steps 4-6): the caller then falls back to spawning
// a fresh server exactly as it would with the capability disabled.
func adoptSurvivingAgentctl(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	store lifecycle.AdoptionRecordStore,
	secretStore secrets.SecretStore,
) *agentctlLauncherResult {
	outcome := lifecycle.AttemptAdoptControlServer(ctx, store, secretStore, controlClientFactory(log),
		cfg.ResolvedHomeDir(), lifecycle.RequiredSurvivalCapabilities, log)
	if !outcome.Adopted {
		log.Info("control server adoption did not complete; spawning a fresh one",
			zap.String("reason", string(outcome.Reason)))
		return nil
	}

	host, port, err := splitEndpoint(outcome.Endpoint)
	if err != nil {
		log.Warn("adopted control server endpoint is malformed; spawning fresh instead", zap.Error(err))
		return nil
	}

	log.Info("adopted a surviving control server", zap.String("endpoint", outcome.Endpoint))
	cfg.Agent.StandaloneHost = host
	cfg.Agent.StandalonePort = port
	cfg.Agent.StandaloneAuthToken = outcome.Credential
	// This process never spawned the adopted server, so it has no PID to
	// record here. Standalone liveness for an adopted record is judged by
	// enumeration against the adopted server (design 02, "Persistence"),
	// not by a stale PID probe -- a later layer of this feature.
	cfg.Agent.StandalonePID = 0

	return &agentctlLauncherResult{
		// Survival is enabled and this server was adopted, not spawned: the
		// registered cleanup must not stop it (AC-EXECUTORS-SURVIVAL-001.1),
		// and there is no local process for this launcher to own regardless.
		cleanup:    func() error { return nil },
		binaryPath: launcher.FindAgentctlBinary(),
	}
}

// spawnFreshAgentctl is today's unconditional launch path. If the
// agent-survival capability is enabled, the freshly spawned server is also
// durably recorded as the installation's control server (failure-table row
// "Own server started after a refused or failed adoption"); recording
// failure is logged and non-fatal to startup, since the next restart simply
// finds no record and spawns fresh again.
func spawnFreshAgentctl(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	availability *agentctlclient.Availability,
	store lifecycle.AdoptionRecordStore,
	secretStore secrets.SecretStore,
) (*agentctlLauncherResult, error) {
	l, cleanup, err := launcher.Provide(ctx, launcher.Config{
		Host:             cfg.Agent.StandaloneHost,
		Port:             cfg.Agent.StandalonePort,
		StartupConfig:    cfg.ManagedAgentctlStartupConfig(),
		OnUnexpectedExit: availability.MarkUnavailable,
	}, log)
	if err != nil {
		return nil, err
	}
	availability.MarkAvailable()
	// Update config with the actual port (may differ if fallback was used)
	if actualPort := l.Port(); actualPort != cfg.Agent.StandalonePort {
		log.Info("agentctl port changed from configured value",
			zap.Int("configured_port", cfg.Agent.StandalonePort),
			zap.Int("actual_port", actualPort))
		cfg.Agent.StandalonePort = actualPort
	}
	// Store the per-launch auth token so downstream clients can authenticate
	cfg.Agent.StandaloneAuthToken = l.AuthToken()
	// Store the agentctl control-server PID so local/standalone executor rows can
	// carry a real host-local liveness handle (executors_running.local_pid).
	cfg.Agent.StandalonePID = l.Pid()

	if cfg.Features.AgentSurvival {
		endpoint := net.JoinHostPort(cfg.Agent.StandaloneHost, strconv.Itoa(cfg.Agent.StandalonePort))
		client, err := controlClientFactory(log)(endpoint)
		if err != nil {
			log.Warn("failed to build control client for the freshly spawned agentctl; adoption record not written", zap.Error(err))
		} else if err := lifecycle.RecordFreshControlServer(ctx, store, secretStore, client, endpoint, l.AuthToken()); err != nil {
			log.Warn("failed to record freshly spawned control server", zap.Error(err))
		}
	}

	return &agentctlLauncherResult{
		cleanup:    cleanup,
		binaryPath: l.BinaryPath(),
	}, nil
}

// controlClientFactory builds the real lifecycle.AdoptionControlClientFactory
// used in production: it parses "host:port" and returns a live
// agentctlclient.ControlClient targeting it.
func controlClientFactory(log *logger.Logger) lifecycle.AdoptionControlClientFactory {
	return func(endpoint string) (lifecycle.AdoptionControlClient, error) {
		host, port, err := splitEndpoint(endpoint)
		if err != nil {
			return nil, err
		}
		return agentctlclient.NewControlClient(host, port, log), nil
	}
}

func splitEndpoint(endpoint string) (host string, port int, err error) {
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", 0, fmt.Errorf("split control server endpoint %q: %w", endpoint, err)
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("parse control server port from endpoint %q: %w", endpoint, err)
	}
	return host, port, nil
}

// waitForAgentctlControlHealthy waits for the agentctl control server to be healthy.
// This is called during startup to ensure agentctl is ready before accepting requests.
func waitForAgentctlControlHealthy(ctx context.Context, cfg *config.Config, log *logger.Logger) {
	client := agentctlclient.NewControlClient(cfg.Agent.StandaloneHost, cfg.Agent.StandalonePort, log)
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		attemptCtx, attemptCancel := context.WithTimeout(healthCtx, 1*time.Second)
		err := client.Health(attemptCtx)
		attemptCancel()
		if err == nil {
			log.Info("agentctl control server is healthy")
			return
		}
		lastErr = err
		if healthCtx.Err() != nil {
			log.Warn("agentctl control server not ready; skipping resume wait", zap.Error(lastErr))
			return
		}
		<-ticker.C
	}
}
