package launcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kandev/kandev/internal/common/config"
)

var (
	waitForURLFn   = waitForURLReady
	openBrowserFn  = openBrowser
	startProcessFn = startProcess
)

// runDev launches the dev stack: backend via `make -C apps/backend dev` (so a
// Settings-triggered restart rebuilds from source) plus the Vite dev server.
// All dev state lives under <repoRoot>/.kandev-dev/ so `make dev` never
// mutates the user's production ~/.kandev.
func runDev(ctx context.Context, opts Options) int {
	cfg, code := devLaunchConfigFor(opts)
	if code != 0 {
		return code
	}
	if err := backupProductionDBIfNeeded(cfg.dbPath); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	logStartup("dev mode: using local repo", cfg.ports, cfg.dbPath, cfg.logLevel, serverHostForConfig(cfg.startup))

	ignoreBrokenPipeSignal()
	setLauncherShutdownDebug(cfg.debug || os.Getenv("KANDEV_SHUTDOWN_DEBUG") == "1")
	supervisor := newSupervisorFn()
	attachSignalsFn(supervisor)

	healthToken, err := newHealthToken()
	if err != nil {
		supervisor.shutdown("health token failure")
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	env := backendEnvForConfig(cfg.ports, cfg.logLevel, resolveConsoleLogLevel(opts), opts.Debug, healthToken, cfg.extra, cfg.startup)
	env = upsertEnv(env, backendPIDFileEnv, filepath.Join(devKandevHome(cfg.repoRoot), "supervisor", "backend.pid"))
	backend, dumpLogs, err := launchBackendFn(backendLaunchConfig{
		Command:    "make",
		Args:       []string{"-C", "apps/backend", "dev"},
		CWD:        cfg.repoRoot,
		Env:        env,
		Quiet:      false,
		Ports:      cfg.ports,
		Mode:       "dev",
		HomeDir:    cfg.homeDir,
		Supervisor: supervisor,
	})
	if err != nil {
		supervisor.shutdown("backend launch failure")
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}

	fmt.Println("[kandev] starting backend...")
	readyURL, err := waitForHealthFn(ctx, cfg.endpoints, backend, healthTimeoutForConfig(healthTimeoutDevMS, cfg.startup), healthToken, dumpLogs)
	if err != nil {
		supervisor.shutdown("backend health failure")
		fmt.Fprintln(os.Stderr, formatStartupFailure(err, cfg.endpoints, cfg.startup, backendLogPathForDevConfig(cfg), startupFailureStoppedBackend(err)))
		return 1
	}
	if readyURL != "" {
		cfg.ports.BackendURL = readyURL
	}
	if err := waitForReadyFn(ctx, cfg.ports.BackendURL, backend); err != nil {
		supervisor.shutdown("backend readiness failure")
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	fmt.Printf("[kandev] backend ready at %s\n", cfg.ports.BackendURL)

	webURL := fmt.Sprintf("http://localhost:%d", cfg.ports.WebPort)
	fmt.Println("[kandev] starting web...")
	webProc, err := launchWebChild(cfg.repoRoot, cfg.ports, supervisor)
	if err != nil {
		supervisor.shutdown("web launch failure")
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	if err := waitForURLFn(ctx, webURL, webProc, healthTimeoutForConfig(healthTimeoutDevMS, cfg.startup)); err != nil {
		supervisor.shutdown("web readiness failure")
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}

	if !shouldOpenBrowser(opts, cfg.startup) {
		fmt.Printf("[kandev] ready (headless) at %s\n", cfg.ports.BackendURL)
		return waitForAppExit(supervisor, backend, webProc)
	}
	fmt.Println("[kandev] open: " + cfg.ports.BackendURL)
	openBrowserFn(cfg.ports.BackendURL)
	return waitForAppExit(supervisor, backend, webProc)
}

type devLaunchConfig struct {
	repoRoot  string
	ports     portConfig
	dbPath    string
	extra     []string
	logLevel  string
	debug     bool
	homeDir   string
	startup   *config.Config
	endpoints backendEndpointSet
}

// devLaunchConfigFor resolves everything runDev needs before any process is
// spawned: repo root, dev ports, and the dev backend env. It returns a
// nonzero exit code (having printed the error) when any step fails.
func devLaunchConfigFor(opts Options, configs ...*config.Config) (devLaunchConfig, int) {
	startupConfig, exitCode := devStartupConfig(configs...)
	if exitCode != 0 {
		return devLaunchConfig{}, exitCode
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return devLaunchConfig{}, 2
	}
	repoRoot, err := findRepoRoot(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return devLaunchConfig{}, 2
	}
	backendPort, err := resolvePorts(opts, startupConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return devLaunchConfig{}, 2
	}
	webPort, err := resolveWebPort(opts, startupConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return devLaunchConfig{}, 2
	}
	ports, err := pickDevPorts(backendPort, backendPortSource(opts), webPort, webPortSource(opts), true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return devLaunchConfig{}, 1
	}
	endpoints, err := resolveBackendEndpoints(startupConfig, ports.BackendPort)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return devLaunchConfig{}, 1
	}
	ports.BackendURL = endpoints.accessURL
	dbPath, extra := resolveDevBackendEnv(repoRoot, startupConfig)
	homeDir := devKandevHome(repoRoot)
	if !isInsideKandevTask(repoRoot) && configSourceIsExplicit(startupConfig, "homeDir") {
		homeDir = startupConfig.ResolvedHomeDir()
	}
	return devLaunchConfig{
		repoRoot:  repoRoot,
		ports:     ports,
		dbPath:    dbPath,
		extra:     extra,
		logLevel:  resolveLogLevelForConfig(opts, startupConfig),
		debug:     opts.Debug,
		homeDir:   homeDir,
		startup:   startupConfig,
		endpoints: endpoints,
	}, 0
}

func devStartupConfig(configs ...*config.Config) (*config.Config, int) {
	if len(configs) > 0 {
		return configs[0], 0
	}
	// Load dev defaults with the same selector that the child receives.
	if os.Getenv("KANDEV_E2E_MOCK") == "" {
		if err := os.Setenv("KANDEV_DEBUG_DEV_MODE", "true"); err != nil {
			fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
			return nil, 1
		}
	}
	startupConfig, err := loadDevBootstrapConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return nil, 1
	}
	return startupConfig, 0
}

func loadDevBootstrapConfig() (*config.Config, error) {
	if os.Getenv("KANDEV_E2E_MOCK") != "" || os.Getenv("KANDEV_DEBUG_DEV_MODE") != "" {
		return loadBootstrapConfig()
	}

	const selector = "KANDEV_DEBUG_DEV_MODE"
	previous, present := os.LookupEnv(selector)
	if err := os.Setenv(selector, "true"); err != nil {
		return nil, err
	}
	cfg, err := loadBootstrapConfig()
	if present {
		if restoreErr := os.Setenv(selector, previous); err == nil {
			err = restoreErr
		}
	} else if restoreErr := os.Unsetenv(selector); err == nil {
		err = restoreErr
	}
	return cfg, err
}

// backupProductionDBIfNeeded snapshots the resolved DB before dev mode runs
// against it, unless it already lives under a .kandev-dev segment.
func backupProductionDBIfNeeded(dbPath string) error {
	if !isProductionDb(dbPath) {
		return nil
	}
	backupPath, err := backupProductionDb(dbPath, userHomeDir(), time.Now())
	if err != nil {
		// Abort rather than continue: the backup exists precisely to protect
		// the production db before dev mode touches it. Proceeding on failure
		// would remove the safety guarantee that justified the guard.
		return fmt.Errorf("failed to back up production db (%v); aborting dev startup", err)
	}
	if backupPath != "" {
		fmt.Printf("[kandev] backed up production db → %s\n", filepath.Base(backupPath))
	}
	return nil
}

// launchWebChild starts the Vite dev server under the supervisor, joining the
// same process group so Ctrl-C terminates the whole tree.
func launchWebChild(repoRoot string, ports portConfig, supervisor *processSupervisor) (*managedProcess, error) {
	webCmd, webArgs := webCommand()
	webEnv, err := buildWebEnv(ports, true)
	if err != nil {
		return nil, err
	}
	webProc, _, err := startProcessFn(webCmd, webArgs, repoRoot, webEnv, false, "web", supervisor)
	if err != nil {
		return nil, err
	}
	return webProc, nil
}
