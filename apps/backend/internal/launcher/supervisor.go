package launcher

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type launchManifest struct {
	Version           int               `json:"version"`
	BackendExecutable string            `json:"backend_executable"`
	Argv              []string          `json:"argv"`
	CWD               string            `json:"cwd"`
	Env               map[string]string `json:"env"`
	HomeDir           string            `json:"home_dir"`
	Port              int               `json:"port"`
	Mode              string            `json:"mode"`
	CreatedAt         string            `json:"created_at"`
}

type restartableBackend struct {
	mu         sync.Mutex
	restartMu  sync.Mutex
	command    string
	args       []string
	cwd        string
	env        []string
	quiet      bool
	supervisor *processSupervisor
	current    *managedProcess
	exitCh     chan int
	dumpLogs   func()
}

func launchRestartableBackend(command string, args []string, cwd string, env []string, quiet bool, ports portConfig, mode string, supervisor *processSupervisor) (*restartableBackend, func(), error) {
	homeDir := resolveHomeDir()
	supervisorEnv, socket, manifestPath, err := prepareSupervisorEnv(env, homeDir)
	if err != nil {
		return nil, nil, err
	}
	manifest := buildManifest(command, args, cwd, supervisorEnv, homeDir, ports.BackendPort, mode)
	if err := writeManifest(manifest, manifestPath); err != nil {
		return nil, nil, err
	}
	backend := &restartableBackend{
		command:    command,
		args:       args,
		cwd:        cwd,
		env:        supervisorEnv,
		quiet:      quiet,
		supervisor: supervisor,
		exitCh:     make(chan int, 1),
	}
	if err := backend.start(); err != nil {
		return nil, nil, err
	}
	if err := startControlServer(socket, backend.restart); err != nil {
		backend.mu.Lock()
		current := backend.current
		backend.current = nil
		backend.mu.Unlock()
		if current != nil {
			current.kill()
		}
		return nil, nil, err
	}
	return backend, func() {
		backend.mu.Lock()
		dump := backend.dumpLogs
		backend.mu.Unlock()
		if dump != nil {
			dump()
		}
	}, nil
}

func (b *restartableBackend) start() error {
	proc, dump, err := startProcess(b.command, b.args, b.cwd, b.env, b.quiet, "backend", b.supervisor)
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.current = proc
	b.dumpLogs = dump
	b.mu.Unlock()
	go func() {
		<-proc.done
		_, code := proc.Exited()
		b.mu.Lock()
		current := b.current == proc
		b.mu.Unlock()
		if current {
			select {
			case b.exitCh <- code:
			default:
			}
		}
	}()
	return nil
}

func (b *restartableBackend) restart() {
	b.restartMu.Lock()
	defer b.restartMu.Unlock()

	b.mu.Lock()
	current := b.current
	b.current = nil
	b.mu.Unlock()
	if current != nil {
		current.kill()
	}
	if err := b.start(); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] backend restart failed: "+err.Error())
		b.notifyExit(1)
	}
}

func (b *restartableBackend) notifyExit(code int) {
	select {
	case b.exitCh <- code:
	default:
	}
}

func (b *restartableBackend) Exited() (bool, int) {
	b.mu.Lock()
	current := b.current
	b.mu.Unlock()
	if current == nil {
		return false, 0
	}
	return current.Exited()
}

func prepareSupervisorEnv(env []string, homeDir string) ([]string, string, string, error) {
	dir := filepath.Join(homeDir, "supervisor")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", "", err
	}
	_ = os.Chmod(dir, 0o700)
	socket := filepath.Join(dir, "control.sock")
	manifest := filepath.Join(dir, "launch.json")
	env = upsertEnv(env, "KANDEV_SUPERVISOR_SOCKET", socket)
	env = upsertEnv(env, "KANDEV_SUPERVISOR_MANIFEST", manifest)
	env = upsertEnv(env, "KANDEV_RESTART_ADAPTER", "supervisor")
	return env, socket, manifest, nil
}

func buildManifest(command string, args []string, cwd string, env []string, homeDir string, port int, mode string) launchManifest {
	return launchManifest{
		Version:           1,
		BackendExecutable: command,
		Argv:              append([]string(nil), args...),
		CWD:               cwd,
		Env:               allowedSupervisorEnv(env),
		HomeDir:           homeDir,
		Port:              port,
		Mode:              mode,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func allowedSupervisorEnv(env []string) map[string]string {
	allow := map[string]bool{
		"KANDEV_HOME_DIR":              true,
		"KANDEV_DATABASE_PATH":         true,
		"KANDEV_SERVER_PORT":           true,
		"KANDEV_WEB_INTERNAL_URL":      true,
		"KANDEV_AGENT_STANDALONE_PORT": true,
		"KANDEV_LOG_LEVEL":             true,
		"KANDEV_DEBUG_DEV_MODE":        true,
		"KANDEV_DEBUG_AGENT_MESSAGES":  true,
		"KANDEV_DEBUG_PPROF_ENABLED":   true,
		"KANDEV_E2E_MOCK":              true,
		"KANDEV_MOCK_AGENT":            true,
		"KANDEV_MOCK_GITHUB":           true,
		"KANDEV_MOCK_JIRA":             true,
		"KANDEV_MOCK_LINEAR":           true,
		"KANDEV_SUPERVISOR_SOCKET":     true,
		"KANDEV_SUPERVISOR_MANIFEST":   true,
		"KANDEV_RESTART_ADAPTER":       true,
	}
	out := map[string]string{}
	for _, item := range env {
		for key := range allow {
			prefix := key + "="
			if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
				out[key] = item[len(prefix):]
				break
			}
		}
	}
	return out
}

func writeManifest(manifest launchManifest, targetPath string) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(targetPath, data, 0o600)
}

func startControlServer(socket string, onRestart func()) error {
	ln, err := listenControlSocket(socket)
	if err != nil {
		return err
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleControlConn(conn, onRestart)
		}
	}()
	return nil
}

func listenControlSocket(socket string) (net.Listener, error) {
	_ = os.Remove(socket)
	ln, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socket, 0o600); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return ln, nil
}

func handleControlConn(conn net.Conn, onRestart func()) {
	defer func() { _ = conn.Close() }()
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return
	}
	var req map[string]string
	resp := map[string]interface{}{"accepted": false, "message": "unsupported action"}
	if json.Unmarshal(line, &req) == nil && req["action"] == "restart" {
		go onRestart()
		resp = map[string]interface{}{"accepted": true, "message": "Restart accepted"}
	}
	_ = json.NewEncoder(conn).Encode(resp)
}
