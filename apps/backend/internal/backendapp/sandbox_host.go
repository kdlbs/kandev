package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	goruntime "runtime"
	"slices"
	"strconv"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/config"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

// sandboxHostOS is the core host OS; tests replace it.
var sandboxHostOS = goruntime.GOOS

const hostDarwin = "darwin"

const sandboxIdentityReadBound = 3 * time.Second

// admitSandboxHost refuses a darwin launch whose executor did not advertise
// sandbox launch after its Seatbelt probe. The read is per launch and uncached;
// a failed read is a refusal, never a retry.
func admitSandboxHost(ctx context.Context, transport *runtime.AgentCtlClient) error {
	if sandboxHostOS != hostDarwin {
		return nil
	}
	if err := transport.SandboxLaunchSupported(ctx); err != nil {
		return fmt.Errorf("%w: %w", policy.ErrHostUnsupported, err)
	}
	return nil
}

// checkManagedSandboxHost is the Settings availability check's host test: with
// no executor in its callback it reads the managed local agentctl on every call.
func checkManagedSandboxHost(ctx context.Context, cfg *config.Config) error {
	if sandboxHostOS != hostDarwin {
		return nil
	}
	host := cfg.Agent.StandaloneHost
	if host == "" {
		host = "127.0.0.1"
	}
	capabilities, err := readIdentityCapabilities(ctx, "http://"+net.JoinHostPort(host, strconv.Itoa(cfg.Agent.StandalonePort))+"/identity")
	if err != nil || !slices.Contains(capabilities, contract.LaunchCapability) {
		return policy.ErrHostUnsupported
	}
	return nil
}

func readIdentityCapabilities(ctx context.Context, url string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, sandboxIdentityReadBound)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("identity unavailable")
	}
	var identity struct {
		Capabilities []string `json:"capabilities"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<16)).Decode(&identity); err != nil {
		return nil, err
	}
	return identity.Capabilities, nil
}
