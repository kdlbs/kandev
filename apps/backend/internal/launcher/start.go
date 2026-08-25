package launcher

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/kandev/kandev/internal/common/config"
)

var (
	executablePath  = os.Executable
	launchManaged   = runManagedApp
	newSupervisorFn = newSupervisor
	launchBackendFn = launchRestartableBackend
	waitForHealthFn = waitForHealth
	waitForReadyFn  = waitForReady
	attachSignalsFn = func(supervisor *processSupervisor) {
		supervisor.attachSignals()
	}
	startParentWatchFn = func(supervisor *processSupervisor) *parentWatchdog {
		watchdog := newParentWatchdogFromEnv(supervisor.shutdown, launcherExit)
		watchdog.start()
		return watchdog
	}
)

func runStart(ctx context.Context, opts Options) int {
	startupConfig, err := loadBootstrapConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	backendPort, err := resolvePorts(opts, startupConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 2
	}
	ports, err := pickPorts(backendPort, backendPortSource(opts))
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	endpoints, err := resolveBackendEndpoints(startupConfig, ports.BackendPort)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	ports.BackendURL = endpoints.accessURL
	if err := ensureDataDirForConfig(startupConfig); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}

	logLevel := resolveLogLevelForConfig(opts, startupConfig)

	self, err := executablePath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	return launchManaged(ctx, managedAppConfig{
		Header:     "start mode: using local build",
		Mode:       "start",
		Backend:    self,
		BackendCWD: filepath.Dir(self),
		Ports:      ports,
		LogLevel:   logLevel,
		Opts:       opts,
		Startup:    startupConfig,
		Endpoints:  endpoints,
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
	Startup    *config.Config
	Endpoints  backendEndpointSet
}

func resolveLogLevel(opts Options, configs ...*config.Config) string {
	if len(configs) > 0 {
		return resolveLogLevelForConfig(opts, configs[0])
	}
	return resolveLogLevelForConfig(opts, nil)
}

func resolveLogLevelForConfig(opts Options, cfg *config.Config) string {
	if logLevel := os.Getenv("KANDEV_LOG_LEVEL"); logLevel != "" {
		return logLevel
	}
	if configSourceIsExplicit(cfg, "logging.level") && cfg.Logging.Level != "" {
		return cfg.Logging.Level
	}
	switch {
	case opts.Debug:
		return "debug"
	default:
		return "info"
	}
}

func resolveConsoleLogLevel(opts Options) string {
	if opts.Verbose {
		return "info"
	}
	return "warn"
}

func runManagedApp(ctx context.Context, cfg managedAppConfig) int {
	if cfg.Endpoints.accessURL == "" {
		if cfg.Startup != nil {
			endpoints, err := resolveBackendEndpoints(cfg.Startup, cfg.Ports.BackendPort)
			if err != nil {
				fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
				return 1
			}
			cfg.Endpoints = endpoints
		} else {
			cfg.Endpoints = endpointSetForAccessURL(cfg.Ports.BackendURL)
		}
	}
	cfg.Ports.BackendURL = cfg.Endpoints.accessURL
	ignoreBrokenPipeSignal()
	logStartup(cfg.Header, cfg.Ports, resolveDatabasePathForConfig(cfg.Startup), cfg.LogLevel, serverHostForConfig(cfg.Startup))
	setLauncherShutdownDebug(cfg.Opts.Debug || os.Getenv("KANDEV_SHUTDOWN_DEBUG") == "1")
	shutdownDebugf("runManagedApp start mode=%q backend=%q backend_cwd=%q debug=%t", cfg.Mode, cfg.Backend, cfg.BackendCWD, cfg.Opts.Debug)

	supervisor := newSupervisorFn()
	attachSignalsFn(supervisor)
	shutdownDebugf("runManagedApp signal handler attached")
	parentWatchdog := startParentWatchFn(supervisor)
	if parentWatchdog != nil {
		defer parentWatchdog.stop()
	}
	healthToken, err := launchHealthToken()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	env := backendEnvForConfig(cfg.Ports, cfg.LogLevel, resolveConsoleLogLevel(cfg.Opts), cfg.Opts.Debug, healthToken, nil, cfg.Startup)
	backend, dumpLogs, err := launchBackendFn(backendLaunchConfig{
		Command:    cfg.Backend,
		Args:       []string{"__backend"},
		CWD:        cfg.BackendCWD,
		Env:        env,
		Quiet:      false,
		Ports:      cfg.Ports,
		Mode:       cfg.Mode,
		HomeDir:    resolveHomeDirForConfig(cfg.Startup),
		Supervisor: supervisor,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	shutdownDebugf("runManagedApp backend launched")
	fmt.Println("[kandev] starting backend...")
	readyURL, err := waitForHealthFn(ctx, cfg.Endpoints, backend, healthTimeoutForConfig(healthTimeoutReleaseMS, cfg.Startup), healthToken, dumpLogs)
	if err != nil {
		supervisor.shutdown("backend health failure")
		fmt.Fprintln(os.Stderr, formatStartupFailure(err, cfg.Endpoints, cfg.Startup, backendLogPathForConfig(cfg.Startup), startupFailureStoppedBackend(err)))
		return 1
	}
	if readyURL != "" {
		cfg.Ports.BackendURL = readyURL
	}
	shutdownDebugf("runManagedApp backend healthy, waiting for readiness")
	if err := waitForReadyFn(ctx, cfg.Ports.BackendURL, backend); err != nil {
		supervisor.shutdown("backend readiness failure")
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	fmt.Printf("[kandev] backend ready at %s\n", cfg.Ports.BackendURL)

	if !shouldOpenBrowser(cfg.Opts, cfg.Startup) {
		fmt.Printf("[kandev] ready (headless) at %s\n", cfg.Ports.BackendURL)
		return waitForAppExit(supervisor, backend)
	}
	fmt.Println("[kandev] open: " + cfg.Ports.BackendURL)
	openBrowser(cfg.Ports.BackendURL)
	return waitForAppExit(supervisor, backend)
}

func logStartup(header string, ports portConfig, dbPath, logLevel string, serverHosts ...string) {
	fmt.Println("[kandev] " + header)
	fmt.Println("[kandev] url:", ports.BackendURL)
	serverHost := os.Getenv("KANDEV_SERVER_HOST")
	if len(serverHosts) > 0 && serverHosts[0] != "" {
		serverHost = serverHosts[0]
	}
	hosts := networkAddressesForBindHost(listHostNetworkAddresses(), serverHost)
	for _, url := range networkURLsForPort(ports.BackendPort, hosts) {
		fmt.Println("[kandev]   network:", url)
	}
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
		cmd = exec.Command(cmdExe, "/c", "start", "", url)
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func serverHostForConfig(cfg *config.Config) string {
	if cfg != nil && configSourceIsExplicit(cfg, "server.host") {
		return cfg.Server.Host
	}
	return os.Getenv("KANDEV_SERVER_HOST")
}

func shouldOpenBrowser(opts Options, cfg *config.Config) bool {
	if opts.Headless {
		return false
	}
	if cfg != nil {
		return !cfg.Launcher.NoBrowser
	}
	return os.Getenv("KANDEV_NO_BROWSER") != "1"
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
