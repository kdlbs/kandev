package backendapp

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/agentctl/launcher"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/ownershipperiod"
	"github.com/kandev/kandev/internal/secrets"
	"go.uber.org/zap"
)

// agentctlLauncherResult holds the outputs of provideAgentctlLauncher.
type agentctlLauncherResult struct {
	cleanup    func() error
	binaryPath string
	// recoveryDeadlineStart is the AC-EXECUTORS-SURVIVAL-003.7 recovery
	// deadline's clock start (the instant this launch first contacted a
	// recorded control endpoint). Zero when this launch never contacted one
	// (adoption disabled, no record, or a fresh spawn) -- the lifecycle
	// manager falls back to its own Start-time default in that case.
	recoveryDeadlineStart time.Time
	// inheritedRecordScope is what this launch found at the recorded control
	// endpoint. It governs how recovery classifies recovery-inventory records
	// left by an earlier launch, which cannot be judged against a server this
	// backend started itself.
	inheritedRecordScope agentruntime.InheritedRecordScope
}

// provideAgentctlLauncher starts or adopts the agentctl control server for
// standalone runtime. agentctl is a core service that always runs - it's used
// by the Standalone runtime for agent execution on the host machine.
//
// When the agent-survival capability is enabled (Layer 6.1 of
// agent-survival-across-restart), a surviving control server from a prior
// launch is tried first via agentruntime.AttemptAdoptControlServer; only when
// that fails or there is no record does this spawn a fresh one, same as
// when the capability is disabled. Either way, cfg.Agent.Standalone* is
// updated to point at whichever server this launch ends up using.
func provideAgentctlLauncher(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	availability *agentctlclient.Availability,
	store agentruntime.AdoptionRecordStore,
	secretStore secrets.SecretStore,
) (*agentctlLauncherResult, error) {
	result, scope := resolveSurvivingAgentctl(ctx, cfg, log, store, secretStore)
	if result != nil {
		availability.MarkAvailable()
		return result, nil
	}
	fresh, err := spawnFreshAgentctl(ctx, cfg, log, availability, store, secretStore)
	if fresh != nil {
		// The server this launch just started never launched the instances a
		// prior launch recorded, so recovery needs to know what was found at
		// the recorded endpoint rather than judging them against it.
		fresh.inheritedRecordScope = scope
	}
	return fresh, err
}

// resolveSurvivingAgentctl applies the agent-survival capability gate to a
// control server a prior launch left running, and returns non-nil only when
// this launch took that server over. With the capability disabled nothing is
// ever adopted, but a server an earlier survival-enabled launch detached is
// still running with no backend attached to it, so it is stopped here rather
// than left to its own unowned-shutdown timer. Either way a nil return means
// the caller spawns fresh.
func resolveSurvivingAgentctl(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	store agentruntime.AdoptionRecordStore,
	secretStore secrets.SecretStore,
) (*agentctlLauncherResult, agentruntime.InheritedRecordScope) {
	if !cfg.Features.AgentSurvival {
		agentruntime.ReclaimUnneededControlServer(ctx, store, secretStore, controlClientFactory(log), cfg.ResolvedHomeDir(),
			cfg.Agentctl.RecoveryReadTimeout, cfg.Agentctl.RecoveryReadRetries, log)
		return nil, agentruntime.InheritedRecordScopeNoServer
	}
	return adoptSurvivingAgentctl(ctx, cfg, log, store, secretStore)
}

// refusedAdoptionScope maps a refused adoption to what recovery may conclude
// about records an earlier launch left behind. A refusal that never reached
// the server -- nothing answered at the recorded endpoint, or no endpoint was
// recorded -- means no control server holds those instances, so they can be
// probed and repaired. Every refusal that did reach a reachable server leaves
// that server running, and an instance may still be alive on it, so nothing
// may be concluded from its absence from a server this launch started.
func refusedAdoptionScope(outcome agentruntime.AdoptionOutcome) agentruntime.InheritedRecordScope {
	if outcome.ContactedAt.IsZero() {
		return agentruntime.InheritedRecordScopeNoServer
	}
	return agentruntime.InheritedRecordScopeForeignServer
}

