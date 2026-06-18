package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func runStart(opts Options) int {
	backendPort, err := resolvePorts(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 2
	}
	ports, err := pickPorts(backendPort)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	if err := ensureDataDir(); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}

	logLevel := resolveLogLevel(opts)

	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	return runManagedApp(managedAppConfig{
		Header:     "start mode: using local build",
		Mode:       "start",
		Backend:    self,
		BackendCWD: filepath.Dir(self),
		Ports:      ports,
		LogLevel:   logLevel,
		Opts:       opts,
	})
}

type managedAppConfig struct {
	Header     string
	Mode       string
	Backend    string
	BackendCWD string
	Ports      portConfig
	LogLevel   string
	Opts       Options
}

func resolveLogLevel(opts Options) string {
	if logLevel := os.Getenv("KANDEV_LOG_LEVEL"); logLevel != "" {
		return logLevel
	}
	switch {
	case opts.Debug:
		return "debug"
	case opts.Verbose:
		return "info"
	default:
		return "warn"
	}
}

func runManagedApp(cfg managedAppConfig) int {
	logStartup(cfg.Header, cfg.Ports, resolveDatabasePath(), cfg.LogLevel)

	supervisor := newSupervisor()
	supervisor.attachSignals()
	showOutput := cfg.Opts.Verbose || cfg.Opts.Debug
	backend, dumpLogs, err := launchRestartableBackend(cfg.Backend, []string{"__backend"}, cfg.BackendCWD, backendEnv(cfg.Ports, cfg.LogLevel, cfg.Opts.Debug), !showOutput, cfg.Ports, cfg.Mode, supervisor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	fmt.Println("[kandev] starting backend...")
	if err := waitForHealth(cfg.Ports.BackendURL, backend, healthTimeout(healthTimeoutReleaseMS), dumpLogs); err != nil {
		supervisor.shutdown("backend health failure")
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	fmt.Printf("[kandev] backend ready at %s\n", cfg.Ports.BackendURL)

	if cfg.Opts.Headless {
		fmt.Printf("[kandev] ready (headless) at %s\n", cfg.Ports.BackendURL)
		return waitForAppExit(supervisor, backend)
	}
	fmt.Println("[kandev] open: " + cfg.Ports.BackendURL)
	openBrowser(cfg.Ports.BackendURL)
	return waitForAppExit(supervisor, backend)
}

func logStartup(header string, ports portConfig, dbPath, logLevel string) {
	fmt.Println("[kandev] " + header)
	fmt.Println("[kandev] url:", ports.BackendURL)
	fmt.Println("[kandev] mcp:", ports.BackendURL+"/mcp")
	if dbPath != "" {
		fmt.Println("[kandev] db:", dbPath)
	}
	if logLevel != "" {
		fmt.Println("[kandev] log level:", logLevel)
	}
}

func openBrowser(url string) {
	if os.Getenv("KANDEV_NO_BROWSER") == "1" {
		return
	}
	var cmd *exec.Cmd
	switch {
	case os.Getenv("OS") == "Windows_NT":
		cmd = exec.Command("cmd.exe", "/c", "start", "", url)
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
