package client

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
)

type RuntimeControlProbe interface {
	Health(context.Context) error
	GetIdentity(context.Context) (*IdentityInfo, error)
	GetServerDetails(context.Context) (*ServerDetails, error)
}

type RuntimeEvidence string

const (
	RuntimeEvidenceHealthy             RuntimeEvidence = "healthy"
	RuntimeEvidenceTransportFailure    RuntimeEvidence = "transport_failure"
	RuntimeEvidenceProcessExited       RuntimeEvidence = "process_exited"
	RuntimeEvidenceCredentialsRejected RuntimeEvidence = "credentials_rejected"
	RuntimeEvidenceForeignOwner        RuntimeEvidence = "foreign_owner"
	RuntimeEvidenceOwnershipUnverified RuntimeEvidence = "ownership_unverified"
)

type RuntimeObservation struct {
	Evidence        RuntimeEvidence
	ProcessIdentity processidentity.Identity
}

type ProcessIdentityInspector func(processidentity.Identity) (processidentity.State, error)

// AssessAdoptedRuntime distinguishes a live-but-unresponsive server from a
// proven exited process, stale credential, or foreign process on the endpoint.
// Only a matching OS birth identity can prove that the recorded process died.
func AssessAdoptedRuntime(
	ctx context.Context,
	probe RuntimeControlProbe,
	expectedServerIdentity string,
	storedIdentity processidentity.Identity,
	inspect ProcessIdentityInspector,
) RuntimeObservation {
	if probe == nil || expectedServerIdentity == "" {
		return RuntimeObservation{Evidence: RuntimeEvidenceOwnershipUnverified}
	}
	if inspect == nil {
		inspect = processidentity.Inspect
	}

	healthErr := probe.Health(ctx)
	identity, err := probe.GetIdentity(ctx)
	if err != nil || identity == nil {
		return processEvidence(storedIdentity, inspect)
	}
	if identity.ServerIdentity != expectedServerIdentity {
		return RuntimeObservation{Evidence: RuntimeEvidenceForeignOwner}
	}
	return assessAdoptedServer(ctx, probe, healthErr, storedIdentity, inspect)
}

func assessAdoptedServer(
	ctx context.Context,
	probe RuntimeControlProbe,
	healthErr error,
	storedIdentity processidentity.Identity,
	inspect ProcessIdentityInspector,
) RuntimeObservation {
	details, err := probe.GetServerDetails(ctx)
	if errors.Is(err, ErrControlCredentialsRejected) {
		return RuntimeObservation{Evidence: RuntimeEvidenceCredentialsRejected}
	}
	if err != nil || details == nil {
		return processEvidence(storedIdentity, inspect)
	}
	return assessReportedIdentity(healthErr, details.ProcessIdentity, storedIdentity)
}

func assessReportedIdentity(
	healthErr error,
	reported *processidentity.Identity,
	stored processidentity.Identity,
) RuntimeObservation {
	var observed processidentity.Identity
	if reported != nil {
		observed = *reported
	}
	storedValid := stored.Validate() == nil
	if storedValid && (observed.Validate() != nil || observed != stored) {
		return RuntimeObservation{Evidence: RuntimeEvidenceOwnershipUnverified}
	}
	if healthErr == nil {
		return RuntimeObservation{Evidence: RuntimeEvidenceHealthy, ProcessIdentity: observed}
	}
	return RuntimeObservation{Evidence: RuntimeEvidenceTransportFailure, ProcessIdentity: observed}
}

