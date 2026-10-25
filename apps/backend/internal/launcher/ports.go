package launcher

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/netprobe"
)

type portConfig struct {
	BackendPort  int
	WebPort      int
	AgentctlPort int
	BackendURL   string
}

const (
	backendPortFlag      = "--port"
	backendPortEnv       = "KANDEV_BACKEND_PORT"
	legacyBackendPortEnv = "KANDEV_PORT"
	webPortFlag          = "--web-internal-port"
	webPortEnv           = "KANDEV_WEB_PORT"
)

func resolvePorts(opts Options, configs ...*config.Config) (int, error) {
	if len(configs) > 0 {
		return resolvePortsForConfig(opts, configs[0])
	}
	backend := opts.BackendPort
	if backend == 0 {
		if p, err := envPort(backendPortEnv); err != nil {
			return 0, err
		} else if p != 0 {
			backend = p
		} else if p, err := envPort(legacyBackendPortEnv); err != nil {
			return 0, err
		} else {
			backend = p
		}
	}
	return backend, nil
}

func resolvePortsForConfig(opts Options, cfg *config.Config) (int, error) {
	if opts.BackendPort != 0 {
		return opts.BackendPort, nil
	}
	if _, ok := os.LookupEnv(backendPortEnv); ok {
		return resolvePorts(opts)
	}
	if _, ok := os.LookupEnv(legacyBackendPortEnv); ok {
		return resolvePorts(opts)
	}
	// KANDEV_SERVER_PORT is the child-backend handoff variable. The launcher
	// has historically resolved its own port from the flag, KANDEV_BACKEND_PORT,
	// or KANDEV_PORT, so an inherited child value must not pin a dev launcher to
	// an already-running parent backend.
	if cfg != nil && (cfg.SourceFor("server.port") == config.SourceConfiguration ||
		(cfg.SourceFor("server.port") == config.SourceEnvironment && serviceEnvironmentActive())) {
		if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
			return 0, ParseError{Message: fmt.Sprintf("server.port must be an integer between 1 and 65535, got %d", cfg.Server.Port)}
		}
		return cfg.Server.Port, nil
	}
	return 0, nil
}

func serviceEnvironmentActive() bool {
	return os.Getenv("KANDEV_SERVICE_MODE") != "" || os.Getenv("KANDEV_RUNNING_AS_SERVICE") != ""
}

// resolveWebPort resolves the dev-mode web port: the --web-internal-port flag
// beats KANDEV_WEB_PORT, and 0 means "unset".
func resolveWebPort(opts Options, configs ...*config.Config) (int, error) {
	if len(configs) > 0 {
		return resolveWebPortForConfig(opts, configs[0])
	}
	if opts.WebPort != 0 {
		return opts.WebPort, nil
	}
	return envPort(webPortEnv)
}

func resolveWebPortForConfig(opts Options, cfg *config.Config) (int, error) {
	if opts.WebPort != 0 {
		return opts.WebPort, nil
	}
	if _, ok := os.LookupEnv(webPortEnv); ok {
		return envPort(webPortEnv)
	}
	if cfg != nil && cfg.SourceFor("launcher.webPort") == config.SourceConfiguration {
		if cfg.Launcher.WebPort < 1 || cfg.Launcher.WebPort > 65535 {
			return 0, ParseError{Message: fmt.Sprintf("launcher.webPort must be an integer between 1 and 65535, got %d", cfg.Launcher.WebPort)}
		}
		return cfg.Launcher.WebPort, nil
	}
	return 0, nil
}

func backendPortSource(opts Options) string {
	if opts.BackendPort != 0 {
		return backendPortFlag
	}
	if _, ok := os.LookupEnv(backendPortEnv); ok {
		return backendPortEnv
	}
	if _, ok := os.LookupEnv(legacyBackendPortEnv); ok {
		return legacyBackendPortEnv
	}
	return ""
}

func webPortSource(opts Options) string {
	if opts.WebPort != 0 {
		return webPortFlag
	}
	if _, ok := os.LookupEnv(webPortEnv); ok {
		return webPortEnv
	}
	return ""
}

func envPort(name string) (int, error) {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if raw == "" || err != nil || n < 1 || n > 65535 {
		return 0, ParseError{Message: fmt.Sprintf("%s must be an integer between 1 and 65535, got %q", name, raw)}
	}
	return n, nil
}

// pickPorts resolves the start/run port pair (backend + agentctl). The web
// port selection is dev-only: with wantWeb=false it is skipped entirely, so a
// busy web port can never fail (or slow down) a release launch.
func pickPorts(backendPort int, source string) (portConfig, error) {
	return pickDevPorts(backendPort, source, 0, "", false)
}

// pickDevPorts resolves the backend, web, and agentctl ports. Explicitly
// requested ports must be bindable or the launch fails hard naming the port
// and its source (flag vs. env); omitted ports fall back to the preferred
// value, then a random free port. The ports are guaranteed distinct. Web
// selection only runs when wantWeb is set (dev mode).
func pickDevPorts(backendPort int, backendSource string, webPort int, webSource string, wantWeb bool) (portConfig, error) {
	used := map[int]bool{}
	backend := backendPort
	if backend == 0 {
		p, err := pickAvailablePortExcept(defaultBackendPort, used)
		if err != nil {
			return portConfig{}, err
		}
		backend = p
	} else if !canBind(backend) {
		sourceSuffix := ""
		if backendSource != "" {
			sourceSuffix = fmt.Sprintf(" from %s", backendSource)
		}
		return portConfig{}, fmt.Errorf("backend port %d%s is already in use", backend, sourceSuffix)
	}
	used[backend] = true

	web := 0
	if wantWeb {
		switch {
		case webPort == 0:
			p, err := pickAvailablePortExcept(defaultWebPort, used)
			if err != nil {
				return portConfig{}, err
			}
			web = p
		case used[webPort]:
			return portConfig{}, fmt.Errorf("web port %d conflicts with the backend port", webPort)
		case !canBind(webPort):
			sourceSuffix := ""
			if webSource != "" {
				sourceSuffix = fmt.Sprintf(" from %s", webSource)
			}
			return portConfig{}, fmt.Errorf("web port %d%s is already in use", webPort, sourceSuffix)
		default:
			web = webPort
		}
		used[web] = true
	}

	agentctl, err := pickAvailablePortExcept(defaultAgentctlPort, used)
	if err != nil {
		return portConfig{}, err
	}
	return portConfig{
		BackendPort:  backend,
		WebPort:      web,
		AgentctlPort: agentctl,
		BackendURL:   fmt.Sprintf("http://localhost:%d", backend),
	}, nil
}

func pickAvailablePortExcept(preferred int, used map[int]bool) (int, error) {
	if canBind(preferred) {
		if !used[preferred] {
			return preferred, nil
		}
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10; i++ {
		candidate := randomPortMin + r.Intn(randomPortMax-randomPortMin+1)
		if !used[candidate] && canBind(candidate) {
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("unable to find a free port")
}

// canBind reports whether a port is available, using the shared probe
// contract in internal/common/netprobe so the launcher and the agentctl
// launcher can never disagree about whether a port is occupied.
func canBind(port int) bool {
	return netprobe.PortAvailable(port)
}
