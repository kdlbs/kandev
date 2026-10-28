package plugins

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/plugins/hostcaps"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

// ErrExecutorCapabilityMismatch reports that the installed manifest, the host
// registry, and the running provider's echo do not agree. Launches fail with
// the stable code capability_mismatch before any provider allocation call.
var ErrExecutorCapabilityMismatch = models.ErrExecutorCapabilityMismatch

const defaultCapabilityValidationTimeout = 10 * time.Second

// capabilityGateKey names one provider entry of one running plugin process
// and installed generation. A restart or upgrade changes process or
// generation and so starts a new, echo-less entry.
type capabilityGateKey struct {
	installationID string
	providerKey    string
	process        any
	generation     pluginDispatchGeneration
}

type capabilityEchoEntry struct {
	process    any
	generation pluginDispatchGeneration
	echo       []string
	accepted   bool
	mismatched bool
}

type capabilityFlight struct {
	done chan struct{}
	err  error
}

// capabilityGate keeps the last accepted capability echo per provider entry
// and generation, and shares one profile validation between concurrent first
// launches.
type capabilityGate struct {
	mu      sync.Mutex
	entries map[string]*capabilityEchoEntry
	flights map[string]*capabilityFlight
	timeout time.Duration
}

func (k capabilityGateKey) id() string { return k.installationID + "\x00" + k.providerKey }

func (e *capabilityEchoEntry) current(key capabilityGateKey) bool {
	return e.process == key.process && e.generation.version == key.generation.version &&
		e.generation.installPath == key.generation.installPath && e.generation.installedAt.Equal(key.generation.installedAt)
}

// confirm checks that the declared set agrees with the registry and with the
// echo of the running provider. A provider declaring nothing is skipped
// without a validation. When the generation has no accepted echo it runs one
// profile validation through validate.
func (g *capabilityGate) confirm(
	ctx context.Context,
	key capabilityGateKey,
	declared []hostcaps.Requirement,
	validate func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error),
) error {
	if len(declared) == 0 {
		return nil
	}
	if errs := hostcaps.ValidateRequirements(declared); len(errs) > 0 {
		return fmt.Errorf("%w: %v", ErrExecutorCapabilityMismatch, errs[0])
	}
	g.mu.Lock()
	if entry := g.entries[key.id()]; entry != nil && entry.current(key) {
		if entry.mismatched {
			g.mu.Unlock()
			return ErrExecutorCapabilityMismatch
		}
		if entry.accepted {
			defer g.mu.Unlock()
			return compareEcho(declared, entry.echo)
		}
	}
	flight, running := g.flights[key.id()]
	if !running {
		flight = &capabilityFlight{done: make(chan struct{})}
		if g.flights == nil {
			g.flights = make(map[string]*capabilityFlight)
		}
		g.flights[key.id()] = flight
	}
	g.mu.Unlock()
	if running {
		select {
		case <-flight.done:
			return flight.err
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ErrExecutorProviderUnavailable, ctx.Err())
		}
	}
	flight.err = g.validateOnce(ctx, key, declared, validate)
	g.mu.Lock()
	delete(g.flights, key.id())
	g.mu.Unlock()
	close(flight.done)
	return flight.err
}

