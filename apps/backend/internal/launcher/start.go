package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func runStart(opts Options) int {
	backendPort, webPort, err := resolvePorts(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 2
	}
	ports, err := pickPorts(backendPort, webPort)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	repoRoot, err := findRepoRoot(mustGetwd())
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	webServer, err := resolveStandaloneServerPath(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	webDir := filepath.Dir(webServer)
	if err := ensureStandaloneAssets(repoRoot, webDir); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	if err := os.MkdirAll(resolveDataDir(), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	_ = os.Chmod(resolveDataDir(), 0o700)

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
		WebServer:  webServer,
		WebCWD:     webDir,
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
	WebServer  string
	WebCWD     string
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

	webURL := fmt.Sprintf("http://localhost:%d", cfg.Ports.WebPort)
	fmt.Println("[kandev] starting web...")
	web, _, err := startProcess("node", []string{cfg.WebServer}, cfg.WebCWD, webEnv(cfg.Ports, true, cfg.Opts.Debug), !showOutput, "web", supervisor)
	if err != nil {
		supervisor.shutdown("web start failure")
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	if err := waitForURL(webURL, web, healthTimeout(healthTimeoutReleaseMS)); err != nil {
		supervisor.shutdown("web health failure")
		fmt.Fprintln(os.Stderr, "[kandev] "+err.Error())
		return 1
	}
	if cfg.Opts.Headless {
		fmt.Printf("[kandev] ready (headless) at %s\n", cfg.Ports.BackendURL)
		return waitForAppExit(supervisor, backend, web)
	}
	fmt.Println("[kandev] open: " + cfg.Ports.BackendURL)
	openBrowser(cfg.Ports.BackendURL)
	return waitForAppExit(supervisor, backend, web)
}

func findRepoRoot(startDir string) (string, error) {
	current, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		if filepath.Base(current) == "apps" && exists(filepath.Join(current, "backend")) && exists(filepath.Join(current, "web")) {
			return filepath.Dir(current), nil
		}
		if exists(filepath.Join(current, "apps", "backend")) && exists(filepath.Join(current, "apps", "web")) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("unable to locate repo root for start. Run from the repo")
		}
		current = parent
	}
}

func resolveStandaloneServerPath(repoRoot string) (string, error) {
	standaloneDir := filepath.Join(repoRoot, "apps", "web", ".next", "standalone")
	expected := filepath.Join(standaloneDir, "web", "server.js")
	if exists(expected) {
		return expected, nil
	}
	var found string
	if exists(standaloneDir) {
		_ = filepath.WalkDir(standaloneDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || found != "" {
				return nil
			}
			if d.IsDir() && d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if !d.IsDir() && d.Name() == "server.js" && filepath.Base(filepath.Dir(path)) == "web" {
				found = path
			}
			return nil
		})
	}
	if found != "" {
		return found, nil
	}
	if !exists(standaloneDir) {
		return "", fmt.Errorf("web standalone build not found. Run `make build` first")
	}
	return "", fmt.Errorf("web standalone build is missing server.js under %s", standaloneDir)
}

func ensureStandaloneAssets(repoRoot, webDir string) error {
	webStatic := filepath.Join(repoRoot, "apps", "web", ".next", "static")
	standaloneStatic := filepath.Join(webDir, ".next", "static")
	if exists(webStatic) && !exists(standaloneStatic) {
		if err := os.MkdirAll(filepath.Dir(standaloneStatic), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(webStatic, standaloneStatic); err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to link Next.js static assets: %w", err)
		}
	}
	webPublic := filepath.Join(repoRoot, "apps", "web", "public")
	standalonePublic := filepath.Join(webDir, "public")
	if exists(webPublic) && !exists(standalonePublic) {
		if err := os.Symlink(webPublic, standalonePublic); err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to link public assets: %w", err)
		}
	}
	return nil
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

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
