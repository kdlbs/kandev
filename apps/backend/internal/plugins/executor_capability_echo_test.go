package plugins

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/hostcaps"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

var (
	declWorkspace = []hostcaps.Requirement{{Name: "workspace_descriptor", Version: 1}}
	genA          = pluginDispatchGeneration{version: "1.0.0"}
	genB          = pluginDispatchGeneration{version: "1.1.0"}
)

func echoResponse(reqs ...hostcaps.Requirement) *pluginsdk.ValidateExecutorProfileResponse {
	response := &pluginsdk.ValidateExecutorProfileResponse{}
	for _, req := range reqs {
		response.ImplementedCapabilities = append(response.ImplementedCapabilities,
			&pluginsdk.ExecutorCapabilityVersion{Name: req.Name, Version: uint32(req.Version)})
	}
	return response
}

func newGateKey(process *int) capabilityGateKey {
	return capabilityGateKey{installationID: "inst", providerKey: "p", process: process, generation: genA}
}

func TestGateSkipsProvidersDeclaringNothing(t *testing.T) {
	var gate capabilityGate
	calls := 0
	err := gate.confirm(context.Background(), newGateKey(new(int)), nil, func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		calls++
		return nil, nil
	})
	if err != nil || calls != 0 {
		t.Fatalf("confirm() = %v after %d validations, want nil and none", err, calls)
	}
}

func TestGateRunsOneValidationThenServesTheCachedEcho(t *testing.T) {
	var gate capabilityGate
	process := new(int)
	calls := 0
	validate := func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		calls++
		return echoResponse(declWorkspace...), nil
	}
	for range 3 {
		if err := gate.confirm(context.Background(), newGateKey(process), declWorkspace, validate); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("validations = %d, want 1", calls)
	}
}

func TestGateRunsAgainForANewProcessOrGeneration(t *testing.T) {
	var gate capabilityGate
	calls := 0
	validate := func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		calls++
		return echoResponse(declWorkspace...), nil
	}
	first := newGateKey(new(int))
	restarted := newGateKey(new(int))
	upgraded := first
	upgraded.generation = genB
	for _, key := range []capabilityGateKey{first, restarted, upgraded} {
		if err := gate.confirm(context.Background(), key, declWorkspace, validate); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 3 {
		t.Fatalf("validations = %d, want 3", calls)
	}
}

func TestGateTreatsAnAbsentOrDifferingEchoAsMismatch(t *testing.T) {
	cases := map[string]*pluginsdk.ValidateExecutorProfileResponse{
		"absent echo":    echoResponse(),
		"wrong version":  echoResponse(hostcaps.Requirement{Name: "workspace_descriptor", Version: 2}),
		"missing member": echoResponse(hostcaps.Requirement{Name: "secret_references", Version: 1}),
		"extra member":   echoResponse(append([]hostcaps.Requirement{{Name: "secret_references", Version: 1}}, declWorkspace...)...),
	}
	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			var gate capabilityGate
			err := gate.confirm(context.Background(), newGateKey(new(int)), declWorkspace, func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
				return response, nil
			})
			if !errors.Is(err, ErrExecutorCapabilityMismatch) {
				t.Fatalf("confirm() = %v, want capability mismatch", err)
			}
		})
	}
}

func TestGateRefusesAfterTwoDifferingEchoesUntilTheGenerationChanges(t *testing.T) {
	var gate capabilityGate
	key := newGateKey(new(int))
	if err := gate.confirm(context.Background(), key, declWorkspace, func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		return echoResponse(declWorkspace...), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := gate.observe(key, declWorkspace, echoResponse(hostcaps.Requirement{Name: "secret_references", Version: 1})); !errors.Is(err, ErrExecutorCapabilityMismatch) {
		t.Fatalf("a second, differing echo must mismatch, got %v", err)
	}
	calls := 0
	err := gate.confirm(context.Background(), key, declWorkspace, func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		calls++
		return echoResponse(declWorkspace...), nil
	})
	if !errors.Is(err, ErrExecutorCapabilityMismatch) || calls != 0 {
		t.Fatalf("a mismatched generation must refuse without calling the provider: err=%v calls=%d", err, calls)
	}
	key.generation = genB
	if err := gate.confirm(context.Background(), key, declWorkspace, func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		return echoResponse(declWorkspace...), nil
	}); err != nil {
		t.Fatalf("a new generation starts clean: %v", err)
	}
}

func TestGateDoesNotCacheAFailedValidation(t *testing.T) {
	var gate capabilityGate
	key := newGateKey(new(int))
	boom := errors.New("provider down")
	for range 2 {
		err := gate.confirm(context.Background(), key, declWorkspace, func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
			return nil, boom
		})
		if !errors.Is(err, ErrExecutorProviderUnavailable) || errors.Is(err, ErrExecutorCapabilityMismatch) {
			t.Fatalf("a failed validation must refuse as unavailable, got %v", err)
		}
	}
	providerError := &pluginsdk.ValidateExecutorProfileResponse{Error: &pluginsdk.ExecutorProviderError{Code: "unavailable"}}
	err := gate.confirm(context.Background(), key, declWorkspace, func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		return providerError, nil
	})
	if !errors.Is(err, ErrExecutorProviderUnavailable) {
		t.Fatalf("an unavailable response is not an empty echo: %v", err)
	}
	if err := gate.confirm(context.Background(), key, declWorkspace, func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		return echoResponse(declWorkspace...), nil
	}); err != nil {
		t.Fatalf("the next launch must retry and succeed: %v", err)
	}
}

func TestGateTimesOutAValidationAfterTenSeconds(t *testing.T) {
	var gate capabilityGate
	gate.timeout = 20 * time.Millisecond
	err := gate.confirm(context.Background(), newGateKey(new(int)), declWorkspace, func(ctx context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if !errors.Is(err, ErrExecutorProviderUnavailable) {
		t.Fatalf("confirm() = %v, want unavailable", err)
	}
	if defaultCapabilityValidationTimeout != 10*time.Second {
		t.Fatalf("default timeout = %s, want 10s", defaultCapabilityValidationTimeout)
	}
}

func TestGateSharesOneValidationAcrossConcurrentLaunches(t *testing.T) {
	var gate capabilityGate
	key := newGateKey(new(int))
	var calls atomic.Int32
	release := make(chan struct{})
	validate := func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		calls.Add(1)
		<-release
		return echoResponse(declWorkspace...), nil
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- gate.confirm(context.Background(), key, declWorkspace, validate)
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("validations = %d, want 1", calls.Load())
	}
}

func TestGateComparesEachProviderEntryWithItsOwnDeclaration(t *testing.T) {
	var gate capabilityGate
	process := new(int)
	a := capabilityGateKey{installationID: "inst", providerKey: "a", process: process, generation: genA}
	b := capabilityGateKey{installationID: "inst", providerKey: "b", process: process, generation: genA}
	other := []hostcaps.Requirement{{Name: "secret_references", Version: 1}}
	if err := gate.confirm(context.Background(), a, declWorkspace, func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		return echoResponse(declWorkspace...), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := gate.confirm(context.Background(), b, other, func(context.Context) (*pluginsdk.ValidateExecutorProfileResponse, error) {
		return echoResponse(other...), nil
	}); err != nil {
		t.Fatalf("provider b must be compared with its own declaration: %v", err)
	}
}