func (g *capabilityGate) validateOnce(
	ctx context.Context,
	key capabilityGateKey,
	declared []hostcaps.Requirement,
	validate func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error),
) error {
	timeout := g.timeout
	if timeout <= 0 {
		timeout = defaultCapabilityValidationTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	response, err := validate(callCtx)
	if err != nil {
		return fmt.Errorf("%w: capability validation failed", ErrExecutorProviderUnavailable)
	}
	if response == nil || response.GetError() != nil {
		return fmt.Errorf("%w: capability validation was not accepted", ErrExecutorProviderUnavailable)
	}
	return g.observe(key, declared, response)
}

// observe records the echo of any profile validation response. An echo that
// differs from the declaration, or from an earlier accepted echo of the same
// generation, marks the entry mismatched until the generation changes.
func (g *capabilityGate) observe(key capabilityGateKey, declared []hostcaps.Requirement, response *pluginsdk.ValidateExecutorProfileResponse) error {
	echo := canonicalEcho(response)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.entries == nil {
		g.entries = make(map[string]*capabilityEchoEntry)
	}
	entry := g.entries[key.id()]
	if entry == nil || !entry.current(key) {
		entry = &capabilityEchoEntry{process: key.process, generation: key.generation}
		g.entries[key.id()] = entry
	}
	if entry.mismatched {
		return ErrExecutorCapabilityMismatch
	}
	if entry.accepted && !slices.Equal(entry.echo, echo) {
		entry.mismatched = true
		return ErrExecutorCapabilityMismatch
	}
	if err := compareEcho(declared, echo); err != nil {
		entry.mismatched = true
		return err
	}
	entry.echo, entry.accepted = echo, true
	return nil
}

func canonicalEcho(response *pluginsdk.ValidateExecutorProfileResponse) []string {
	echo := make([]string, 0, len(response.GetImplementedCapabilities()))
	for _, item := range response.GetImplementedCapabilities() {
		if item != nil {
			echo = append(echo, fmt.Sprintf("%s@%d", item.GetName(), item.GetVersion()))
		}
	}
	slices.Sort(echo)
	return echo
}

func compareEcho(declared []hostcaps.Requirement, echo []string) error {
	want := make([]string, len(declared))
	for i, req := range declared {
		want[i] = fmt.Sprintf("%s@%d", req.Name, req.Version)
	}
	slices.Sort(want)
	if !slices.Equal(want, echo) {
		return ErrExecutorCapabilityMismatch
	}
	return nil
}

// hostCapabilityRows is the registry as offered to a provider in the profile
// validation request.
func hostCapabilityRows() []*pluginsdk.ExecutorHostCapability {
	entries := hostcaps.Registry()
	rows := make([]*pluginsdk.ExecutorHostCapability, len(entries))
	for i, entry := range entries {
		rows[i] = &pluginsdk.ExecutorHostCapability{
			Name: entry.Name, MinVersion: uint32(entry.Min), MaxVersion: uint32(entry.Max), DeprecatedIn: entry.DeprecatedIn,
		}
	}
	return rows
}

func capabilityKeyFor(record *store.Record, remote *pluginsdk.RemotePlugin, providerKey string) capabilityGateKey {
	return capabilityGateKey{
		installationID: record.InstallationID, providerKey: providerKey,
		process: remote, generation: dispatchGeneration(record),
	}
}

// ConfirmExecutorProviderCapabilities is the allocation gate: it refuses with
// ErrExecutorCapabilityMismatch when the manifest, the registry, and the
// accepted echo of the running provider disagree, and runs one profile
// validation first when the current generation has no echo. A provider that
// declares no capability is not validated and not gated.
func (s *Service) ConfirmExecutorProviderCapabilities(ctx context.Context, pluginID, providerKey string, profile models.ExecutorProviderProfile) error {
	return s.withExecutorProvider(ctx, pluginID, providerKey, func(callCtx context.Context, remote *pluginsdk.RemotePlugin, declaration *manifest.ExecutorProvider) error {
		declared := declaration.HostCapabilityRequirements()
		if len(declared) == 0 {
			return nil
		}
		record, err := s.Get(pluginID)
		if err != nil {
			return err
		}
		return s.capabilityGate.confirm(callCtx, capabilityKeyFor(record, remote, providerKey), declared,
			func(validateCtx context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
				snapshot, snapshotErr := validationProfileSnapshot(declaration, profile)
				if snapshotErr != nil {
					return nil, snapshotErr
				}
				return remote.ValidateExecutorProfile(validateCtx, &pluginsdk.ValidateExecutorProfileRequest{
					Context: &pluginsdk.ExecutorProviderRequestContext{
						PluginId: record.ID, InstallationId: record.InstallationID,
						ProviderKey: providerKey, ContractVersion: int32(declaration.ContractVersion),
					},
					Profile:          snapshot,
					HostCapabilities: hostCapabilityRows(),
				})
			})
	})
}
