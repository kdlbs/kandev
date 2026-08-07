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
	AgentctlPort int
	BackendURL   string
}

const (
	backendPortFlag      = "--port"
	backendPortEnv       = "KANDEV_BACKEND_PORT"
	legacyBackendPortEnv = "KANDEV_PORT"
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

func pickPorts(backendPort int, source ...string) (portConfig, error) {
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
		if len(source) > 0 && source[0] != "" {
			sourceSuffix = fmt.Sprintf(" from %s", source[0])
		}
		return portConfig{}, fmt.Errorf("backend port %d%s is already in use", backend, sourceSuffix)
	}
	used[backend] = true
	agentctl, err := pickAvailablePortExcept(defaultAgentctlPort, used)
	if err != nil {
		return portConfig{}, err
	}
	return portConfig{
		BackendPort:  backend,
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
