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

func resolvePorts(opts Options) (int, int, error) {
	backend := opts.BackendPort
	if backend == 0 {
		if p, err := envPort("KANDEV_BACKEND_PORT"); err != nil {
			return 0, 0, err
		} else if p != 0 {
			backend = p
		} else if p, err := envPort("KANDEV_PORT"); err != nil {
			return 0, 0, err
		} else {
			backend = p
		}
	}
	web := opts.WebPort
	if web == 0 {
		p, err := envPort("KANDEV_WEB_PORT")
		if err != nil {
			return 0, 0, err
		}
		web = p
	}
	return backend, web, nil
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

func pickPorts(backendPort, webPort int) (portConfig, error) {
	if backendPort != 0 && webPort != 0 && backendPort == webPort {
		return portConfig{}, fmt.Errorf("backend and web ports must be different")
	}
	used := map[int]bool{}
	backend := backendPort
	if backend == 0 {
		p, err := pickAvailablePortExcept(defaultBackendPort, used)
		if err != nil {
			return portConfig{}, err
		}
		backend = p
	}
	used[backend] = true
	web := webPort
	if web == 0 {
		p, err := pickAvailablePortExcept(defaultWebPort, used)
		if err != nil {
			return portConfig{}, err
		}
		web = p
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

func pickAvailablePort(preferred int) (int, error) {
	return pickAvailablePortExcept(preferred, nil)
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