func processEvidence(identity processidentity.Identity, inspect ProcessIdentityInspector) RuntimeObservation {
	if identity.Validate() != nil {
		return RuntimeObservation{Evidence: RuntimeEvidenceOwnershipUnverified}
	}
	state, err := inspect(identity)
	if err != nil || state == processidentity.StateUnknown {
		return RuntimeObservation{Evidence: RuntimeEvidenceOwnershipUnverified}
	}
	switch state {
	case processidentity.StateAlive:
		return RuntimeObservation{Evidence: RuntimeEvidenceTransportFailure, ProcessIdentity: identity}
	case processidentity.StateExited, processidentity.StateReused:
		return RuntimeObservation{Evidence: RuntimeEvidenceProcessExited, ProcessIdentity: identity}
	default:
		return RuntimeObservation{Evidence: RuntimeEvidenceOwnershipUnverified}
	}
}

// AdoptedRuntimeMonitor polls one adopted server without treating a network
// timeout as process death. It stops after evidence requires operator or
// coordinator action.
type AdoptedRuntimeMonitor struct {
	probe                  RuntimeControlProbe
	expectedServerIdentity string
	processIdentity        processidentity.Identity
	interval               time.Duration
	probeTimeout           time.Duration
	retries                int
	inspect                ProcessIdentityInspector
	onObservation          func(RuntimeObservation)

	mu      sync.Mutex
	cancel  context.CancelFunc
	started bool
	wg      sync.WaitGroup
}

func NewAdoptedRuntimeMonitor(
	probe RuntimeControlProbe,
	expectedServerIdentity string,
	identity processidentity.Identity,
	interval time.Duration,
	probeTimeout time.Duration,
	retries int,
	inspect ProcessIdentityInspector,
	onObservation func(RuntimeObservation),
) *AdoptedRuntimeMonitor {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if probeTimeout <= 0 {
		probeTimeout = interval
	}
	if retries < 0 {
		retries = 0
	}
	return &AdoptedRuntimeMonitor{
		probe:                  probe,
		expectedServerIdentity: expectedServerIdentity,
		processIdentity:        identity,
		interval:               interval,
		probeTimeout:           probeTimeout,
		retries:                retries,
		inspect:                inspect,
		onObservation:          onObservation,
	}
}

func (monitor *AdoptedRuntimeMonitor) Start(ctx context.Context) {
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	if monitor.started || monitor.probe == nil || monitor.onObservation == nil {
		return
	}
	monitor.started = true
	ctx, monitor.cancel = context.WithCancel(ctx)
	monitor.wg.Add(1)
	go monitor.loop(ctx)
}

func (monitor *AdoptedRuntimeMonitor) Stop() {
	monitor.mu.Lock()
	cancel := monitor.cancel
	monitor.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	monitor.wg.Wait()
	monitor.mu.Lock()
	monitor.started = false
	monitor.mu.Unlock()
}

func (monitor *AdoptedRuntimeMonitor) loop(ctx context.Context) {
	defer monitor.wg.Done()
	ticker := time.NewTicker(monitor.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		observation := monitor.observe(ctx)
		if ctx.Err() != nil {
			return
		}
		if observation.Evidence == RuntimeEvidenceHealthy {
			select {
			case <-ctx.Done():
				return
			case <-time.After(monitor.interval):
				continue
			}
		}
		monitor.onObservation(observation)
		return
	}
}

func (monitor *AdoptedRuntimeMonitor) observe(ctx context.Context) RuntimeObservation {
	var observation RuntimeObservation
	for attempt := 0; attempt <= monitor.retries; attempt++ {
		probeCtx, cancel := context.WithTimeout(ctx, monitor.probeTimeout)
		observation = AssessAdoptedRuntime(probeCtx, monitor.probe, monitor.expectedServerIdentity,
			monitor.processIdentity, monitor.inspect)
		cancel()
		if observation.Evidence != RuntimeEvidenceTransportFailure &&
			observation.Evidence != RuntimeEvidenceOwnershipUnverified {
			return observation
		}
		if attempt < monitor.retries {
			select {
			case <-ctx.Done():
				return RuntimeObservation{Evidence: RuntimeEvidenceOwnershipUnverified}
			case <-time.After(monitor.interval):
			}
		}
	}
	return observation
}
