package launcher

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"
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

func resolvePorts(opts Options) (int, error) {
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

// resolveWebPort resolves the dev-mode web port: the --web-internal-port flag
// beats KANDEV_WEB_PORT, and 0 means "unset".
func resolveWebPort(opts Options) (int, error) {
	if opts.WebPort != 0 {
		return opts.WebPort, nil
	}
	return envPort(webPortEnv)
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
// port selection is dev-only: the picker runs for shared collision handling,
// then the web port is dropped so start/run never emit KANDEV_WEB_INTERNAL_URL.
func pickPorts(backendPort int, source string) (portConfig, error) {
	cfg, err := pickDevPorts(backendPort, source, 0, "")
	if err != nil {
		return portConfig{}, err
	}
	cfg.WebPort = 0
	return cfg, nil
}

// pickDevPorts resolves the backend, web, and agentctl ports. Explicitly
// requested ports must be bindable or the launch fails hard naming the port
// and its source (flag vs. env); omitted ports fall back to the preferred
// value, then a random free port. The three ports are guaranteed distinct.
func pickDevPorts(backendPort int, backendSource string, webPort int, webSource string) (portConfig, error) {
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

	web := webPort
	if web == 0 {
		p, err := pickAvailablePortExcept(defaultWebPort, used)
		if err != nil {
			return portConfig{}, err
		}
		web = p
	} else if !canBind(web) {
		sourceSuffix := ""
		if webSource != "" {
			sourceSuffix = fmt.Sprintf(" from %s", webSource)
		}
		return portConfig{}, fmt.Errorf("web port %d%s is already in use", web, sourceSuffix)
	}
	used[web] = true

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

func canBind(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