// adoptSurvivingAgentctl attempts to adopt a control server recorded by a
// prior launch. Returns nil when adoption did not complete for any reason
// (design 02's "Startup" steps 4-6): the caller then falls back to spawning
// a fresh server exactly as it would with the capability disabled.
func adoptSurvivingAgentctl(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	store agentruntime.AdoptionRecordStore,
	secretStore secrets.SecretStore,
) (*agentctlLauncherResult, agentruntime.InheritedRecordScope) {
	outcome := agentruntime.AttemptAdoptControlServer(ctx, store, secretStore, controlClientFactory(log),
		cfg.ResolvedHomeDir(), agentruntime.RequiredSurvivalCapabilities,
		cfg.Agentctl.RecoveryReadTimeout, cfg.Agentctl.RecoveryReadRetries, log)
	if !outcome.Adopted {
		log.Info("control server adoption did not complete; spawning a fresh one",
			zap.String("reason", string(outcome.Reason)))
		return nil, refusedAdoptionScope(outcome)
	}

	host, port, err := splitEndpoint(outcome.Endpoint)
	if err != nil {
		log.Warn("adopted control server endpoint is malformed; spawning fresh instead", zap.Error(err))
		return nil, refusedAdoptionScope(outcome)
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

	// AC-EXECUTORS-CONTROL-OWNERSHIP-003.8: the rotation AttemptAdoptControlServer
	// just performed already counted as the first renewal, but nothing renews
	// ownership again after that without this loop. The renewal cadence is
	// computed from the adopted server's OWN reported unowned period
	// (outcome.UnownedPeriod), never this launch's local config: the two can
	// disagree across a restart that changed agentctl.unownedPeriod, and
	// renewing on a stale/mismatched cadence lets the server's own reaper
	// fire while this backend still believes it owns it (Review round 3,
	// finding 5).
	renewer := startOwnershipRenewal(ctx, log, outcome.Endpoint, outcome.Credential,
		resolveAdoptedRenewalPeriod(outcome.UnownedPeriod))

	return &agentctlLauncherResult{
		// Survival is enabled and this server was adopted, not spawned: the
		// registered cleanup must not stop it (AC-EXECUTORS-SURVIVAL-001.1),
		// and there is no local process for this launcher to own regardless.
		// It must still stop the renewal loop this backend started, or the
		// goroutine leaks past backend shutdown.
		cleanup: func() error {
			if renewer != nil {
				renewer.Stop()
			}
			return nil
		},
		binaryPath:            launcher.FindAgentctlBinary(),
		recoveryDeadlineStart: outcome.ContactedAt,
		inheritedRecordScope:  agentruntime.InheritedRecordScopeAdopted,
	}, agentruntime.InheritedRecordScopeAdopted
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
	store agentruntime.AdoptionRecordStore,
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
	// Store the per-launch auth token so downstream clients can authenticate.
	cfg.Agent.StandaloneAuthToken = l.AuthToken()
	// Store the agentctl control-server PID so local/standalone executor rows can
	// carry a real host-local liveness handle (executors_running.local_pid).
	cfg.Agent.StandalonePID = l.Pid()

	var renewer *agentruntime.OwnershipRenewer
	if cfg.Features.AgentSurvival {
		endpoint := net.JoinHostPort(cfg.Agent.StandaloneHost, strconv.Itoa(cfg.Agent.StandalonePort))
		client, err := controlClientFactory(log)(endpoint)
		if err != nil {
			log.Warn("failed to build control client for the freshly spawned agentctl; adoption record not written", zap.Error(err))
		} else if err := agentruntime.RecordFreshControlServer(ctx, store, secretStore, client, endpoint, l.AuthToken()); err != nil {
			log.Warn("failed to record freshly spawned control server", zap.Error(err))
		}
		// AC-EXECUTORS-CONTROL-OWNERSHIP-003.8: the bootstrap handshake this
		// launch just completed already counted as the first renewal, but
		// nothing renews ownership again after that without this loop. This
		// launch spawned the server moments ago from this same config, so
		// (unlike the adopted path above) the local resolution IS the value
		// the server itself enforces.
		period, _ := ownershipperiod.Resolve(cfg.Agentctl.UnownedPeriod, cfg.Agentctl.IdleTimeout)
		renewer = startOwnershipRenewal(ctx, log, endpoint, l.AuthToken(), period)
	}

	return &agentctlLauncherResult{
		cleanup: func() error {
			if renewer != nil {
				renewer.Stop()
			}
			return cleanup()
		},
		binaryPath: l.BinaryPath(),
	}, nil
}

// startOwnershipRenewal builds a dedicated control client authenticated with
// the given credential and starts the periodic ownership-claim loop against
// it (AC-EXECUTORS-CONTROL-OWNERSHIP-003.2/.8), computing the renewal
// interval from the given already-resolved unowned period. The caller
// decides where that period comes from: this launch's own local config is
// only correct for a server this launch just spawned itself -- an adopted
// server may have been spawned by a prior launch with different config, so
// its renewal period must come from what that server itself reports (see
// resolveAdoptedRenewalPeriod). Returns nil (logged, not fatal) when the
// endpoint can't be parsed; startup already succeeded by this point, so
// refusing to run the surviving server over a renewal-loop failure would
// trade a smaller problem for a bigger one.
func startOwnershipRenewal(
	ctx context.Context,
	log *logger.Logger,
	endpoint string,
	credential string,
	period time.Duration,
) *agentruntime.OwnershipRenewer {
	host, port, err := splitEndpoint(endpoint)
	if err != nil {
		log.Warn("failed to start ownership renewal loop; endpoint malformed", zap.Error(err))
		return nil
	}
	client := agentctlclient.NewControlClient(host, port, log)
	client.SetAuthToken(credential)
	renewer := agentruntime.NewOwnershipRenewer(client, ownershipperiod.RenewalInterval(period), log)
	renewer.Start(ctx)
	return renewer
}

// resolveAdoptedRenewalPeriod picks the unowned period an adopted server's
// ownership-renewal loop renews against: the value that server itself
// reported on /identity during adoption (outcome.UnownedPeriod), never this
// launch's local config -- the two can disagree across a restart that
// changed agentctl.unownedPeriod, and renewing on this launch's own
// (possibly longer) cadence lets the adopted server's own reaper fire while
// this backend still believes it owns it. Falls back to the shared floor
// only for a legacy pre-upgrade server that answered /identity without an
// unowned_period_ms field at all (reported as zero).
func resolveAdoptedRenewalPeriod(reported time.Duration) time.Duration {
	if reported <= 0 {
		return ownershipperiod.MinPeriod
	}
	return reported
}

// controlClientFactory builds the real agentruntime.AdoptionControlClientFactory
// used in production: it parses "host:port" and returns a live
// agentctlclient.ControlClient targeting it.
func controlClientFactory(log *logger.Logger) agentruntime.AdoptionControlClientFactory {
	return func(endpoint string) (agentruntime.AdoptionControlClient, error) {
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
